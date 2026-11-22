package users

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// loginPattern — допустимые логины (контракт UserCreateRequest: 3..64 символа).
var loginPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Service реализует правила AUTH-4..AUTH-9:
//   - USER всегда с активным зданием, ADMIN — без закрепления;
//   - блокировка и понижение последнего активного ADMIN запрещены;
//   - блокировка и сброс пароля отзывают refresh-сессии;
//   - assignable: USER получает только себя, ADMIN — USER здания заявки + ADMIN.
type Service struct {
	repo    Repository
	tokens  TokenRevoker
	tickets TicketBuildingResolver
	logger  *slog.Logger
}

// NewService создаёт сервис пользователей.
func NewService(repo Repository, tokens TokenRevoker, tickets TicketBuildingResolver, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, tokens: tokens, tickets: tickets, logger: logger}
}

// List возвращает страницу пользователей (ADMIN-маршрут, AUTH-4).
func (s *Service) List(ctx context.Context, filter ListFilter) (*ListResult, error) {
	if filter.Role != "" && !filter.Role.Valid() {
		return nil, validation("role", "неизвестная роль")
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Size <= 0 {
		filter.Size = 20
	}
	if filter.Size > 100 {
		return nil, validation("size", "максимум 100")
	}
	filter.Search = strings.TrimSpace(filter.Search)
	if len([]rune(filter.Search)) > 200 {
		return nil, validation("search", "максимум 200 символов")
	}

	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return &ListResult{Items: items, Total: total, Page: filter.Page, Size: filter.Size}, nil
}

// Create создаёт пользователя (AUTH-4). Для USER обязательно активное здание;
// пароль сразу помечается как временный (must_change_password=true).
func (s *Service) Create(ctx context.Context, input CreateInput) (*auth.User, error) {
	login := strings.TrimSpace(input.Login)
	fullName := strings.TrimSpace(input.FullName)
	if err := validateLogin(login); err != nil {
		return nil, validation("login", err.Error())
	}
	if err := validateFullName(fullName); err != nil {
		return nil, validation("full_name", err.Error())
	}
	if !input.Role.Valid() {
		return nil, validation("role", "роль обязательна и должна быть USER или ADMIN")
	}
	if err := auth.ValidatePassword(input.Password); err != nil {
		return nil, validation("password", err.Error())
	}

	var buildingID *int64
	if input.Role == auth.RoleUser {
		if input.BuildingID == nil || *input.BuildingID <= 0 {
			return nil, buildingRequired()
		}
		if err := s.checkBuilding(ctx, *input.BuildingID); err != nil {
			return nil, err
		}
		buildingID = input.BuildingID
	}
	// AUTH-8: у ADMIN закрепления нет — присланное здание игнорируется.

	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	user, err := s.repo.Create(ctx, CreateParams{
		Login:              login,
		FullName:           fullName,
		Role:               input.Role,
		PasswordHash:       hash,
		BuildingID:         buildingID,
		IsActive:           true,
		MustChangePassword: true,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrDuplicateLogin):
			return nil, apierr.Conflict(apierr.CodeDuplicateLogin, "Пользователь с таким логином уже существует").
				WithDetail("login", "логин уже занят")
		default:
			return nil, apierr.Internal(err)
		}
	}
	return user, nil
}

// Update изменяет пользователя (AUTH-7/AUTH-8/AUTH-9).
//
// Порядок проверок: существование цели → самоблокировка/самопонижение →
// итоговая роль и здание → транзакционное правило «последний ADMIN» в repo →
// отзыв сессий при блокировке.
func (s *Service) Update(ctx context.Context, actorID, targetID int64, input UpdateInput) (*auth.User, error) {
	if targetID <= 0 {
		return nil, validation("id", "некорректный идентификатор")
	}
	target, err := s.repo.Get(ctx, targetID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apierr.NotFound("Пользователь не найден")
		}
		return nil, apierr.Internal(err)
	}

	if input.FullName != nil {
		fullName := strings.TrimSpace(*input.FullName)
		if err := validateFullName(fullName); err != nil {
			return nil, validation("full_name", err.Error())
		}
		input.FullName = &fullName
	}
	if input.Role != nil && !input.Role.Valid() {
		return nil, validation("role", "неизвестная роль")
	}

	// AUTH-7: запрет самоблокировки и самостоятельной смены роли.
	if actorID == targetID {
		if input.IsActive != nil && !*input.IsActive {
			return nil, cannotModifySelf("Нельзя заблокировать собственную учётную запись")
		}
		if input.Role != nil && *input.Role != target.Role {
			return nil, cannotModifySelf("Нельзя изменить собственную роль")
		}
	}

	finalRole := target.Role
	if input.Role != nil {
		finalRole = *input.Role
	}
	buildingID := target.BuildingID
	buildingSet := false
	if input.BuildingID.Set {
		buildingID = input.BuildingID.Value
		buildingSet = true
	}

	// AUTH-8: ADMIN всегда без здания (USER→ADMIN очищает закрепление).
	if finalRole == auth.RoleAdmin {
		buildingID = nil
		buildingSet = true
	}

	if finalRole == auth.RoleUser {
		if buildingID == nil || *buildingID <= 0 {
			return nil, buildingRequired()
		}
		// проверяем здание, если оно меняется или роль назначена заново
		roleChanged := input.Role != nil && *input.Role != target.Role
		if buildingSet || roleChanged {
			if err := s.checkBuilding(ctx, *buildingID); err != nil {
				return nil, err
			}
		}
	}

	updated, err := s.repo.Update(ctx, targetID, UpdateParams{
		FullName:      input.FullName,
		Role:          input.Role,
		BuildingID:    buildingID,
		BuildingIDSet: buildingSet,
		IsActive:      input.IsActive,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrLastAdmin):
			return nil, apierr.Conflict(apierr.CodeLastAdmin,
				"Нельзя понизить или заблокировать последнего активного администратора")
		case errors.Is(err, ErrNotFound):
			return nil, apierr.NotFound("Пользователь не найден")
		default:
			return nil, apierr.Internal(err)
		}
	}

	// AUTH-6: блокировка действует со следующего запроса — отзываем все
	// refresh-токены, чтобы пользователь не смог обновить сессию.
	if !updated.IsActive {
		if err := s.tokens.RevokeAllRefreshTokens(ctx, targetID, ""); err != nil {
			s.logger.Error("users: не удалось отозвать сессии заблокированного пользователя",
				"error", err, "user_id", targetID)
		}
	}
	return updated, nil
}

