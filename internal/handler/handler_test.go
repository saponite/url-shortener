package handler

import (
	"strings"
	"testing"
)

// go test -run handler_test.go
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
