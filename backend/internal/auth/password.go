package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// AUTH-1: параметры argon2id из ТЗ — память 64 МБ, 3 итерации, parallelism 1.
const (
	argonMemory      uint32 = 64 * 1024 // KiB
	argonIterations  uint32 = 3
	argonParallelism uint8  = 1
	argonSaltLength         = 16
	argonKeyLength   uint32 = 32
)

// ErrInvalidHash — сохранённый хеш имеет неизвестный/повреждённый формат.
var ErrInvalidHash = errors.New("auth: некорректный формат хеша пароля")

// HashPassword считает argon2id-хеш в формате PHC:
// $argon2id$v=19$m=65536,t=3,p=1$<salt>$<hash>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: генерация соли: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword проверяет пароль против PHC-хеша argon2id за константное время.
// Возвращает false без ошибки при неверном пароле; ошибка — только при битом хеше.
func VerifyPassword(password, encoded string) (bool, error) {
	memory, iterations, parallelism, salt, expected, err := parseArgon2Hash(encoded)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

// parseArgon2Hash разбирает PHC-строку argon2id.
func parseArgon2Hash(encoded string) (memory, iterations uint32, parallelism uint8, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash]
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return 0, 0, 0, nil, nil, ErrInvalidHash
	}
	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return 0, 0, 0, nil, nil, ErrInvalidHash
	}
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return 0, 0, 0, nil, nil, ErrInvalidHash
	}
	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return 0, 0, 0, nil, nil, ErrInvalidHash
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return 0, 0, 0, nil, nil, ErrInvalidHash
	}
	if memory == 0 || iterations == 0 || parallelism == 0 || len(salt) == 0 || len(key) == 0 {
		return 0, 0, 0, nil, nil, ErrInvalidHash
	}
	return memory, iterations, parallelism, salt, key, nil
}

// ValidatePassword проверяет требования AUTH-4/AUTH-5: длина 8..128 символов.
func ValidatePassword(password string) error {
	n := len([]rune(password))
	if n < 8 {
		return errors.New("пароль должен содержать не менее 8 символов")
	}
	if n > 128 {
		return errors.New("пароль должен содержать не более 128 символов")
	}
	return nil
}
