package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saponite/url-shortner/internal/crypto"
)

// go test -run handler_test.go

type mockStorage struct {
	getByOriginalURLFunc func(ctx context.Context, url string) (*Link, error)
	getByCodeFunc        func(ctx context.Context, code string) (*Link, error)
	createFunc           func(ctx context.Context, code, url string) (string, error)
	loginFunc            func(ctx context.Context, email string) (*User, error)
}

func (m *mockStorage) GetByOriginalURL(ctx context.Context, url string) (*Link, error) {
	return m.getByOriginalURLFunc(ctx, url)
}

func (m *mockStorage) GetByCode(ctx context.Context, code string) (*Link, error) {
	return m.getByCodeFunc(ctx, code)
}

func (m *mockStorage) CreateNewShortLink(ctx context.Context, code, url string) (string, error) {
	return m.createFunc(ctx, code, url)
}

// UserStorage — заглушки; конкретное поведение задаётся через *Func поля.
func (m *mockStorage) CreateNewUserAccount(ctx context.Context, firstName, lastName, email, password string) error {
	return nil
}

func (m *mockStorage) UpdatePassword(ctx context.Context, email, password string) error {
	return nil
}

func (m *mockStorage) UpdateFirstNameAndLastName(ctx context.Context, userId uuid.UUID, firstName, lastName string) error {
	return nil
}

func (m *mockStorage) UpdateEmail(ctx context.Context, userId uuid.UUID, email string) error {
	return nil
}

func (m *mockStorage) LoginInAccount(ctx context.Context, email string) (*User, error) {
	return m.loginFunc(ctx, email)
}

