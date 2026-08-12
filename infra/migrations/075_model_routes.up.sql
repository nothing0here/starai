CREATE TABLE model_routes (
  id BIGSERIAL PRIMARY KEY,
  model_id BIGINT NOT NULL REFERENCES models(id) ON DELETE CASCADE,
  route_name VARCHAR(128) NOT NULL,
  provider VARCHAR(64) NOT NULL DEFAULT '',
  protocol VARCHAR(32) NOT NULL DEFAULT 'openai',
  upstream_model VARCHAR(128) NOT NULL,
  endpoint VARCHAR(256) NOT NULL,
  base_url TEXT NOT NULL,
  api_key TEXT NOT NULL DEFAULT '',
  auth_type VARCHAR(32) NOT NULL DEFAULT 'bearer',
  api_key_header VARCHAR(128) NOT NULL DEFAULT 'Authorization',
  headers JSONB NOT NULL DEFAULT '{}',
  extra_params JSONB NOT NULL DEFAULT '{}',
  runtime_rule JSONB NOT NULL DEFAULT '{}',
  cost_rule JSONB NOT NULL DEFAULT '{}',
  priority INT NOT NULL DEFAULT 100,
  weight INT NOT NULL DEFAULT 100 CHECK (weight >= 0),
  timeout_seconds INT NOT NULL DEFAULT 120 CHECK (timeout_seconds > 0),
  max_retries INT NOT NULL DEFAULT 0 CHECK (max_retries >= 0),
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  health_status VARCHAR(32) NOT NULL DEFAULT 'healthy',
  consecutive_failures INT NOT NULL DEFAULT 0,
  success_count BIGINT NOT NULL DEFAULT 0,
  failure_count BIGINT NOT NULL DEFAULT 0,
  last_success_at TIMESTAMPTZ,
  last_failure_at TIMESTAMPTZ,
  cooldown_until TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (model_id, route_name)
);

CREATE INDEX idx_model_routes_select
  ON model_routes(model_id, is_enabled, priority, health_status, cooldown_until);

CREATE TABLE model_route_attempts (
  id BIGSERIAL PRIMARY KEY,
  request_id VARCHAR(64) NOT NULL,
  model_id BIGINT REFERENCES models(id) ON DELETE SET NULL,
  route_id BIGINT REFERENCES model_routes(id) ON DELETE SET NULL,
  attempt INT NOT NULL DEFAULT 1,
  status VARCHAR(32) NOT NULL,
  status_code INT,
  error_code VARCHAR(64),
  latency_ms INT NOT NULL DEFAULT 0,
  provider_cost NUMERIC(18,8) NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_model_route_attempts_request ON model_route_attempts(request_id, attempt);
CREATE INDEX idx_model_route_attempts_route_time ON model_route_attempts(route_id, created_at DESC);

ALTER TABLE ai_call_logs
  ADD COLUMN route_id BIGINT REFERENCES model_routes(id) ON DELETE SET NULL,
  ADD COLUMN provider_cost NUMERIC(18,8) NOT NULL DEFAULT 0,
  ADD COLUMN gross_profit NUMERIC(18,8) NOT NULL DEFAULT 0;

ALTER TABLE tasks
  ADD COLUMN route_id BIGINT REFERENCES model_routes(id) ON DELETE SET NULL,
  ADD COLUMN provider_cost NUMERIC(18,8) NOT NULL DEFAULT 0;

-- Preserve every existing model connection as its first route. New and existing
-- deployments therefore keep working without manual re-entry after migration.
INSERT INTO model_routes (
  model_id, route_name, provider, protocol, upstream_model, endpoint,
  base_url, api_key, auth_type, api_key_header, headers, extra_params,
  runtime_rule, priority, weight, timeout_seconds, is_enabled
)
SELECT
  m.id,
  '默认线路',
  COALESCE(m.new_api_extra_params #>> '{connection,provider}', ''),
  COALESCE(NULLIF(m.new_api_extra_params #>> '{connection,protocol}', ''), 'openai'),
  m.new_api_model,
  m.new_api_endpoint,
  COALESCE(m.new_api_extra_params #>> '{connection,base_url}', ''),
  COALESCE(m.new_api_extra_params #>> '{connection,api_key}', ''),
  COALESCE(NULLIF(m.new_api_extra_params #>> '{connection,auth_type}', ''), 'bearer'),
  COALESCE(NULLIF(m.new_api_extra_params #>> '{connection,api_key_header}', ''), 'Authorization'),
  COALESCE(m.new_api_extra_params #> '{connection,headers}', '{}'::jsonb),
  m.new_api_extra_params - 'connection',
  '{}'::jsonb,
  100,
  100,
  120,
  m.is_enabled
FROM models m
WHERE m.category <> 'multi_collab'
  AND COALESCE(m.new_api_extra_params #>> '{connection,base_url}', '') <> ''
ON CONFLICT (model_id, route_name) DO NOTHING;
