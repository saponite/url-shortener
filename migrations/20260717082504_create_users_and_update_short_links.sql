-- +goose Up
create table users (
    id bigserial primary key,
    first_name text not null,
    last_name text not null,
    email text not null,
    created_at timestamptz not null default now()
);

create unique index on users (lower(email));

create table short_links (
    id bigserial primary key,
    original_url text not null,
    short_code text not null unique,
    created_at timestamptz not null default now(),
    user_id bigint not null references users(id) on delete restrict,
    click_count bigint not null default 0,
    expires_at timestamptz
);

create index on short_links(user_id);
create index on short_links(expires_at) where expires_at is not null;

-- +goose Down
CREATE TABLE links (
    id           BIGSERIAL PRIMARY KEY,
    short_code   VARCHAR(16) NOT NULL UNIQUE,
    original_url TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DROP TABLE users;
DROP TABLE short_links;