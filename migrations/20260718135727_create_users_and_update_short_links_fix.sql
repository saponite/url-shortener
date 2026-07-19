-- +goose Up
alter table short_links alter column user_id drop not null;

-- +goose Down
alter table short_links alter column user_id set not null;