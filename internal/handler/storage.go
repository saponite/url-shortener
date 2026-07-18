package handler

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCodeTaken = errors.New("short_code уже занят")

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
        FROM links WHERE original_url = $1
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
        FROM links
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
        INSERT INTO links (short_code, original_url)
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
