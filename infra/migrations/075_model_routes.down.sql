ALTER TABLE tasks DROP COLUMN IF EXISTS provider_cost;
ALTER TABLE tasks DROP COLUMN IF EXISTS route_id;
ALTER TABLE ai_call_logs DROP COLUMN IF EXISTS gross_profit;
ALTER TABLE ai_call_logs DROP COLUMN IF EXISTS provider_cost;
ALTER TABLE ai_call_logs DROP COLUMN IF EXISTS route_id;
DROP TABLE IF EXISTS model_route_attempts;
DROP TABLE IF EXISTS model_routes;
