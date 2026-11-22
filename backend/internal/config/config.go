// Package config загружает и валидирует конфигурацию из переменных окружения.
// Секреты (JWT_SECRET, пароль сидинга, VAPID) приходят только из env и никогда
// не попадают в репозиторий (NFR-2, AGENTS.md).
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Минимальная длина JWT-секрета (NFR-2).
const minJWTSecretLen = 32

// Config — полная конфигурация приложения.
type Config struct {
	AppEnv  string // development | production | test
	Version string

	HTTPAddr string
	Port     int

	DatabaseURL       string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	JWTIssuer       string

	Timezone string
	Location *time.Location

	CORSOrigins []string
	TrustProxy  bool

	UploadDir      string
	MaxUploadBytes int64

	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string

	SeedAdminLogin    string
	SeedAdminPassword string
	SeedAdminFullName string

	OpenAPIPath     string
	ShutdownTimeout time.Duration
	LogLevel        slog.Level
}

// IsProduction сообщает, работаем ли мы в продакшене (влияет на Secure-cookie,
// обязательность VAPID и строгость секретов).
func (c *Config) IsProduction() bool { return c.AppEnv == "production" }

// DSN возвращает DSN для go-sql-driver/mysql.
// multiStatements=true нужен только для миграций (golang-migrate выполняет файл
// миграции целиком), для рабочего пула соединений флаг выключен.
func (c *Config) DSN(multiStatements bool) string {
	raw := strings.TrimSpace(c.DatabaseURL)
	if strings.HasPrefix(raw, "mysql://") || strings.HasPrefix(raw, "mariadb://") {
		return dsnFromURL(raw, multiStatements)
	}
	return raw
}

// dsnFromURL преобразует mysql://-URL в DSN go-sql-driver. Поддерживаются две
// формы: mysql://user:pass@host:3306/db и mysql://user:pass@tcp(host:3306)/db
// (вторая привычна в docker-compose, но не является валидным URL из-за скобок,
// поэтому разбираем вручную, а не через net/url).
func dsnFromURL(raw string, multiStatements bool) string {
	body := strings.TrimPrefix(raw, "mysql://")
	body = strings.TrimPrefix(body, "mariadb://")

	var rawQuery string
	if i := strings.Index(body, "?"); i >= 0 {
		rawQuery = body[i+1:]
		body = body[:i]
	}

	credentials := ""
	address := body
	if at := strings.LastIndex(body, "@"); at >= 0 {
		credentials = body[:at]
		address = body[at+1:]
	}
	credentials = decodeCredentials(credentials)

	networkAddr := address
	dbName := ""
	switch {
	case strings.HasPrefix(address, "tcp("):
		if end := strings.Index(address, ")"); end > len("tcp(") {
			networkAddr = address[len("tcp("):end]
			dbName = strings.TrimPrefix(address[end+1:], "/")
		}
	default:
		if slash := strings.Index(address, "/"); slash >= 0 {
			networkAddr = address[:slash]
			dbName = address[slash+1:]
		}
		if !strings.Contains(networkAddr, ":") {
			networkAddr += ":3306"
		}
	}

	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		q = url.Values{}
	}
	setDefault := func(key, value string) {
		if q.Get(key) == "" {
			q.Set(key, value)
		}
	}
	setDefault("parseTime", "true")
	setDefault("loc", "UTC")
	setDefault("charset", "utf8mb4")
	setDefault("collation", "utf8mb4_0900_ai_ci")
	setDefault("timeout", "5s")
	setDefault("readTimeout", "30s")
	setDefault("writeTimeout", "30s")
	if multiStatements {
		q.Set("multiStatements", "true")
	} else {
		q.Del("multiStatements")
	}

	return fmt.Sprintf("%s@tcp(%s)/%s?%s", credentials, networkAddr, dbName, q.Encode())
}

// decodeCredentials раскодирует percent-encoding в логине/пароле, сохраняя
// разделитель ':' между ними.
func decodeCredentials(credentials string) string {
	if credentials == "" {
		return ""
	}
	userPart, passPart, hasPass := strings.Cut(credentials, ":")
	if decoded, err := url.PathUnescape(userPart); err == nil {
		userPart = decoded
	}
	if !hasPass {
		return userPart
	}
	if decoded, err := url.PathUnescape(passPart); err == nil {
		passPart = decoded
	}
	return userPart + ":" + passPart
}

// Load читает окружение процесса.
func Load() (*Config, error) {
	return LoadFrom(lookupEnv)
}

// lookupEnv — источник значений по умолчанию (реальное окружение процесса).
func lookupEnv(key string) string { return os.Getenv(key) }

