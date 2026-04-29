package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const linkPrefix = "https://short.ly/"
const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type Link struct {
	OriginalURL string    `json:"original_url"`
	ShortCode   string    `json:"short_code"`
	CreatedAt   time.Time `json:"created_at"`
}

type LinkResponse struct {
	OriginalURL string    `json:"original_url"`
	ShortURL    string    `json:"short_url"`
	CreatedAt   time.Time `json:"created_at"`
}

type Handler struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

func (h *Handler) HandlerCreateShortenedLink(w http.ResponseWriter, r *http.Request) {
	var link Link
	if err := json.NewDecoder(r.Body).Decode(&link); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	link.OriginalURL = strings.TrimRight(link.OriginalURL, "/")
	if link.OriginalURL == "" {
		http.Error(w, "original_url пуст", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	exists, err := h.getByOriginalURL(ctx, link.OriginalURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if exists != nil {
		writeJSON(w, http.StatusOK, exists)
		return
	}

	code, err := h.createWithRetry(ctx, link.OriginalURL, 5)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	created, err := h.getByCode(ctx, code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, toResponse(created))
}

func (h *Handler) getByOriginalURL(ctx context.Context, url string) (*Link, error) {
	var l Link
	err := h.pool.QueryRow(ctx, `
        SELECT original_url, short_code, created_at
        FROM links WHERE original_url = $1
    `, url).Scan(&l.OriginalURL, &l.ShortCode, &l.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (h *Handler) getByCode(ctx context.Context, code string) (*Link, error) {
	var l Link
	err := h.pool.QueryRow(ctx, `
        SELECT original_url, short_code, created_at
        FROM links
        WHERE short_code = $1
    `, code).Scan(&l.OriginalURL, &l.ShortCode, &l.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (h *Handler) createWithRetry(ctx context.Context, url string, maxRetries int) (string, error) {
	for i := 0; i < maxRetries; i++ {
		code := generateShortCode()

		var inserted string
		err := h.pool.QueryRow(ctx, `
            INSERT INTO links (short_code, original_url)
            VALUES ($1, $2)
            ON CONFLICT (short_code) DO NOTHING
            RETURNING short_code
        `, code, url).Scan(&inserted)

		if err == nil {
			return inserted, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		return "", err
	}
	return "", errors.New("не удалось сгенерировать короткий код")
}

func generateShortCode() string {
	const length = 7 // 62 - алфавит, 7 позиций => 62^7 комбинаций
	b := make([]rune, length)
	for i := range b {
		b[i] = rune(alphabet[rand.Uint32N(uint32(len(alphabet)))])
	}
	return string(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("ошибка при кодировке ответа: %v", err)
	}
}

func toResponse(l *Link) LinkResponse {
	return LinkResponse{
		OriginalURL: l.OriginalURL,
		ShortURL:    linkPrefix + l.ShortCode,
		CreatedAt:   l.CreatedAt,
	}
}
