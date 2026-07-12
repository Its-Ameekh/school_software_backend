-- +goose Up
CREATE TABLE students (
    id                  SERIAL PRIMARY KEY,
    roll_number         VARCHAR(20) UNIQUE NOT NULL,
    full_name           VARCHAR(255) NOT NULL,
    dob                 DATE NOT NULL,
    gender              VARCHAR(10) NOT NULL,
    blood_group         VARCHAR(5),
    allergies           TEXT,
    special_talents     TEXT,
    languages_spoken    VARCHAR(255),
    food_type           VARCHAR(20),
    class_id            INT REFERENCES classes(id),
    grade_tier          VARCHAR(50) NOT NULL,
    created_at          TIMESTAMP DEFAULT now(),
    deleted_at          TIMESTAMP
);

CREATE TABLE guardians (
    id                    SERIAL PRIMARY KEY,
    student_id            INT REFERENCES students(id) ON DELETE CASCADE,
    user_id               INT REFERENCES users(id),
    full_name             VARCHAR(255) NOT NULL,
    relationship          VARCHAR(30) NOT NULL,
    occupation            VARCHAR(100),
    email                 VARCHAR(255),
    mobile                VARCHAR(20),
    is_primary_contact    BOOLEAN DEFAULT false,
    authorized_for_pickup BOOLEAN DEFAULT true,
    deleted_at            TIMESTAMP
);

CREATE TABLE admission_intake (
    id              SERIAL PRIMARY KEY,
    student_id      INT REFERENCES students(id) ON DELETE CASCADE,
    pay_mode        VARCHAR(20) NOT NULL,
    amount_paid     NUMERIC(10,2) NOT NULL,
    receipt_number  VARCHAR(50) NOT NULL,
    transport_pref  VARCHAR(20) NOT NULL,
    admitted_at     TIMESTAMP DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS admission_intake;
DROP TABLE IF EXISTS guardians;
DROP TABLE IF EXISTS students;
