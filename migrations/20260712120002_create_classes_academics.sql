-- +goose Up
CREATE TABLE classes (
    id                    SERIAL PRIMARY KEY,
    name                  VARCHAR(100) NOT NULL,
    teacher_id            INT REFERENCES users(id),
    substitute_teacher_id INT REFERENCES users(id),
    substitute_active     BOOLEAN DEFAULT false
);

CREATE TABLE timetable_slots (
    id            SERIAL PRIMARY KEY,
    class_id      INT REFERENCES classes(id) ON DELETE CASCADE,
    day_of_week   VARCHAR(10) NOT NULL,
    period_number SMALLINT NOT NULL,
    subject       VARCHAR(100) NOT NULL,
    room          VARCHAR(50),
    start_time    TIME,
    end_time      TIME,
    UNIQUE (class_id, day_of_week, period_number)
);

-- +goose Down
DROP TABLE IF EXISTS timetable_slots;
DROP TABLE IF EXISTS classes;