func TestCreateDuplicateURLReturnsExisting(t *testing.T) {
	existing := &Link{
		OriginalURL: "https://github.com",
		ShortCode:   "abc1234",
		CreatedAt:   time.Now(),
	}

	storage := &mockStorage{
		getByOriginalURLFunc: func(ctx context.Context, url string) (*Link, error) {
			return existing, nil
		},
	}

	h := New(storage, "http://test/")

	body := strings.NewReader(`{"original_url": "https://github.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/create", body)
	w := httptest.NewRecorder()

	h.CreateShortenedLink(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("дубль должен возвращать 200, получили %d", w.Code)
	}

	var resp LinkResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ShortURL != "http://test/abc1234" {
		t.Errorf("ShortURL = %q, ожидалось http://test/abc1234", resp.ShortURL)
	}
}

func TestCreateStorageErrorReturns500(t *testing.T) {
	storage := &mockStorage{
		getByOriginalURLFunc: func(ctx context.Context, url string) (*Link, error) {
			return nil, errors.New("connection refused")
		},
	}

	h := New(storage, "http://test/")

	body := strings.NewReader(`{"original_url": "https://github.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/create", body)
	w := httptest.NewRecorder()
	h.CreateShortenedLink(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("ожидался 500, получили %d", w.Code)
	}
}

func TestCreateInvalidURLReturns400(t *testing.T) {
	h := New(&mockStorage{}, "http://test/")

	body := strings.NewReader(`{"original_url": "не урл вовсе"}`)
	req := httptest.NewRequest(http.MethodPost, "/create", body)
	w := httptest.NewRecorder()
	h.CreateShortenedLink(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ожидался 400, получили %d", w.Code)
	}
}

func TestStripPrefix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"https + www", "https://www.example.com", "example.com"},
		{"http + www", "http://www.example.com", "example.com"},
		{"https без www", "https://example.com", "example.com"},
		{"http без www", "http://example.com", "example.com"},
		{"ftp", "ftp://example.com", "example.com"},
		{"только www", "www.example.com", "example.com"},
		{"WWW в верхнем регистре", "WWW.example.com", "example.com"},
		{"смешанный регистр схемы", "HTTPS://Example.com", "Example.com"},
		{"без префиксов", "example.com", "example.com"},
		{"с путём", "https://example.com/foo/bar", "example.com/foo/bar"},
		{"с путём и query", "https://www.example.com/foo?x=1", "example.com/foo?x=1"},
		{"пустая строка", "", ""},
		{"только схема", "https://", ""},
		{"схема + www без хоста", "https://www.", ""},
		{"wwww. — это не www.", "wwww.example.com", "wwww.example.com"},
		{"www без точки", "wwwexample.com", "wwwexample.com"},
		{"порт в URL", "https://example.com:8080/x", "example.com:8080/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripPrefix(tt.in)
			if got != tt.want {
				t.Errorf("stripPrefix(%q) = %q, ожидалось %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsValidHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		// валидные
		{"простой домен", "example.com", true},
		{"поддомен", "sub.example.com", true},
		{"глубокий поддомен", "a.b.c.example.com", true},
		{"с портом", "example.com:8080", true},
		{"с дефисом в метке", "my-site.example.com", true},
		{"цифры в метке", "site123.example.com", true},

		// невалидные
		{"без TLD", "example", false},
		{"пустая строка", "", false},
		{"ведущая точка", ".example.com", false},
		{"завершающая точка", "example.com.", false},
		{"дефис в начале метки", "-example.com", false},
		{"дефис в конце метки", "example-.com", false},
		{"однобуквенный TLD", "example.c", false},
		{"числовой TLD", "example.123", false},
		{"IP-адрес", "127.0.0.1", false},
		{"подчёркивание", "ex_ample.com", false},
		{"пробел", "example .com", false},
		{"кириллический homograph", "githubа.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidHost(tt.host)
			if got != tt.want {
				t.Errorf("isValidHost(%q) = %v, ожидалось %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestGenerateShortCode(t *testing.T) {
	t.Run("длина равна 7", func(t *testing.T) {
		code := generateShortCode([]byte("example.com"), 0)
		if len(code) != 7 {
			t.Errorf("len(code) = %d, ожидалось 7", len(code))
		}
	})

	t.Run("детерминированность", func(t *testing.T) {
		a := generateShortCode([]byte("example.com/foo"), 3)
		b := generateShortCode([]byte("example.com/foo"), 3)
		if a != b {
			t.Errorf("одинаковый ввод дал разные коды: %q vs %q", a, b)
		}
	})

	t.Run("разные salt — разные коды", func(t *testing.T) {
		a := generateShortCode([]byte("example.com"), 0)
		b := generateShortCode([]byte("example.com"), 1)
		if a == b {
			t.Errorf("разные salt дали одинаковый код: %q", a)
		}
	})

	t.Run("разные URL — разные коды", func(t *testing.T) {
		a := generateShortCode([]byte("example.com"), 0)
		b := generateShortCode([]byte("example.org"), 0)
		if a == b {
			t.Errorf("разные URL дали одинаковый код: %q", a)
		}
	})

	t.Run("все символы из алфавита", func(t *testing.T) {
		inputs := []string{
			"example.com",
			"github.com/user/repo",
			"очень-длинный-урл.рф/с/путём?q=1&z=2",
			"",
			"a",
		}
		for _, in := range inputs {
			for salt := 0; salt < 10; salt++ {
				code := generateShortCode([]byte(in), salt)
				for _, c := range code {
					if !strings.ContainsRune(alphabet, c) {
						t.Errorf("код %q содержит символ %q вне алфавита (вход %q, salt %d)",
							code, c, in, salt)
					}
				}
			}
		}
	})

	t.Run("входной слайс не мутируется", func(t *testing.T) {
		original := []byte("example.com")
		snapshot := append([]byte(nil), original...)
		_ = generateShortCode(original, 5)
		if string(original) != string(snapshot) {
			t.Errorf("входной слайс был изменён: было %q, стало %q", snapshot, original)
		}
	})
}

func TestLoginInAccount_Success(t *testing.T) {
	hash, err := crypto.HashPassword("secret123")
	if err != nil {
		t.Fatalf("хеширование: %v", err)
	}

	storage := &mockStorage{
		loginFunc: func(ctx context.Context, email string) (*User, error) {
			return &User{FirstName: "Иван", LastName: "Иванов", Email: email, Password: hash}, nil
		},
	}
	h := New(storage, "http://test/")

	body := strings.NewReader(`{"email":"ivan@yandex.ru","password":"secret123"}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	w := httptest.NewRecorder()
	h.LoginInAccount(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получили %d (тело: %s)", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("распарсить ответ: %v", err)
	}
	if resp["first_name"] != "Иван" {
		t.Errorf("first_name = %q, ожидалось Иван", resp["first_name"])
	}
	if resp["email"] != "ivan@yandex.ru" {
		t.Errorf("email = %q, ожидалось ivan@yandex.ru", resp["email"])
	}
}

func TestLoginInAccount_WrongPassword(t *testing.T) {
	hash, _ := crypto.HashPassword("secret123")

	storage := &mockStorage{
		loginFunc: func(ctx context.Context, email string) (*User, error) {
			return &User{Email: email, Password: hash}, nil
		},
	}
	h := New(storage, "http://test/")

	body := strings.NewReader(`{"email":"ivan@yandex.ru","password":"неверный"}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	w := httptest.NewRecorder()
	h.LoginInAccount(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("ожидался 401 при неверном пароле, получили %d", w.Code)
	}
}

func TestLoginInAccount_UserNotFound(t *testing.T) {
	storage := &mockStorage{
		loginFunc: func(ctx context.Context, email string) (*User, error) {
			return nil, ErrUserNotFound
		},
	}
	h := New(storage, "http://test/")

	body := strings.NewReader(`{"email":"nobody@yandex.ru","password":"secret123"}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	w := httptest.NewRecorder()
	h.LoginInAccount(w, req)

	// Тот же 401, что и при неверном пароле — не раскрываем, есть ли такой email.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("ожидался 401 для несуществующего email, получили %d", w.Code)
	}
}

func TestLoginInAccount_MissingFields(t *testing.T) {
	h := New(&mockStorage{}, "http://test/")

	body := strings.NewReader(`{"email":"","password":""}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	w := httptest.NewRecorder()
	h.LoginInAccount(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ожидался 400 при пустых полях, получили %d", w.Code)
	}
}

func TestLoginInAccount_InvalidJSON(t *testing.T) {
	h := New(&mockStorage{}, "http://test/")

	body := strings.NewReader(`{ сломанный json`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	w := httptest.NewRecorder()
	h.LoginInAccount(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ожидался 400 при битом JSON, получили %d", w.Code)
	}
}
