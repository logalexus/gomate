CREATE TABLE IF NOT EXISTS gomate_services (
 port integer PRIMARY KEY CHECK (port BETWEEN 1 AND 65535),
 data jsonb NOT NULL
);
CREATE TABLE IF NOT EXISTS gomate_patterns (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 data jsonb NOT NULL
);
CREATE TABLE IF NOT EXISTS gomate_streams (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 flow_key text NOT NULL UNIQUE,
 service integer NOT NULL,
 protocol text NOT NULL,
 start_ms bigint NOT NULL,
 end_ms bigint NOT NULL,
 pattern_ids integer[] NOT NULL DEFAULT '{}',
 favorite boolean NOT NULL DEFAULT false,
 incomplete boolean NOT NULL DEFAULT false,
 ignored boolean NOT NULL DEFAULT false,
 ttl integer NOT NULL,
 size_bytes bigint NOT NULL DEFAULT 0,
 packets_count integer NOT NULL DEFAULT 0,
 client text NOT NULL,
 server text NOT NULL
);
CREATE INDEX IF NOT EXISTS gomate_stream_service_id ON gomate_streams (service, id DESC) WHERE NOT ignored;
CREATE INDEX IF NOT EXISTS gomate_stream_favorite ON gomate_streams (id DESC) WHERE favorite AND NOT ignored;
CREATE INDEX IF NOT EXISTS gomate_stream_patterns ON gomate_streams USING gin (pattern_ids);
CREATE INDEX IF NOT EXISTS gomate_stream_end ON gomate_streams (end_ms) WHERE NOT favorite;
CREATE TABLE IF NOT EXISTS gomate_packets (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 stream_id bigint NOT NULL REFERENCES gomate_streams(id) ON DELETE CASCADE,
 timestamp_ms bigint NOT NULL,
 incoming boolean NOT NULL,
 content bytea NOT NULL,
 matches jsonb NOT NULL,
 incomplete boolean NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS gomate_packet_stream_id ON gomate_packets (stream_id, id);
CREATE TABLE IF NOT EXISTS gomate_chunks (
 token text PRIMARY KEY,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS gomate_chunks_created ON gomate_chunks(created_at);

ALTER TABLE gomate_packets ADD COLUMN IF NOT EXISTS details jsonb NOT NULL DEFAULT '{}';
ALTER TABLE gomate_streams ADD COLUMN IF NOT EXISTS user_agent_hash text;
CREATE TABLE IF NOT EXISTS gomate_schema_versions(version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
INSERT INTO gomate_schema_versions(version) VALUES (1),(2) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS gomate_jobs (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 pattern jsonb NOT NULL,
 since_ms bigint NOT NULL,
 max_stream bigint NOT NULL,
 max_packet bigint NOT NULL,
 stream_cursor bigint NOT NULL DEFAULT 0,
 packet_cursor bigint NOT NULL DEFAULT 0,
 match_state jsonb NOT NULL DEFAULT '{}',
 status text NOT NULL DEFAULT 'queued',
 processed bigint NOT NULL DEFAULT 0,
 matched bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS gomate_one_lookback ON gomate_jobs ((true)) WHERE status IN ('queued','running');
INSERT INTO gomate_schema_versions(version) VALUES (3) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS gomate_import_state (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 source_id text NOT NULL,
 phase text NOT NULL DEFAULT 'settings',
 stream_cursor bigint NOT NULL DEFAULT 0,
 packet_cursor bigint NOT NULL DEFAULT 0
);
INSERT INTO gomate_schema_versions(version) VALUES (4) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS gomate_settings (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 revision bigint NOT NULL DEFAULT 1,
 data jsonb NOT NULL
);
ALTER TABLE gomate_streams ADD COLUMN IF NOT EXISTS payload_purged boolean NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS gomate_packets_time ON gomate_packets(timestamp_ms,id);
CREATE TABLE IF NOT EXISTS gomate_cleanup_jobs (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 mode text NOT NULL CHECK(mode IN ('packets','sessions')),
 preserve_favorites boolean NOT NULL,
 service integer NOT NULL DEFAULT 0,
 max_id bigint NOT NULL,
 status text NOT NULL DEFAULT 'queued',
 deleted bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS gomate_one_cleanup ON gomate_cleanup_jobs((true)) WHERE status IN ('queued','running');
INSERT INTO gomate_schema_versions(version) VALUES (5) ON CONFLICT DO NOTHING;
ALTER TABLE gomate_cleanup_jobs ADD COLUMN IF NOT EXISTS max_packet bigint NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS gomate_chat (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 author text NOT NULL,
 client_key text NOT NULL,
 body text NOT NULL CHECK(octet_length(body) BETWEEN 1 AND 8000),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(author,client_key)
);
CREATE INDEX IF NOT EXISTS gomate_chat_created ON gomate_chat(created_at);
INSERT INTO gomate_schema_versions(version) VALUES (6) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS gomate_anomalies (
 stream_id bigint PRIMARY KEY REFERENCES gomate_streams(id) ON DELETE CASCADE,
 service integer NOT NULL,
 score real NOT NULL,
 kind text NOT NULL DEFAULT '',
 summary text NOT NULL,
 reasons jsonb NOT NULL,
 dismissed boolean NOT NULL DEFAULT false,
 updated_ms bigint NOT NULL
);
CREATE INDEX IF NOT EXISTS gomate_anomalies_score ON gomate_anomalies(service, score DESC);
INSERT INTO gomate_schema_versions(version) VALUES (7) ON CONFLICT DO NOTHING;
