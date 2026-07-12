-- +goose Up
CREATE TABLE homework_assignments (
    id          SERIAL PRIMARY KEY,
    class_id    INT REFERENCES classes(id) ON DELETE CASCADE,
    title       VARCHAR(255) NOT NULL,
    description TEXT,
    due_date    DATE,
    created_by  INT REFERENCES users(id),
    created_at  TIMESTAMP DEFAULT now()
);

CREATE TABLE homework_submissions (
    id             SERIAL PRIMARY KEY,
    assignment_id  INT REFERENCES homework_assignments(id) ON DELETE CASCADE,
    student_id     INT REFERENCES students(id) ON DELETE CASCADE,
    status         VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    file_url       TEXT,
    submitted_at   TIMESTAMP,
    UNIQUE (assignment_id, student_id)
);

CREATE TABLE worksheets (
    id               SERIAL PRIMARY KEY,
    class_id         INT REFERENCES classes(id),
    file_name        VARCHAR(255) NOT NULL,
    file_url         TEXT NOT NULL,
    file_size_bytes  BIGINT,
    uploaded_by      INT REFERENCES users(id),
    uploaded_at      TIMESTAMP DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS worksheets;
DROP TABLE IF EXISTS homework_submissions;
DROP TABLE IF EXISTS homework_assignments;
