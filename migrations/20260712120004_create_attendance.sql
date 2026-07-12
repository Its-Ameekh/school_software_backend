-- +goose Up
CREATE TABLE attendance (
    id                    SERIAL PRIMARY KEY,
    student_id            INT REFERENCES students(id) ON DELETE CASCADE,
    class_id              INT REFERENCES classes(id),
    date                  DATE NOT NULL,
    status                VARCHAR(10) NOT NULL,
    marked_by             INT REFERENCES users(id),
    locked_at             TIMESTAMP,
    locked_by_principal   BOOLEAN DEFAULT false,
    UNIQUE (student_id, date)
);

CREATE TABLE attendance_notifications (
    id                SERIAL PRIMARY KEY,
    attendance_id     INT REFERENCES attendance(id) ON DELETE CASCADE,
    guardian_user_id  INT REFERENCES users(id),
    trigger_reason    VARCHAR(20) NOT NULL,
    sent_at           TIMESTAMP,
    status            VARCHAR(20) DEFAULT 'PENDING'
);

CREATE TABLE attendance_submissions (
    class_id      INT REFERENCES classes(id),
    date          DATE NOT NULL,
    submitted_by  INT REFERENCES users(id),
    submitted_at  TIMESTAMP DEFAULT now(),
    PRIMARY KEY (class_id, date)
);

-- +goose Down
DROP TABLE IF EXISTS attendance_submissions;
DROP TABLE IF EXISTS attendance_notifications;
DROP TABLE IF EXISTS attendance;
