CREATE TABLE java_request_keys (
  request_key VARCHAR(256) PRIMARY KEY,
  user_id BIGINT NOT NULL,
  fingerprint VARCHAR(64) NOT NULL,
  response TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
