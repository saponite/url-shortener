-- +goose Up
alter table users add column password text not null;
alter table users add constraint email unique (email);

-- +goose Down
alter table users drop constraint email;
alter table users drop column password;