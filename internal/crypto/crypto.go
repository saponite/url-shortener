package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	defaultTime    = 1
	defaultMemory  = 64 * 1024 // 64 MB
	defaultThreads = 4
	defaultKeyLen  = 32
	defaultSaltLen = 16
)

// HashPassword хеширует пароль и возвращает строку вида
// argon2id$time=1,memory=65536,threads=4$<base64 salt>$<base64 hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, defaultSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("не удалось сгенерировать соль: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, defaultTime, defaultMemory, defaultThreads, defaultKeyLen)

	encoded := fmt.Sprintf(
		"argon2id$time=%d,memory=%d,threads=%d$%s$%s",
		defaultTime, defaultMemory, defaultThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)

	return encoded, nil
}

func VerifyPassword(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 4 || parts[0] != "argon2id" {
		return false, fmt.Errorf("неверный формат хеша пароля")
	}

	var time, memory uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[1], "time=%d,memory=%d,threads=%d", &time, &memory, &threads); err != nil {
		return false, fmt.Errorf("не удалось разобрать параметры хеша: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, fmt.Errorf("не удалось декодировать соль: %w", err)
	}

	storedHash, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, fmt.Errorf("не удалось декодировать хеш: %w", err)
	}

	computedHash := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(storedHash)))

	if subtle.ConstantTimeCompare(storedHash, computedHash) == 1 {
		return true, nil
	}

	return false, nil
}
