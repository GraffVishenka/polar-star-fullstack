package config_test

import (
	"strings"
	"testing"

	"github.com/graffvishenka/polar-star-service/backend/internal/config"
)

const validSecret = "0123456789abcdef0123456789abcdef"

func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func baseEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL": "mysql://user:pass@tcp(mysql:3306)/polar_star",
		"JWT_SECRET":   validSecret,
	}
}

// Значения по умолчанию соответствуют ТЗ: Asia/Yekaterinburg, порт 8080,
// access 15 минут, refresh 7 дней (AUTH-2, CAL-2).
func TestLoadFrom_Defaults(t *testing.T) {
	cfg, err := config.LoadFrom(envFrom(baseEnv()))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("Port = %d", cfg.Port)
	}
	if cfg.Timezone != "Asia/Yekaterinburg" || cfg.Location == nil {
		t.Fatalf("Timezone = %q", cfg.Timezone)
	}
	if cfg.AccessTokenTTL.String() != "15m0s" {
		t.Fatalf("AccessTokenTTL = %v", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTokenTTL.String() != "168h0m0s" {
		t.Fatalf("RefreshTokenTTL = %v", cfg.RefreshTokenTTL)
	}
	if cfg.IsProduction() {
		t.Fatal("APP_ENV по умолчанию должен быть development")
	}
}

func TestLoadFrom_JWTSecretTooShort(t *testing.T) {
	env := baseEnv()
	env["JWT_SECRET"] = "short"
	if _, err := config.LoadFrom(envFrom(env)); err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("ожидалась ошибка JWT_SECRET, получено %v", err)
	}
}

// CAL-2: неизвестный IANA-пояс → ошибка конфигурации (и 422 в API настроек).
func TestLoadFrom_InvalidTimezone(t *testing.T) {
	env := baseEnv()
	env["TZ"] = "Europe/Mordor"
	if _, err := config.LoadFrom(envFrom(env)); err == nil || !strings.Contains(err.Error(), "TZ") {
		t.Fatalf("ожидалась ошибка TZ, получено %v", err)
	}
}

func TestLoadFrom_MissingDatabaseURL(t *testing.T) {
	env := baseEnv()
	delete(env, "DATABASE_URL")
	if _, err := config.LoadFrom(envFrom(env)); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("ожидалась ошибка DATABASE_URL, получено %v", err)
	}
}

// DATABASE_URL в формате URL преобразуется в DSN go-sql-driver с UTC/parseTime.
func TestDSN_FromURL(t *testing.T) {
	cfg, err := config.LoadFrom(envFrom(baseEnv()))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	dsn := cfg.DSN(false)
	for _, want := range []string{"user:pass@tcp(mysql:3306)/polar_star", "parseTime=true", "loc=UTC", "charset=utf8mb4"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("DSN %q не содержит %q", dsn, want)
		}
	}
	if strings.Contains(dsn, "multiStatements") {
		t.Fatalf("рабочий DSN не должен содержать multiStatements: %q", dsn)
	}
	if multi := cfg.DSN(true); !strings.Contains(multi, "multiStatements=true") {
		t.Fatalf("миграционный DSN должен содержать multiStatements: %q", multi)
	}
}

// Секреты не должны попадать в репозиторий: пароль сидинга валидируется по длине.
func TestLoadFrom_SeedPassword(t *testing.T) {
	env := baseEnv()
	env["SEED_ADMIN_PASSWORD"] = "short"
	if _, err := config.LoadFrom(envFrom(env)); err == nil || !strings.Contains(err.Error(), "SEED_ADMIN_PASSWORD") {
		t.Fatalf("ожидалась ошибка SEED_ADMIN_PASSWORD, получено %v", err)
	}
}

func TestLoadFrom_ProductionRequiresVAPID(t *testing.T) {
	env := baseEnv()
	env["APP_ENV"] = "production"
	if _, err := config.LoadFrom(envFrom(env)); err == nil || !strings.Contains(err.Error(), "VAPID") {
		t.Fatalf("ожидалась ошибка VAPID, получено %v", err)
	}
}

func TestDSN_Raw(t *testing.T) {
	cfg := &config.Config{DatabaseURL: "polar:secret@tcp(db:3306)/polar_star?timeout=3s"}
	dsn := cfg.DSN(true)
	for _, want := range []string{"parseTime=true", "loc=UTC", "multiStatements=true", "timeout=3s"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("DSN %q не содержит %q", dsn, want)
		}
	}
	if strings.Contains(cfg.DSN(false), "multiStatements=true") {
		t.Fatalf("рабочий DSN не должен включать multiStatements: %q", cfg.DSN(false))
	}
}

func TestValidateAddr(t *testing.T) {
	cfg := &config.Config{HTTPAddr: ":8080"}
	if err := cfg.ValidateAddr(); err != nil {
		t.Fatalf("ValidateAddr(:8080): %v", err)
	}
	cfg.HTTPAddr = "bad-addr"
	if err := cfg.ValidateAddr(); err == nil {
		t.Fatal("ожидалась ошибка для некорректного адреса")
	}
}
