-- +goose Up
CREATE TABLE users (
    id              SERIAL PRIMARY KEY,
    auth_id         UUID UNIQUE NOT NULL,
    email           VARCHAR(255) UNIQUE,
    role            VARCHAR(20) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    phone           VARCHAR(20) UNIQUE NOT NULL,
    avatar_url      TEXT,
    created_at      TIMESTAMP DEFAULT now(),
    deleted_at      TIMESTAMP
);

CREATE TABLE teacher_profiles (
    user_id             INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    specialization      VARCHAR(100),
    is_available_sub    BOOLEAN DEFAULT true
);

-- +goose Down
DROP TABLE IF EXISTS teacher_profiles;
DROP TABLE IF EXISTS users;
