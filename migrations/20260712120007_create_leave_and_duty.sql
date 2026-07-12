-- +goose Up
CREATE TABLE teacher_leave_requests (
    id           SERIAL PRIMARY KEY,
    teacher_id   INT REFERENCES users(id) ON DELETE CASCADE,
    from_date    DATE NOT NULL,
    to_date      DATE NOT NULL,
    leave_type   VARCHAR(30) NOT NULL,
    reason       TEXT NOT NULL,
    status       VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    reviewed_by  INT REFERENCES users(id),
    reviewed_at  TIMESTAMP,
    created_at   TIMESTAMP DEFAULT now()
);

CREATE TABLE teacher_duty_assignments (
    id            SERIAL PRIMARY KEY,
    teacher_id    INT REFERENCES users(id) ON DELETE CASCADE,
    date          DATE NOT NULL,
    duty_type     VARCHAR(100) NOT NULL,
    location      VARCHAR(100),
    start_time    TIME,
    end_time      TIME,
    instructions  TEXT
);

-- Student leave requests: intentionally a separate table from teacher_leave_requests
-- (Part 4, item 10) — no substitute-coverage implications on the student side.
CREATE TABLE leave_requests (
    id            SERIAL PRIMARY KEY,
    student_id    INT REFERENCES students(id) ON DELETE CASCADE,
    requested_by  INT REFERENCES users(id),
    date          DATE NOT NULL,
    reason        TEXT NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    reviewed_by   INT REFERENCES users(id),
    reviewed_at   TIMESTAMP,
    created_at    TIMESTAMP DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS leave_requests;
DROP TABLE IF EXISTS teacher_duty_assignments;
DROP TABLE IF EXISTS teacher_leave_requests;
