-- +goose Up
-- Append-only. Never updated or deleted at the DB level. Written from the
-- application layer only (not triggers) so before/after state matches what
-- the API actually validated.
CREATE TABLE audit_log (
    id             BIGSERIAL PRIMARY KEY,
    actor_user_id  INT REFERENCES users(id),
    action         VARCHAR(20) NOT NULL,
    table_name     VARCHAR(50) NOT NULL,
    record_id      INT NOT NULL,
    before_state   JSONB,
    after_state    JSONB,
    created_at     TIMESTAMP DEFAULT now()
);

CREATE INDEX idx_audit_log_table_record ON audit_log(table_name, record_id);
CREATE INDEX idx_audit_log_actor ON audit_log(actor_user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_audit_log_actor;
DROP INDEX IF EXISTS idx_audit_log_table_record;
DROP TABLE IF EXISTS audit_log;
