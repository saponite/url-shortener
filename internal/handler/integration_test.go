//go:build integration

// go test ./internal/handler -tags=integration -race -v

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16",
		postgres.WithDatabase("test-db"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		//testcontainers.WithWaitStrategy(wait.ForLog("БД готова к приему подключений").WithOccurrence(2).WithStartupTimeout(5*time.Second)),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err = pgContainer.Terminate(ctx); err != nil {
			t.Fatalf("ошибка завершения работы pgContainer: %s", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	assert.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("подключение: %v", err)
	}
	t.Cleanup(pool.Close)

	_, err = pool.Exec(ctx, `
        CREATE TABLE links (
            id           BIGSERIAL PRIMARY KEY,
            short_code   VARCHAR(16) NOT NULL UNIQUE,
            original_url TEXT NOT NULL,
            created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
        );
    `)
	if err != nil {
		t.Fatalf("создание схемы: %v", err)
	}

	return pool
}

func TestCreateShortenedLink_Integration(t *testing.T) {
	pool := setupTestDB(t)

	h := New(pool, "http://localhost:1234/")

	body := strings.NewReader(`{"original_url": "https://github.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/create", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	h.CreateShortenedLink(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("ожидался 201, получили %d (тело: %s)", w.Code, w.Body.String())
	}

	var resp LinkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("распарсить ответ: %v", err)
	}

	if resp.OriginalURL != "https://github.com" {
		t.Errorf("OriginalURL = %q, ожидалось https://github.com", resp.OriginalURL)
	}
	if resp.ShortURL == "" {
		t.Errorf("ShortURL пустой")
	}
}

func TestCreateAndRedirect_Integration(t *testing.T) {
	pool := setupTestDB(t)
	h := New(pool, "http://localhost:1234/")

	body := strings.NewReader(`{"original_url": "https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/create", body)
	w := httptest.NewRecorder()
	h.CreateShortenedLink(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("создание: код %d, тело %s", w.Code, w.Body.String())
	}

	var created LinkResponse
	json.Unmarshal(w.Body.Bytes(), &created)

	code := strings.TrimPrefix(created.ShortURL, "http://localhost:1234/")

	router := chi.NewRouter()
	router.Get("/{code}", h.GetOriginalURL)

	req2 := httptest.NewRequest(http.MethodGet, "/"+code, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusFound {
		t.Errorf("редирект: код %d, ожидался 302", w2.Code)
	}
	if loc := w2.Header().Get("Location"); loc != "https://example.com" {
		t.Errorf("Location = %q, ожидалось https://example.com", loc)
	}
}