// LoadFrom парсит конфигурацию из переданной функции getenv. Отдельная функция
// позволяет тестировать валидацию без изменения окружения процесса.
func LoadFrom(getenv func(string) string) (*Config, error) {
	cfg := &Config{
		AppEnv:   def(getenv("APP_ENV"), "development"),
		Version:  def(getenv("APP_VERSION"), "0.1.0"),
		LogLevel: parseLogLevel(getenv("LOG_LEVEL")),
	}

	port, err := parseInt(def(getenv("PORT"), "8080"), "PORT")
	if err != nil {
		return nil, err
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("PORT: значение %d вне диапазона 1..65535", port)
	}
	cfg.Port = port
	cfg.HTTPAddr = def(getenv("HTTP_ADDR"), fmt.Sprintf(":%d", port))

	cfg.DatabaseURL = strings.TrimSpace(getenv("DATABASE_URL"))
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL обязателен (например, mysql://user:pass@tcp(mysql:3306)/polar_star)")
	}

	cfg.JWTSecret = getenv("JWT_SECRET")
	if len(cfg.JWTSecret) < minJWTSecretLen {
		return nil, fmt.Errorf("JWT_SECRET должен быть не короче %d символов (NFR-2)", minJWTSecretLen)
	}
	cfg.JWTIssuer = def(getenv("JWT_ISSUER"), "polar-star-service")

	cfg.AccessTokenTTL, err = parseDuration(def(getenv("ACCESS_TOKEN_TTL"), "15m"), "ACCESS_TOKEN_TTL")
	if err != nil {
		return nil, err
	}
	cfg.RefreshTokenTTL, err = parseDuration(def(getenv("REFRESH_TOKEN_TTL"), "168h"), "REFRESH_TOKEN_TTL")
	if err != nil {
		return nil, err
	}

	cfg.Timezone = def(getenv("TZ"), def(getenv("TIMEZONE"), "Asia/Yekaterinburg"))
	// CAL-2: часовой пояс валидируется как IANA-идентификатор.
	cfg.Location, err = time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("TZ: неизвестный часовой пояс %q: %w", cfg.Timezone, err)
	}

	cfg.CORSOrigins = splitCSV(getenv("CORS_ORIGIN"))
	if len(cfg.CORSOrigins) == 0 {
		cfg.CORSOrigins = []string{"http://localhost:5173", "http://localhost:8081"}
	}
	cfg.TrustProxy = parseBool(def(getenv("TRUST_PROXY"), "true"))

	cfg.UploadDir = def(getenv("UPLOAD_DIR"), "./data/uploads")
	cfg.MaxUploadBytes, err = parseInt64(def(getenv("MAX_UPLOAD_BYTES"), "209715200"), "MAX_UPLOAD_BYTES")
	if err != nil {
		return nil, err
	}

	cfg.VAPIDPublicKey = strings.TrimSpace(getenv("VAPID_PUBLIC_KEY"))
	cfg.VAPIDPrivateKey = strings.TrimSpace(getenv("VAPID_PRIVATE_KEY"))
	cfg.VAPIDSubject = def(getenv("VAPID_SUBJECT"), "mailto:admin@example.com")
	if cfg.IsProduction() && (cfg.VAPIDPublicKey == "" || cfg.VAPIDPrivateKey == "") {
		return nil, fmt.Errorf("VAPID_PUBLIC_KEY и VAPID_PRIVATE_KEY обязательны в production (NOTIF-1)")
	}

	cfg.SeedAdminLogin = def(getenv("SEED_ADMIN_LOGIN"), "admin")
	cfg.SeedAdminPassword = getenv("SEED_ADMIN_PASSWORD")
	cfg.SeedAdminFullName = def(getenv("SEED_ADMIN_FULL_NAME"), "Администратор")
	if cfg.SeedAdminPassword != "" && len([]rune(cfg.SeedAdminPassword)) < 8 {
		return nil, fmt.Errorf("SEED_ADMIN_PASSWORD: временный пароль должен быть не короче 8 символов (AUTH-4)")
	}

	cfg.OpenAPIPath = def(getenv("OPENAPI_PATH"), "openapi/openapi.yaml")

	cfg.ShutdownTimeout, err = parseDuration(def(getenv("SHUTDOWN_TIMEOUT"), "15s"), "SHUTDOWN_TIMEOUT")
	if err != nil {
		return nil, err
	}

	cfg.DBMaxOpenConns, err = parseInt(def(getenv("DB_MAX_OPEN_CONNS"), "25"), "DB_MAX_OPEN_CONNS")
	if err != nil {
		return nil, err
	}
	cfg.DBMaxIdleConns, err = parseInt(def(getenv("DB_MAX_IDLE_CONNS"), "10"), "DB_MAX_IDLE_CONNS")
	if err != nil {
		return nil, err
	}
	cfg.DBConnMaxLifetime, err = parseDuration(def(getenv("DB_CONN_MAX_LIFETIME"), "3m"), "DB_CONN_MAX_LIFETIME")
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// ValidateAddr проверяет, что адрес слушается (для раннего падения при ошибке).
func (c *Config) ValidateAddr() error {
	host, port, err := net.SplitHostPort(c.HTTPAddr)
	if err != nil {
		return fmt.Errorf("HTTP_ADDR %q: %w", c.HTTPAddr, err)
	}
	if host != "" && net.ParseIP(host) == nil && host != "localhost" {
		return fmt.Errorf("HTTP_ADDR: некорректный хост %q", host)
	}
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("HTTP_ADDR: некорректный порт %q", port)
	}
	return nil
}

func def(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func parseInt(value, name string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: ожидается целое число, получено %q", name, value)
	}
	return n, nil
}

func parseInt64(value, name string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: ожидается целое число, получено %q", name, value)
	}
	return n, nil
}

func parseDuration(value, name string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: ожидается длительность (15m, 168h), получено %q", name, value)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s: длительность должна быть положительной", name)
	}
	return d, nil
}
