CREATE INDEX IF NOT EXISTS auth_rate_limit_buckets_window_idx
  ON auth_rate_limit_buckets(window_started_at);

CREATE INDEX IF NOT EXISTS ai_daily_usage_date_idx
  ON ai_daily_usage(usage_date);

CREATE INDEX IF NOT EXISTS ai_request_limits_updated_idx
  ON ai_request_limits(updated_at)
  WHERE in_flight = false;
