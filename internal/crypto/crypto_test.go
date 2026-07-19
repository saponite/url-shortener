package crypto

import (
	"strings"
	"testing"
)

// go test ./... -v

func TestHashPassword_ReturnsValidFormat(t *testing.T) {
	hash, err := HashPassword("mySecretPassword123")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	parts := strings.Split(hash, "$")
	if len(parts) != 4 {
		t.Fatalf("ожидалось 4 части в хеше, получено %d: %s", len(parts), hash)
	}

	if parts[0] != "argon2id" {
		t.Errorf("ожидался алгоритм argon2id, получен %s", parts[0])
	}
}

func TestHashPassword_DifferentSaltsForSamePassword(t *testing.T) {
	password := "samePassword"

	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	if hash1 == hash2 {
		t.Error("два хеша одного пароля совпали — соль не рандомизируется")
	}
}

func TestVerifyPassword_CorrectPassword(t *testing.T) {
	password := "correctHorseBatteryStaple"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("неожиданная ошибка при хешировании: %v", err)
	}

	ok, err := VerifyPassword(password, hash)
	if err != nil {
		t.Fatalf("неожиданная ошибка при проверке: %v", err)
	}
	if !ok {
		t.Error("правильный пароль не прошёл проверку")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hash, err := HashPassword("correctPassword")
	if err != nil {
		t.Fatalf("неожиданная ошибка при хешировании: %v", err)
	}

	ok, err := VerifyPassword("wrongPassword", hash)
	if err != nil {
		t.Fatalf("неожиданная ошибка при проверке: %v", err)
	}
	if ok {
		t.Error("неправильный пароль прошёл проверку")
	}
}

func TestVerifyPassword_EmptyPassword(t *testing.T) {
	hash, err := HashPassword("somePassword")
	if err != nil {
		t.Fatalf("неожиданная ошибка при хешировании: %v", err)
	}

	ok, err := VerifyPassword("", hash)
	if err != nil {
		t.Fatalf("неожиданная ошибка при проверке: %v", err)
	}
	if ok {
		t.Error("пустой пароль не должен проходить проверку")
	}
}

func TestVerifyPassword_InvalidHashFormat(t *testing.T) {
	testCases := []struct {
		name string
		hash string
	}{
		{"пустая строка", ""},
		{"случайный текст", "not-a-valid-hash"},
		{"неверный алгоритм", "bcrypt$time=1,memory=65536,threads=4$c2FsdA$aGFzaA"},
		{"недостаточно частей", "argon2id$time=1,memory=65536,threads=4"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := VerifyPassword("anyPassword", tc.hash)
			if err == nil {
				t.Error("ожидалась ошибка при разборе некорректного хеша, но её не было")
			}
			if ok {
				t.Error("VerifyPassword не должен возвращать true при ошибке разбора")
			}
		})
	}
}

func TestVerifyPassword_CorruptedSaltOrHash(t *testing.T) {
	hash, err := HashPassword("somePassword")
	if err != nil {
		t.Fatalf("неожиданная ошибка при хешировании: %v", err)
	}

	parts := strings.Split(hash, "$")
	parts[2] = "!!!not-valid-base64!!!"
	corrupted := strings.Join(parts, "$")

	_, err = VerifyPassword("somePassword", corrupted)
	if err == nil {
		t.Error("ожидалась ошибка при декодировании повреждённой соли")
	}
}

func TestHashPassword_EmptyPasswordStillProducesHash(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if hash == "" {
		t.Error("ожидался непустой хеш даже для пустого пароля")
	}
}

func TestVerifyPassword_TamperedHashSegment(t *testing.T) {
	hash, err := HashPassword("somePassword")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	// Подменяем сам хеш на валидный base64, но не тот. Формат корректен,
	// ошибки разбора нет — но пароль не должен пройти проверку.
	parts := strings.Split(hash, "$")
	parts[3] = "AAAA"
	tampered := strings.Join(parts, "$")

	ok, err := VerifyPassword("somePassword", tampered)
	if err != nil {
		t.Fatalf("не ожидали ошибку разбора (base64 валиден): %v", err)
	}
	if ok {
		t.Error("подменённый хеш не должен проходить проверку")
	}
}

func TestHashPassword_UnicodePassword(t *testing.T) {
	password := "пароль123!@#"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("неожиданная ошибка при хешировании unicode-пароля: %v", err)
	}

	ok, err := VerifyPassword(password, hash)
	if err != nil {
		t.Fatalf("неожиданная ошибка при проверке: %v", err)
	}
	if !ok {
		t.Error("unicode-пароль не прошёл проверку")
	}
}
