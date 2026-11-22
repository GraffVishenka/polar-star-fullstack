package auth

import (
	"strings"
	"testing"
)

// AUTH-1: argon2id с параметрами из ТЗ; хеш проверяется и не содержит пароля.
func TestHashAndVerifyPassword(t *testing.T) {
	password := "Sup3r-secret!"
	encoded, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=1$") {
		t.Fatalf("неожиданный формат хеша: %s", encoded)
	}
	if strings.Contains(encoded, password) {
		t.Fatal("хеш не должен содержать пароль")
	}

	ok, err := VerifyPassword(password, encoded)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("верный пароль должен проходить проверку")
	}

	ok, err = VerifyPassword("wrong-password", encoded)
	if err != nil {
		t.Fatalf("VerifyPassword(wrong): %v", err)
	}
	if ok {
		t.Fatal("неверный пароль не должен проходить проверку")
	}
}

// Два хеша одного пароля различаются из-за случайной соли.
func TestHashPasswordUniqueSalt(t *testing.T) {
	first, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	second, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if first == second {
		t.Fatal("хеши одного пароля не должны совпадать")
	}
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	for _, encoded := range []string{"", "plain", "$argon2id$v=19$m=1,t=1,p=1$not-base64!$not-base64!", "$bcrypt$..."} {
		if _, err := VerifyPassword("x", encoded); err == nil {
			t.Fatalf("ожидалась ошибка формата для %q", encoded)
		}
	}
}

// AUTH-4/AUTH-5: минимальная длина временного/нового пароля — 8 символов.
func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("1234567"); err == nil {
		t.Fatal("пароль из 7 символов должен отклоняться")
	}
	if err := ValidatePassword("12345678"); err != nil {
		t.Fatalf("пароль из 8 символов должен приниматься: %v", err)
	}
	if err := ValidatePassword(strings.Repeat("я", 129)); err == nil {
		t.Fatal("слишком длинный пароль должен отклоняться")
	}
}
