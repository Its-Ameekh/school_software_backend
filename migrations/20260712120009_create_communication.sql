-- +goose Up
-- priority column included directly here (Normal|Important teacher axis),
-- rather than as a separate ALTER, since this is the initial migration.
CREATE TABLE bulletin (
    id              SERIAL PRIMARY KEY,
    type            VARCHAR(20) NOT NULL,
    title           VARCHAR(255),
    content         TEXT NOT NULL,
    category_tag    VARCHAR(30),
    priority        VARCHAR(20) DEFAULT 'Normal',
    scheduled_date  TIMESTAMP,
    created_by      INT REFERENCES users(id),
    class_id        INT REFERENCES classes(id),
    created_at      TIMESTAMP DEFAULT now()
);

CREATE TABLE messages (
    id                  SERIAL PRIMARY KEY,
    sender_id           INT REFERENCES users(id),
    recipient_id        INT REFERENCES users(id),
    student_context_id  INT REFERENCES students(id),
    content             TEXT NOT NULL,
    read_at             TIMESTAMP,
    created_at          TIMESTAMP DEFAULT now()
);

CREATE TABLE gallery_photos (
    id           SERIAL PRIMARY KEY,
    class_id     INT REFERENCES classes(id),
    url          TEXT NOT NULL,
    caption      VARCHAR(255),
    uploaded_by  INT REFERENCES users(id),
    uploaded_at  TIMESTAMP DEFAULT now()
);

CREATE TABLE broadcast_logs (
    id               SERIAL PRIMARY KEY,
    fee_type         VARCHAR(20) NOT NULL,
    scope            VARCHAR(20) NOT NULL,
    class_id         INT REFERENCES classes(id),
    channels         VARCHAR(50) NOT NULL,
    recipient_count  INT NOT NULL,
    sent_by          INT REFERENCES users(id),
    sent_at          TIMESTAMP DEFAULT now()
);

CREATE TABLE daily_audit_reports (
    id                  SERIAL PRIMARY KEY,
    report_date         DATE UNIQUE NOT NULL,
    attendance_summary  JSONB,
    finance_summary     JSONB,
    signed_off_by       INT REFERENCES users(id),
    signed_off_at       TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS daily_audit_reports;
DROP TABLE IF EXISTS broadcast_logs;
DROP TABLE IF EXISTS gallery_photos;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS bulletin;
