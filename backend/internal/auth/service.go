package auth

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/ratelimit"
)

// Options — настройки сервиса аутентификации.
type Options struct {
	RefreshTTL  time.Duration
	LoginLimit  int
	LoginWindow time.Duration
}

// Session — результат логина/refresh: access-токен + данные для cookie.
type Session struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	AccessExpiresIn  int
	RefreshToken     string // открытый токен, только для Set-Cookie
	RefreshExpiresAt time.Time
	User             *User
}

// Service реализует сценарии AUTH-1..AUTH-6: вход, ротацию refresh, выход,
// профиль и смену пароля.
type Service struct {
	repo         Repository
	tokens       *TokenManager
	clock        platform.Clock
	refreshTTL   time.Duration
	loginLimiter *ratelimit.Limiter
	logger       *slog.Logger
}

// NewService создаёт сервис. loginLimiter — общий in-memory лимитер неудачных
// входов (10/10 мин на login+IP, AUTH-3).
func NewService(repo Repository, tokens *TokenManager, clock platform.Clock, logger *slog.Logger, opts Options) *Service {
	if clock == nil {
		clock = platform.RealClock{}
	}
	if opts.RefreshTTL <= 0 {
		opts.RefreshTTL = 7 * 24 * time.Hour
	}
	if opts.LoginLimit <= 0 {
		opts.LoginLimit = 10
	}
	if opts.LoginWindow <= 0 {
		opts.LoginWindow = 10 * time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:         repo,
		tokens:       tokens,
		clock:        clock,
		refreshTTL:   opts.RefreshTTL,
		loginLimiter: ratelimit.New(opts.LoginLimit, opts.LoginWindow, clock),
		logger:       logger,
	}
}

// LoginLimiter отдаёт лимитер (для тестов/диагностики).
func (s *Service) LoginLimiter() *ratelimit.Limiter { return s.loginLimiter }

// Login проверяет логин/пароль (AUTH-1). Неверные данные дают одинаковый
// 401 INVALID_CREDENTIALS независимо от существования логина.
func (s *Service) Login(ctx context.Context, login, password, ip string) (*Session, error) {
	login = strings.TrimSpace(login)
	if login == "" || password == "" {
		return nil, apierr.Validation("Укажите логин и пароль", map[string]any{
			"login":    login == "",
			"password": password == "",
		})
	}

	// AUTH-3: лимит на неудачные попытки по паре login+IP. Сначала Peek —
	// успешный вход не должен расходовать лимит.
	limitKey := strings.ToLower(login) + "|" + ip
	if allowed, retryAfter := s.loginLimiter.Peek(limitKey); !allowed {
		return nil, apierr.TooManyRequests(ratelimit.RetryAfterSeconds(retryAfter))
	}

	user, err := s.repo.UserByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			s.rejectLogin(limitKey)
			return nil, invalidCredentials()
		}
		return nil, apierr.Internal(err)
	}
	// Заблокированный аккаунт получает 401 с тем же кодом, чтобы не раскрывать
	// состояние аккаунта (AUTH-6, NFR-2).
	if !user.IsActive {
		s.rejectLogin(limitKey)
		return nil, invalidCredentials()
	}

	ok, err := VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	if !ok {
		s.rejectLogin(limitKey)
		return nil, invalidCredentials()
	}
	s.loginLimiter.Reset(limitKey)

	return s.newSession(ctx, user)
}

// Refresh ротирует refresh-токен (AUTH-2): старый отзывается, выпускается новый.
// Повторное использование отозванного токена отзывает всю цепочку.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*Session, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, invalidRefreshToken()
	}
	hash := HashToken(refreshToken)
	stored, err := s.repo.RefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, invalidRefreshToken()
		}
		return nil, apierr.Internal(err)
	}

	now := s.clock.Now()
	if stored.RevokedAt != nil {
		// Повтор уже использованного токена — вероятная кража cookie: отзываем
		// всю цепочку потомков, пользователь войдёт заново (AUTH-2).
		if chainErr := s.repo.RevokeRefreshTokenChain(ctx, stored.ID); chainErr != nil {
			s.logger.Error("auth: не удалось отозвать цепочку refresh-токенов",
				"error", chainErr, "user_id", stored.UserID)
		}
		s.logger.Warn("auth: повторное использование refresh-токена", "user_id", stored.UserID)
		return nil, invalidRefreshToken()
	}
	if !now.Before(stored.ExpiresAt) {
		if revokeErr := s.repo.RevokeRefreshToken(ctx, stored.ID); revokeErr != nil {
			s.logger.Error("auth: не удалось отозвать истёкший refresh-токен", "error", revokeErr)
		}
		return nil, invalidRefreshToken()
	}

	user, err := s.repo.UserByID(ctx, stored.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			_ = s.repo.RevokeAllRefreshTokens(ctx, stored.UserID, "")
			return nil, invalidRefreshToken()
		}
		return nil, apierr.Internal(err)
	}
	// AUTH-6: блокировка действует не позднее следующего запроса.
	if !user.IsActive {
		_ = s.repo.RevokeAllRefreshTokens(ctx, user.ID, "")
		return nil, invalidRefreshToken()
	}

	plain, newHash, err := NewRefreshToken()
	if err != nil {
		return nil, apierr.Internal(err)
	}
	expiresAt := now.Add(s.refreshTTL)
	if _, err := s.repo.RotateRefreshToken(ctx, stored.ID, user.ID, newHash, expiresAt); err != nil {
		if errors.Is(err, ErrRefreshReuse) {
			_ = s.repo.RevokeRefreshTokenChain(ctx, stored.ID)
			return nil, invalidRefreshToken()
		}
		return nil, apierr.Internal(err)
	}

	access, accessExpiresAt, err := s.tokens.Issue(user)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return &Session{
		AccessToken:      access,
		AccessExpiresAt:  accessExpiresAt,
		AccessExpiresIn:  int(s.tokens.TTL().Seconds()),
		RefreshToken:     plain,
		RefreshExpiresAt: expiresAt,
		User:             user,
	}, nil
}