// ResetPassword сбрасывает пароль без текущего (ADMIN-маршрут, AUTH-5):
// устанавливает временный пароль, must_change_password=true и отзывает сессии.
func (s *Service) ResetPassword(ctx context.Context, actorID, targetID int64, newPassword string) error {
	if targetID <= 0 {
		return validation("id", "некорректный идентификатор")
	}
	if err := auth.ValidatePassword(newPassword); err != nil {
		return validation("new_password", err.Error())
	}
	if _, err := s.repo.Get(ctx, targetID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierr.NotFound("Пользователь не найден")
		}
		return apierr.Internal(err)
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return apierr.Internal(err)
	}
	if err := s.repo.SetPassword(ctx, targetID, hash, true); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierr.NotFound("Пользователь не найден")
		}
		return apierr.Internal(err)
	}
	if err := s.tokens.RevokeAllRefreshTokens(ctx, targetID, ""); err != nil {
		s.logger.Error("users: не удалось отозвать сессии после сброса пароля",
			"error", err, "user_id", targetID)
	}
	s.logger.Info("users: пароль сброшен администратором",
		"actor_id", actorID, "user_id", targetID)
	return nil
}

// Assignable подбирает исполнителей для заявки (TICKET-8):
//   - USER назначает только себя, поэтому получает только свой профиль;
//   - ADMIN получает активных USER здания заявки и всех активных ADMIN.
func (s *Service) Assignable(ctx context.Context, actor *auth.User, ticketID int64) ([]*auth.User, error) {
	if actor == nil {
		return nil, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация")
	}
	if ticketID <= 0 {
		return nil, validation("ticket_id", "обязательный параметр")
	}

	if actor.Role == auth.RoleUser {
		// AUTH-9: USER без здания не может работать с заявками.
		if actor.BuildingID == nil {
			return nil, apierr.Conflict(apierr.CodeBuildingNotAssigned,
				"Здание не назначено, обратитесь к администратору")
		}
		return []*auth.User{actor}, nil
	}
	if actor.Role != auth.RoleAdmin {
		return nil, apierr.Forbidden("Недостаточно прав")
	}

	buildingID, err := s.tickets.TicketBuildingID(ctx, ticketID)
	if err != nil {
		if errors.Is(err, ErrTicketNotFound) {
			return nil, apierr.NotFound("Заявка не найдена")
		}
		return nil, apierr.Internal(err)
	}
	items, err := s.repo.ListAssignable(ctx, buildingID)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return items, nil
}

// checkBuilding проверяет, что здание существует и активно (AUTH-8).
func (s *Service) checkBuilding(ctx context.Context, buildingID int64) error {
	exists, active, err := s.repo.BuildingStatus(ctx, buildingID)
	if err != nil {
		return apierr.Internal(err)
	}
	if !exists {
		return validation("building_id", "здание не найдено")
	}
	if !active {
		return apierr.New(422, apierr.CodeBuildingInactive,
			"Нельзя назначить пользователя в неактивное здание").
			WithDetail("building_id", "здание деактивировано")
	}
	return nil
}

func validateLogin(login string) error {
	n := len([]rune(login))
	if n < 3 || n > 64 {
		return errors.New("логин должен содержать от 3 до 64 символов")
	}
	if !loginPattern.MatchString(login) {
		return errors.New("логин может содержать только латиницу, цифры, точку, дефис и подчёркивание")
	}
	return nil
}

func validateFullName(fullName string) error {
	n := len([]rune(strings.TrimSpace(fullName)))
	if n < 1 || n > 200 {
		return errors.New("ФИО должно содержать от 1 до 200 символов")
	}
	return nil
}

func validation(field, message string) *apierr.Error {
	return apierr.Validation("Некорректные данные пользователя", map[string]any{field: message})
}

func buildingRequired() *apierr.Error {
	return apierr.New(422, apierr.CodeBuildingRequiredForUser,
		"Для пользователя с ролью USER обязательно активное здание").
		WithDetail("building_id", "обязательно для USER")
}

func cannotModifySelf(message string) *apierr.Error {
	return apierr.Conflict(apierr.CodeCannotModifySelf, message)
}
