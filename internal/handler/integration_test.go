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
        CREATE TABLE users (
            id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
            first_name text NOT NULL,
            last_name  text NOT NULL,
            email      text NOT NULL,
            password   text NOT NULL,
            created_at timestamptz NOT NULL DEFAULT now()
        );
        CREATE UNIQUE INDEX ON users (lower(email));

        CREATE TABLE short_links (
            id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
            original_url text NOT NULL,
            short_code   text NOT NULL UNIQUE,
            created_at   timestamptz NOT NULL DEFAULT now(),
            user_id      uuid REFERENCES users(id) ON DELETE RESTRICT,
            click_count  bigint NOT NULL DEFAULT 0,
            expires_at   timestamptz
        );
    `)
	if err != nil {
		t.Fatalf("создание схемы: %v", err)
	}

	return pool
}

func TestCreateShortenedLink_Integration(t *testing.T) {
	pool := setupTestDB(t)

	h := New(NewPGStorage(pool), "http://localhost:1234/")

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
	h := New(NewPGStorage(pool), "http://localhost:1234/")

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

func TestRegisterAndLogin_Integration(t *testing.T) {
	pool := setupTestDB(t)
	h := New(NewPGStorage(pool), "http://localhost:1234/")

	// Регистрация
	reg := strings.NewReader(`{"first_name":"Иван","last_name":"Иванов","email":"ivan@yandex.ru","password":"secret123"}`)
	regReq := httptest.NewRequest(http.MethodPost, "/create_account", reg)
	regW := httptest.NewRecorder()
	h.CreateNewUserAccount(regW, regReq)
	if regW.Code != http.StatusCreated {
		t.Fatalf("регистрация: код %d, тело %s", regW.Code, regW.Body.String())
	}

	// Успешный вход
	okBody := strings.NewReader(`{"email":"ivan@yandex.ru","password":"secret123"}`)
	okReq := httptest.NewRequest(http.MethodPost, "/login", okBody)
	okW := httptest.NewRecorder()
	h.LoginInAccount(okW, okReq)
	if okW.Code != http.StatusOK {
		t.Fatalf("вход: код %d, тело %s", okW.Code, okW.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(okW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("распарсить ответ: %v", err)
	}
	if resp["first_name"] != "Иван" {
		t.Errorf("first_name = %q, ожидалось Иван", resp["first_name"])
	}

	// Неверный пароль → 401
	badBody := strings.NewReader(`{"email":"ivan@yandex.ru","password":"wrong"}`)
	badReq := httptest.NewRequest(http.MethodPost, "/login", badBody)
	badW := httptest.NewRecorder()
	h.LoginInAccount(badW, badReq)
	if badW.Code != http.StatusUnauthorized {
		t.Errorf("неверный пароль: код %d, ожидался 401", badW.Code)
	}

	// Неизвестный email → тот же 401 (не раскрываем существование аккаунта)
	unknownBody := strings.NewReader(`{"email":"nobody@yandex.ru","password":"secret123"}`)
	unknownReq := httptest.NewRequest(http.MethodPost, "/login", unknownBody)
	unknownW := httptest.NewRecorder()
	h.LoginInAccount(unknownW, unknownReq)
	if unknownW.Code != http.StatusUnauthorized {
		t.Errorf("неизвестный email: код %d, ожидался 401", unknownW.Code)
	}
}
