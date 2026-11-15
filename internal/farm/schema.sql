CREATE TABLE IF NOT EXISTS gomate_farm_config (id bool PRIMARY KEY DEFAULT true CHECK(id), revision bigint NOT NULL DEFAULT 1, data jsonb NOT NULL);
CREATE TABLE IF NOT EXISTS gomate_farm_flags (
 id bigserial PRIMARY KEY, flag text NOT NULL UNIQUE, team text NOT NULL, sploit text NOT NULL,
 status text NOT NULL DEFAULT 'QUEUED', response text NOT NULL DEFAULT '', attempts int NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), next_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS gomate_farm_queue ON gomate_farm_flags(next_at,id) WHERE status IN ('QUEUED','SUBMITTING');
CREATE INDEX IF NOT EXISTS gomate_farm_created ON gomate_farm_flags(created_at DESC,id DESC);
