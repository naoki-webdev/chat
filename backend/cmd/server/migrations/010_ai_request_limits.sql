CREATE TABLE IF NOT EXISTS ai_request_limits (
    request_key TEXT PRIMARY KEY,
    in_flight BOOLEAN NOT NULL DEFAULT FALSE,
    last_started_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
