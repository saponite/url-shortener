-- +goose Up
CREATE TABLE links (
    id           BIGSERIAL PRIMARY KEY,
    short_code   TEXT NOT NULL UNIQUE,
    original_url TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE links;