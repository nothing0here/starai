DELETE FROM system_configs
WHERE key IN (
  'web_search_enabled',
  'web_search_provider',
  'web_search_api_key',
  'web_search_base_url',
  'web_search_max_results',
  'web_search_timeout_sec',
  'web_search_daily_limit'
);
