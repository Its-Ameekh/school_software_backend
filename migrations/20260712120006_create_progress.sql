-- +goose Up
CREATE TABLE progress_scores (
    id            SERIAL PRIMARY KEY,
    student_id    INT REFERENCES students(id) ON DELETE CASCADE,
    term          VARCHAR(20) NOT NULL,
    subject       VARCHAR(100) NOT NULL,
    max_score     NUMERIC(5,2) NOT NULL,
    scored_value  NUMERIC(5,2) NOT NULL,
    grade_value   VARCHAR(5) NOT NULL,
    graded_by     INT REFERENCES users(id),
    updated_at    TIMESTAMP DEFAULT now(),
    UNIQUE (student_id, term, subject)
);

CREATE TABLE progress_remarks (
    student_id  INT REFERENCES students(id) ON DELETE CASCADE,
    term        VARCHAR(20) NOT NULL,
    remarks     TEXT NOT NULL,
    written_by  INT REFERENCES users(id),
    updated_at  TIMESTAMP DEFAULT now(),
    PRIMARY KEY (student_id, term)
);

-- +goose Down
DROP TABLE IF EXISTS progress_remarks;
DROP TABLE IF EXISTS progress_scores;
