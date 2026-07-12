-- +goose Up
CREATE TABLE transport_routes (
    id            SERIAL PRIMARY KEY,
    bus_number    VARCHAR(20) NOT NULL,
    driver_name   VARCHAR(100),
    driver_phone  VARCHAR(20),
    route_name    VARCHAR(100)
);

CREATE TABLE student_transport_assignment (
    student_id  INT PRIMARY KEY REFERENCES students(id) ON DELETE CASCADE,
    route_id    INT REFERENCES transport_routes(id),
    mode        VARCHAR(20) NOT NULL
);

-- OTP fields ship in v1 schema but are DEFERRED from active use (Part 4, item 3):
-- v1 only toggles status PENDING/PICKED_UP manually; otp_* columns stay unused
-- until OTP generation is actually built, avoiding a future schema change.
CREATE TABLE daily_pickup_log (
    id                 SERIAL PRIMARY KEY,
    student_id         INT REFERENCES students(id) ON DELETE CASCADE,
    date               DATE NOT NULL,
    pickup_type        VARCHAR(30) NOT NULL,
    picked_up_by_name  VARCHAR(255),
    otp_code           VARCHAR(6),
    otp_verified       BOOLEAN DEFAULT false,
    otp_generated_at   TIMESTAMP,
    otp_verified_at    TIMESTAMP,
    UNIQUE (student_id, date)
);

-- +goose Down
DROP TABLE IF EXISTS daily_pickup_log;
DROP TABLE IF EXISTS student_transport_assignment;
DROP TABLE IF EXISTS transport_routes;
