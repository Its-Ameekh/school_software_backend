-- +goose Up
CREATE TABLE fee_structures (
    id                 SERIAL PRIMARY KEY,
    academic_year      VARCHAR(9) NOT NULL,
    grade_tier         VARCHAR(50) NOT NULL,
    initial_payment    NUMERIC(10,2) NOT NULL,
    regular_fee_total  NUMERIC(10,2) NOT NULL,
    created_at         TIMESTAMP DEFAULT now(),
    UNIQUE (academic_year, grade_tier)
);

CREATE TABLE fee_terms (
    id                SERIAL PRIMARY KEY,
    fee_structure_id  INT REFERENCES fee_structures(id) ON DELETE CASCADE,
    term_number       SMALLINT NOT NULL CHECK (term_number BETWEEN 1 AND 4),
    amount            NUMERIC(10,2) NOT NULL,
    due_date          DATE NOT NULL,
    UNIQUE (fee_structure_id, term_number)
);

CREATE TABLE fee_materials (
    id                SERIAL PRIMARY KEY,
    fee_structure_id  INT REFERENCES fee_structures(id) ON DELETE CASCADE,
    term_number       SMALLINT NOT NULL,
    item_label        VARCHAR(100) NOT NULL,
    amount            NUMERIC(10,2) NOT NULL
);

-- payment_method already supports 'online' (Razorpay, v2) alongside v1's bank|desk
CREATE TABLE student_fee_ledger (
    id              SERIAL PRIMARY KEY,
    student_id      INT REFERENCES students(id) ON DELETE CASCADE,
    fee_term_id     INT REFERENCES fee_terms(id),
    amount_due      NUMERIC(10,2) NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    payment_method  VARCHAR(20),
    waive_reason    TEXT,
    paid_at         TIMESTAMP,
    created_at      TIMESTAMP DEFAULT now()
);

CREATE TABLE student_transport_ledger (
    id              SERIAL PRIMARY KEY,
    student_id      INT REFERENCES students(id) ON DELETE CASCADE,
    term_number     SMALLINT NOT NULL,
    amount_due      NUMERIC(10,2) NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    payment_method  VARCHAR(20),
    waive_reason    TEXT,
    paid_at         TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS student_transport_ledger;
DROP TABLE IF EXISTS student_fee_ledger;
DROP TABLE IF EXISTS fee_materials;
DROP TABLE IF EXISTS fee_terms;
DROP TABLE IF EXISTS fee_structures;