// Logout отзывает refresh-токен из cookie (идемпотентно).
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil
	}
	stored, err := s.repo.RefreshTokenByHash(ctx, HashToken(refreshToken))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return apierr.Internal(err)
	}
	if err := s.repo.RevokeRefreshToken(ctx, stored.ID); err != nil {
		return apierr.Internal(err)
	}
	return nil
}

// Me возвращает аккаунт по id с проверкой активности (AUTH-6).
func (s *Service) Me(ctx context.Context, userID int64) (*User, error) {
	user, err := s.repo.UserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация")
		}
		return nil, apierr.Internal(err)
	}
	if !user.IsActive {
		return nil, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация")
	}
	return user, nil
}

// ChangePassword меняет свой пароль (AUTH-5): проверяет текущий, требует новый
// ≥8 символов, снимает must_change_password и отзывает прочие refresh-токены.
func (s *Service) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword, currentRefreshToken string) error {
	if err := ValidatePassword(newPassword); err != nil {
		return apierr.Validation(err.Error(), map[string]any{"new_password": err.Error()})
	}
	if currentPassword == newPassword {
		return apierr.Validation("Новый пароль не должен совпадать с текущим",
			map[string]any{"new_password": "должен отличаться от текущего"})
	}

	user, err := s.repo.UserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация")
		}
		return apierr.Internal(err)
	}
	ok, err := VerifyPassword(currentPassword, user.PasswordHash)
	if err != nil {
		return apierr.Internal(err)
	}
	if !ok {
		return apierr.New(422, apierr.CodeInvalidCurrentPassword, "Неверный текущий пароль").
			WithDetail("current_password", "неверный пароль")
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return apierr.Internal(err)
	}
	if err := s.repo.UpdatePassword(ctx, userID, hash, false); err != nil {
		return apierr.Internal(err)
	}

	// Оставляем текущую сессию (по cookie), остальные устройства разлогиниваем.
	keepHash := ""
	if currentRefreshToken != "" {
		keepHash = HashToken(currentRefreshToken)
	}
	if err := s.repo.RevokeAllRefreshTokens(ctx, userID, keepHash); err != nil {
		s.logger.Error("auth: не удалось отозвать прочие refresh-токены", "error", err, "user_id", userID)
	}
	return nil
}

func (s *Service) newSession(ctx context.Context, user *User) (*Session, error) {
	access, accessExpiresAt, err := s.tokens.Issue(user)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	plain, hash, err := NewRefreshToken()
	if err != nil {
		return nil, apierr.Internal(err)
	}
	expiresAt := s.clock.Now().Add(s.refreshTTL)
	if _, err := s.repo.CreateRefreshToken(ctx, user.ID, hash, expiresAt); err != nil {
		return nil, apierr.Internal(err)
	}
	return &Session{
		AccessToken:      access,
		AccessExpiresAt:  accessExpiresAt,
		AccessExpiresIn:  int(s.tokens.TTL().Seconds()),
		RefreshToken:     plain,
		RefreshExpiresAt: expiresAt,
		User:             user,
	}, nil
}

func (s *Service) rejectLogin(limitKey string) {
	// Расходуем токен лимита на неудачную попытку (AUTH-3).
	_, _ = s.loginLimiter.Take(limitKey)
}

// RefreshTTL возвращает срок жизни refresh-cookie (для Max-Age).
func (s *Service) RefreshTTL() time.Duration { return s.refreshTTL }

func invalidCredentials() error {
	return apierr.Unauthorized(apierr.CodeInvalidCredentials, "Неверный логин или пароль")
}

func invalidRefreshToken() error {
	return apierr.Unauthorized(apierr.CodeInvalidRefreshToken, "Сессия недействительна, войдите снова")
}
