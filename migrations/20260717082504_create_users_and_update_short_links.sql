-- +goose Up
create extension if not exists pgcrypto;

create table users (
    id         uuid primary key default gen_random_uuid(),
    first_name text not null,
    last_name  text not null,
    email      text not null,
    created_at timestamptz not null default now()
);

create unique index on users (lower(email));

create table short_links (
    id           uuid primary key default gen_random_uuid(),
    original_url text not null,
    short_code   text not null unique,
    created_at   timestamptz not null default now(),
    user_id      uuid not null references users(id) on delete restrict,
    click_count  bigint not null default 0,
    expires_at   timestamptz
);

create index on short_links(user_id);
create index on short_links(expires_at) where expires_at is not null;

-- +goose Down
drop table if exists short_links;
drop table if exists users;