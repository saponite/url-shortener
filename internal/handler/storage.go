package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrCodeTaken    = errors.New("short_code уже занят")
	ErrUserNotFound = errors.New("пользователя с таким email не существует")
)

type LinkStorage interface {
	GetByOriginalURL(ctx context.Context, url string) (*Link, error)
	GetByCode(ctx context.Context, code string) (*Link, error)
	CreateNewShortLink(ctx context.Context, code, url string) (string, error)
}

type PgStorage struct {
	pool *pgxpool.Pool
}

func NewPGStorage(pool *pgxpool.Pool) *PgStorage {
	return &PgStorage{pool: pool}
}

func (s *PgStorage) GetByOriginalURL(ctx context.Context, originalURL string) (*Link, error) {
	var l Link
	err := s.pool.QueryRow(ctx, `
        SELECT original_url, short_code, created_at
        FROM short_links WHERE original_url = $1
    `, originalURL).Scan(&l.OriginalURL, &l.ShortCode, &l.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *PgStorage) GetByCode(ctx context.Context, code string) (*Link, error) {
	var l Link
	err := s.pool.QueryRow(ctx, `
        SELECT original_url, short_code, created_at
        FROM short_links
        WHERE short_code = $1
    `, code).Scan(&l.OriginalURL, &l.ShortCode, &l.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *PgStorage) CreateNewShortLink(ctx context.Context, code, url string) (string, error) {
	var inserted string
	err := s.pool.QueryRow(ctx, `
        INSERT INTO short_links (short_code, original_url)
        VALUES ($1, $2)
        ON CONFLICT (short_code) DO NOTHING
        RETURNING short_code
    `, code, url).Scan(&inserted)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCodeTaken
	}
	if err != nil {
		return "", err
	}
	return inserted, err
}

func (s *PgStorage) CreateNewUserAccount(ctx context.Context, firstName, lastName, email, password string) error {
	// Exec, так как не делаем returning
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (first_name, last_name, email, password)
		VALUES ($1, $2, $3, $4)
	`, firstName, lastName, email, password)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation {
				return fmt.Errorf("аккаунт с email %s существует: %w", email, err)
			}
		}
	}

	return nil
}

func (s *PgStorage) UpdatePassword(ctx context.Context, email, password string) error {
	result, err := s.pool.Exec(ctx, `
    	update users
    	set password = $1
    	where email = $2
	`, password, email)

	if err != nil {
		return fmt.Errorf("ошибка обновления пароля: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

func (s *PgStorage) UpdateFirstNameAndLastName(ctx context.Context, userId uuid.UUID, firstName, lastName string) error {
	result, err := s.pool.Exec(ctx, `
    	update users
    	set first_name = $1, last_name = $2
    	where id = $3
	`, firstName, lastName, userId.String())

	if err != nil {
		return fmt.Errorf("ошибка обновления имени и фамилии: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

func (s *PgStorage) UpdateEmail(ctx context.Context, userId uuid.UUID, email string) error {
	result, err := s.pool.Exec(ctx, `
    	update users
    	set email = $1
    	where id = $2
	`, email, userId.String())

	if err != nil {
		return fmt.Errorf("ошибка обновления почты: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}
