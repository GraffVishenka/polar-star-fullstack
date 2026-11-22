package tickets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
	"github.com/graffvishenka/polar-star-service/backend/internal/users"
)

// Service реализует TICKET-1..TICKET-16 и публичный периметр заявок.
type Service struct {
	repo     Repository
	users    UserLookup
	settings TicketSettings
	clock    platform.Clock
	notifier platform.Notifier
	files    FileAttacher
	logger   *slog.Logger
}

// NewService создаёт сервис заявок. files может быть nil — тогда комментарии
// с вложениями отклоняются (до мержа Б3 в main передаётся DisabledFileAttacher).
func NewService(repo Repository, userLookup UserLookup, settings TicketSettings, clock platform.Clock,
	notifier platform.Notifier, files FileAttacher, logger *slog.Logger) *Service {
	if clock == nil {
		clock = platform.RealClock{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:     repo,
		users:    userLookup,
		settings: settings,
		clock:    clock,
		notifier: notifier,
		files:    files,
		logger:   logger,
	}
}

// calendar возвращает рабочий календарь приложения (CAL-4).
func (s *Service) calendar() platform.Calendar { return s.settings.Calendar() }

// Create создаёт заявку от имени авторизованного пользователя (TICKET-2).
func (s *Service) Create(ctx context.Context, actor *auth.User, input CreateInput) (*Ticket, error) {
	if actor == nil {
		return nil, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация")
	}
	applicant := strings.TrimSpace(input.ApplicantName)
	cabinet := strings.TrimSpace(input.Cabinet)
	if err := validateTicketFields(applicant, cabinet, input.Description); err != nil {
		return nil, err
	}
	if input.ProblemTypeID <= 0 {
		return nil, validation("problem_type_id", "обязательное поле")
	}
	buildingID, err := s.resolveCreateBuilding(ctx, actor, input.BuildingID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureProblemType(ctx, input.ProblemTypeID); err != nil {
		return nil, err
	}
	executorID, err := s.resolveExecutorForActor(ctx, actor, buildingID, input.ExecutorID)
	if err != nil {
		return nil, err
	}

	ticket, err := s.repo.Create(ctx, CreateParams{
		BuildingID:      buildingID,
		ApplicantName:   applicant,
		Cabinet:         cabinet,
		ProblemTypeID:   input.ProblemTypeID,
		Description:     strings.TrimSpace(input.Description),
		Deadline:        s.calculateDeadline(),
		ExecutorID:      executorID,
		CreatedByUserID: &actor.ID,
	})
	if err != nil {
		return nil, apierr.Internal(err)
	}
	s.publish(ctx, platform.NotificationEvent{
		Type:        platform.EventNewTicket,
		TicketID:    ticket.ID,
		BuildingID:  &ticket.BuildingID,
		ActorUserID: actor.ID,
		Title:       "Новая заявка",
		Body:        fmt.Sprintf("%s · %s", ticket.ApplicantName, ticket.Cabinet),
		URL:         fmt.Sprintf("/tickets/%d", ticket.ID),
	})
	if executorID != nil {
		s.publishAssigned(ctx, ticket, *executorID, actor.ID)
	}
	return ticket, nil
}

// CreatePublic создаёт анонимную заявку и выдаёт токен доступа (TICKET-4, TICKET-13).
func (s *Service) CreatePublic(ctx context.Context, input PublicCreateInput) (*Ticket, string, error) {
	applicant := strings.TrimSpace(input.ApplicantName)
	cabinet := strings.TrimSpace(input.Cabinet)
	if err := validateTicketFields(applicant, cabinet, input.Description); err != nil {
		return nil, "", err
	}
	if input.BuildingID <= 0 {
		return nil, "", validation("building_id", "обязательное поле")
	}
	building, err := s.repo.Building(ctx, input.BuildingID)
	if err != nil {
		if errors.Is(err, ErrBuildingNotFound) {
			return nil, "", apierr.NotFound("Здание не найдено")
		}
		return nil, "", apierr.Internal(err)
	}
	if !building.IsActive {
		return nil, "", apierr.NotFound("Здание не найдено")
	}
	if err := s.ensureProblemType(ctx, input.ProblemTypeID); err != nil {
		return nil, "", err
	}

	plainToken, tokenHash, err := newAccessToken()
	if err != nil {
		return nil, "", apierr.Internal(err)
	}
	ticket, err := s.repo.Create(ctx, CreateParams{
		BuildingID:      building.ID,
		ApplicantName:   applicant,
		Cabinet:         cabinet,
		ProblemTypeID:   input.ProblemTypeID,
		Description:     strings.TrimSpace(input.Description),
		Deadline:        s.calculateDeadline(),
		AccessTokenHash: &tokenHash,
	})
	if err != nil {
		return nil, "", apierr.Internal(err)
	}
	s.publish(ctx, platform.NotificationEvent{
		Type:       platform.EventNewTicket,
		TicketID:   ticket.ID,
		BuildingID: &ticket.BuildingID,
		Title:      "Новая заявка",
		Body:       fmt.Sprintf("%s · %s", ticket.ApplicantName, ticket.Cabinet),
		URL:        fmt.Sprintf("/tickets/%d", ticket.ID),
	})
	return ticket, plainToken, nil
}

// List — авторизованный список (TICKET-7, TICKET-15).
func (s *Service) List(ctx context.Context, actor *auth.User, filter ListFilter) (*ListResult, error) {
	if actor == nil {
		return nil, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация")
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
	for _, status := range filter.Statuses {
		if !status.Valid() {
			return nil, validation("status", "неизвестный статус")
		}
	}
	if filter.Sort == "" {
		filter.Sort = "created_at"
	}
	if filter.Sort != "created_at" && filter.Sort != "deadline" {
		return nil, validation("sort", "допустимо created_at или deadline")
	}
	if filter.Order == "" {
		filter.Order = "desc"
	}
	if filter.Order != "asc" && filter.Order != "desc" {
		return nil, validation("order", "допустимо asc или desc")
	}
	if filter.DateFrom != nil && filter.DateTo != nil && filter.DateFrom.After(*filter.DateTo) {
		return nil, validation("date_from", "начало периода позже конца")
	}
	if filter.ExecutorNone && filter.ExecutorID != nil {
		return nil, validation("executor_id", "укажите либо id исполнителя, либо none")
	}
	filter.Search = strings.TrimSpace(filter.Search)
	if len([]rune(filter.Search)) > 120 {
		return nil, validation("search", "максимум 120 символов")
	}

	// TICKET-7: USER видит только заявки своего здания; фильтр принудительно
	// подменяется его зданием. AUTH-9: без здания — 409.
	buildingID := filter.BuildingID
	if actor.Role != auth.RoleAdmin {
		if actor.BuildingID == nil {
			return nil, buildingNotAssigned()
		}
		buildingID = actor.BuildingID
	}

	params := ListParams{
		BuildingID:    buildingID,
		Statuses:      filter.Statuses,
		ExecutorID:    filter.ExecutorID,
		ExecutorNone:  filter.ExecutorNone,
		ProblemTypeID: filter.ProblemTypeID,
		Search:        filter.Search,
		Sort:          filter.Sort,
		Order:         filter.Order,
		Page:          filter.Page,
		Size:          filter.Size,
	}
	// Диапазон дат задаётся локальными датами, в SQL — UTC-границы суток.
	if filter.DateFrom != nil {
		from := startOfLocalDayUTC(*filter.DateFrom, s.calendar().Location())
		params.DateFrom = &from
	}
	if filter.DateTo != nil {
		to := startOfLocalDayUTC(*filter.DateTo, s.calendar().Location()).AddDate(0, 0, 1)
		params.DateTo = &to
	}

	items, total, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return &ListResult{Items: items, Total: total, Page: filter.Page, Size: filter.Size}, nil
}

// Get возвращает карточку заявки с проверкой видимости (TICKET-7).
func (s *Service) Get(ctx context.Context, actor *auth.User, id int64) (*Ticket, error) {
	return s.accessTicket(ctx, actor, id)
}

// Update изменяет заявку по правилам TICKET-8..TICKET-11. Возвращает флаг
// executor_reset (TICKET-9).
func (s *Service) Update(ctx context.Context, actor *auth.User, id int64, input UpdateInput) (*Ticket, bool, error) {
	ticket, err := s.accessTicket(ctx, actor, id)
	if err != nil {
		return nil, false, err
	}
	if id <= 0 {
		return nil, false, validation("id", "некорректный идентификатор")
	}

	// Дедлайн и здание меняет только ADMIN (TICKET-9).
	if actor.Role != auth.RoleAdmin {
		if input.Deadline.Set {
			return nil, false, apierr.Forbidden("Изменять дедлайн может только администратор")
		}
		if input.BuildingID.Set {
			return nil, false, apierr.Forbidden("Изменять здание заявки может только администратор")
		}
	}

	// TICKET-11: USER правит поля только как исполнитель.
	fieldsProvided := input.ApplicantName != nil || input.Cabinet != nil ||
		input.ProblemTypeID != nil || input.Description != nil
	if fieldsProvided && actor.Role != auth.RoleAdmin {
		if ticket.ExecutorID == nil || *ticket.ExecutorID != actor.ID {
			return nil, false, apierr.New(403, apierr.CodeNotExecutor,
				"Редактировать заявку может только её исполнитель")
		}
	}

	params := UpdateParams{}
	changes := 0

	if input.ApplicantName != nil {
		value := strings.TrimSpace(*input.ApplicantName)
		if len([]rune(value)) < 1 || len([]rune(value)) > 120 {
			return nil, false, validation("applicant_name", "от 1 до 120 символов")
		}
		params.ApplicantName = &value
		changes++
	}
	if input.Cabinet != nil {
		value := strings.TrimSpace(*input.Cabinet)
		if len([]rune(value)) < 1 || len([]rune(value)) > 32 {
			return nil, false, validation("cabinet", "от 1 до 32 символов")
		}
		params.Cabinet = &value
		changes++
	}
	if input.Description != nil {
		value := strings.TrimSpace(*input.Description)
		if len([]rune(value)) < 1 || len([]rune(value)) > 5000 {
			return nil, false, validation("description", "от 1 до 5000 символов")
		}
		params.Description = &value
		changes++
	}
	if input.ProblemTypeID != nil {
		if *input.ProblemTypeID <= 0 {
			return nil, false, validation("problem_type_id", "некорректный идентификатор")
		}
		if err := s.ensureProblemType(ctx, *input.ProblemTypeID); err != nil {
			return nil, false, err
		}
		params.ProblemTypeID = input.ProblemTypeID
		changes++
	}

	// TICKET-10: статус меняет USER своего здания и ADMIN; completed_at
	// проставляется при переходе в done и обнуляется при выходе из done.
	if input.Status != nil {
		if !input.Status.Valid() {
			return nil, false, validation("status", "неизвестный статус")
		}
		if *input.Status != ticket.Status {
			status := *input.Status
			params.Status = &status
			changes++
			if status == StatusDone {
				now := s.clock.Now().UTC()
				params.CompletedAt = &now
				params.CompletedAtSet = true
			} else if ticket.Status == StatusDone {
				params.CompletedAt = nil
				params.CompletedAtSet = true
			}
		}
	}

	// TICKET-9: смена здания ADMIN'ом.
	buildingChanged := false
	newBuildingID := ticket.BuildingID
	if input.BuildingID.Set {
		if input.BuildingID.Value == nil || *input.BuildingID.Value <= 0 {
			return nil, false, validation("building_id", "некорректный идентификатор")
		}
		building, err := s.repo.Building(ctx, *input.BuildingID.Value)
		if err != nil {
			if errors.Is(err, ErrBuildingNotFound) {
				return nil, false, apierr.NotFound("Здание не найдено")
			}
			return nil, false, apierr.Internal(err)
		}
		if !building.IsActive {
			return nil, false, apierr.NotFound("Здание не найдено")
		}
		newBuildingID = building.ID
		buildingChanged = newBuildingID != ticket.BuildingID
		params.BuildingID = &newBuildingID
		params.BuildingIDSet = true
		changes++
	}

	// TICKET-8: назначение исполнителя.
	finalExecutor := ticket.ExecutorID
	executorChanged := false
	if input.ExecutorID.Set {
		// Снимать исполнителя может только ADMIN (TICKET-8).
		if input.ExecutorID.Value == nil && actor.Role != auth.RoleAdmin {
			return nil, false, apierr.New(403, apierr.CodeAssignOtherUser,
				"Снимать исполнителя может только администратор")
		}
		executorID, err := s.resolveExecutorForActor(ctx, actor, newBuildingID, input.ExecutorID.Value)
		if err != nil {
			return nil, false, err
		}
		finalExecutor = executorID
		executorChanged = !sameInt64Ptr(ticket.ExecutorID, finalExecutor)
		params.ExecutorID = executorID
		params.ExecutorIDSet = true
		changes++
	}

	// TICKET-9: при смене здания USER-исполнитель из другого здания
	// сбрасывается, ADMIN-исполнитель сохраняется.
	executorReset := false
	if buildingChanged && !input.ExecutorID.Set && ticket.ExecutorID != nil {
		executor, err := s.users.Get(ctx, *ticket.ExecutorID)
		if err != nil {
			if !errors.Is(err, users.ErrNotFound) {
				return nil, false, apierr.Internal(err)
			}
			// Исполнитель удалён/не найден — сбрасываем.
			finalExecutor = nil
			params.ExecutorID = nil
			params.ExecutorIDSet = true
			executorReset = true
			executorChanged = true
			changes++
		} else if executor.Role == auth.RoleUser &&
			(executor.BuildingID == nil || *executor.BuildingID != newBuildingID) {
			finalExecutor = nil
			params.ExecutorID = nil
			params.ExecutorIDSet = true
			executorReset = true
			executorChanged = true
			changes++
		}
	}

	// TICKET-9: дедлайн ADMIN'а, дата в прошлом допустима.
	if input.Deadline.Set {
		params.DeadlineSet = true
		changes++
		if input.Deadline.Value == nil {
			params.Deadline = nil
		} else {
			deadline, err := parseDateUTC(*input.Deadline.Value)
			if err != nil {
				return nil, false, validation("deadline", "ожидается YYYY-MM-DD")
			}
			params.Deadline = &deadline
		}
	}

	if changes == 0 {
		return nil, false, validation("body", "нет полей для изменения")
	}

	updated, err := s.repo.Update(ctx, id, params)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, false, apierr.NotFound("Заявка не найдена")
		}
		return nil, false, apierr.Internal(err)
	}
	if executorChanged && finalExecutor != nil {
		s.publishAssigned(ctx, updated, *finalExecutor, actor.ID)
	}
	return updated, executorReset, nil
}

// Delete мягко удаляет заявку в корзину (TICKET-12, только ADMIN).
func (s *Service) Delete(ctx context.Context, actor *auth.User, id int64) error {
	if actor == nil || actor.Role != auth.RoleAdmin {
		return apierr.Forbidden("Удалять заявки может только администратор")
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierr.NotFound("Заявка не найдена")
		}
		return apierr.Internal(err)
	}
	return nil
}

// Comments — комментарии заявки (TICKET-14).
func (s *Service) Comments(ctx context.Context, actor *auth.User, ticketID int64, page, size int) (*CommentPage, error) {
	if _, err := s.accessTicket(ctx, actor, ticketID); err != nil {
		return nil, err
	}
	page, size, err := normalizeCommentPage(page, size)
	if err != nil {
		return nil, err
	}
	items, total, err := s.repo.ListComments(ctx, ticketID, page, size)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return &CommentPage{Items: items, Total: total, Page: page, Size: size}, nil
}

// AddComment добавляет комментарий от сотрудника и привязывает вложения (Б3).
func (s *Service) AddComment(ctx context.Context, actor *auth.User, ticketID int64, text string, fileIDs []int64) (*Comment, error) {
	ticket, err := s.accessTicket(ctx, actor, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.validateComment(ctx, text, fileIDs); err != nil {
		return nil, err
	}
	comment, err := s.repo.CreateComment(ctx, CommentParams{
		TicketID:     ticket.ID,
		AuthorUserID: &actor.ID,
		AuthorName:   actor.FullName,
		Body:         text,
	})
	if err != nil {
		return nil, apierr.Internal(err)
	}
	if err := s.attachFiles(ctx, fileIDs, comment.ID); err != nil {
		return nil, err
	}
	s.notifyComment(ctx, ticket, actor.ID)
	return comment, nil
}

// PublicBuildings — активные здания со счётчиками (TICKET-6).
func (s *Service) PublicBuildings(ctx context.Context) ([]*BuildingCount, error) {
	items, err := s.repo.PublicBuildings(ctx)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return items, nil
}

// PublicBoard — публичный дашборд здания или сводка по всем зданиям (TICKET-5).
func (s *Service) PublicBoard(ctx context.Context, buildingID *int64) (*PublicBoard, error) {
	if buildingID != nil {
		building, err := s.repo.Building(ctx, *buildingID)
		if err != nil {
			if errors.Is(err, ErrBuildingNotFound) {
				return nil, apierr.NotFound("Здание не найдено")
			}
			return nil, apierr.Internal(err)
		}
		if !building.IsActive {
			return nil, apierr.NotFound("Здание не найдено")
		}
		items, err := s.repo.PublicList(ctx, &building.ID)
		if err != nil {
			return nil, apierr.Internal(err)
		}
		name := building.Name
		return &PublicBoard{BuildingID: &building.ID, BuildingName: &name, Items: items}, nil
	}
	items, err := s.repo.PublicList(ctx, nil)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return &PublicBoard{Items: items}, nil
}

// PublicTicket — карточка по токену с комментариями (TICKET-13/PUB-6).
func (s *Service) PublicTicket(ctx context.Context, token string) (*Ticket, []*Comment, error) {
	ticket, err := s.byAccessToken(ctx, token)
	if err != nil {
		return nil, nil, err
	}
	comments, _, err := s.repo.ListComments(ctx, ticket.ID, 1, 50)
	if err != nil {
		return nil, nil, apierr.Internal(err)
	}
	return ticket, comments, nil
}

// PublicComments — чтение комментариев по токену.
func (s *Service) PublicComments(ctx context.Context, token string, page, size int) (*CommentPage, error) {
	ticket, err := s.byAccessToken(ctx, token)
	if err != nil {
		return nil, err
	}
	page, size, err = normalizeCommentPage(page, size)
	if err != nil {
		return nil, err
	}
	items, total, err := s.repo.ListComments(ctx, ticket.ID, page, size)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return &CommentPage{Items: items, Total: total, Page: page, Size: size}, nil
}

// AddPublicComment — комментарий анонима, автор = applicant_name заявки.
func (s *Service) AddPublicComment(ctx context.Context, token, text string, fileIDs []int64) (*Comment, error) {
	ticket, err := s.byAccessToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if err := s.validateComment(ctx, text, fileIDs); err != nil {
		return nil, err
	}
	comment, err := s.repo.CreateComment(ctx, CommentParams{
		TicketID:   ticket.ID,
		AuthorName: ticket.ApplicantName,
		Body:       text,
	})
	if err != nil {
		return nil, apierr.Internal(err)
	}
	if err := s.attachFiles(ctx, fileIDs, comment.ID); err != nil {
		return nil, err
	}
	s.notifyComment(ctx, ticket, 0)
	return comment, nil
}

// accessTicket загружает заявку и проверяет видимость по зданию (TICKET-7).
func (s *Service) accessTicket(ctx context.Context, actor *auth.User, id int64) (*Ticket, error) {
	if actor == nil {
		return nil, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация")
	}
	ticket, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apierr.NotFound("Заявка не найдена")
		}
		return nil, apierr.Internal(err)
	}
	if actor.Role != auth.RoleAdmin {
		if actor.BuildingID == nil {
			return nil, buildingNotAssigned()
		}
		if ticket.BuildingID != *actor.BuildingID {
			// TICKET-7: чужое здание не раскрываем — 404.
			return nil, apierr.NotFound("Заявка не найдена")
		}
	}
	return ticket, nil
}

func (s *Service) byAccessToken(ctx context.Context, token string) (*Ticket, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, apierr.NotFound("Заявка не найдена")
	}
	ticket, err := s.repo.ByAccessTokenHash(ctx, auth.HashToken(token))
	if err != nil {
		if errors.Is(err, ErrAccessTokenNotFound) || errors.Is(err, ErrNotFound) {
			return nil, apierr.NotFound("Заявка не найдена")
		}
		return nil, apierr.Internal(err)
	}
	return ticket, nil
}

// resolveCreateBuilding определяет здание новой заявки (TICKET-2/TICKET-4).
func (s *Service) resolveCreateBuilding(ctx context.Context, actor *auth.User, requested *int64) (int64, error) {
	if actor.Role != auth.RoleAdmin {
		if actor.BuildingID == nil {
			return 0, buildingNotAssigned()
		}
		// USER создаёт только в своём здании; чужое не раскрываем (TICKET-7).
		if requested != nil && *requested != *actor.BuildingID {
			return 0, apierr.NotFound("Здание не найдено")
		}
		// Неактивное здание не принимает новые заявки (TICKET-4/EQ-6).
		building, err := s.repo.Building(ctx, *actor.BuildingID)
		if err != nil {
			if errors.Is(err, ErrBuildingNotFound) {
				return 0, apierr.NotFound("Здание не найдено")
			}
			return 0, apierr.Internal(err)
		}
		if !building.IsActive {
			return 0, apierr.NotFound("Здание не найдено")
		}
		return building.ID, nil
	}
	if requested == nil || *requested <= 0 {
		return 0, validation("building_id", "обязательное поле")
	}
	building, err := s.repo.Building(ctx, *requested)
	if err != nil {
		if errors.Is(err, ErrBuildingNotFound) {
			return 0, apierr.NotFound("Здание не найдено")
		}
		return 0, apierr.Internal(err)
	}
	if !building.IsActive {
		return 0, apierr.NotFound("Здание не найдено")
	}
	return building.ID, nil
}

// resolveExecutorForActor проверяет право назначать исполнителя (TICKET-8):
// USER — только себя, ADMIN — USER здания заявки или ADMIN.
func (s *Service) resolveExecutorForActor(ctx context.Context, actor *auth.User, buildingID int64, executorID *int64) (*int64, error) {
	if executorID == nil {
		return nil, nil
	}
	if *executorID <= 0 {
		return nil, validation("executor_id", "некорректный идентификатор")
	}
	if actor.Role != auth.RoleAdmin {
		if *executorID != actor.ID {
			return nil, apierr.New(403, apierr.CodeAssignOtherUser,
				"Назначить исполнителем можно только себя")
		}
		if actor.BuildingID == nil {
			return nil, buildingNotAssigned()
		}
		return executorID, nil
	}

	executor, err := s.users.Get(ctx, *executorID)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			return nil, validation("executor_id", "пользователь не найден")
		}
		return nil, apierr.Internal(err)
	}
	if !executor.IsActive {
		return nil, validation("executor_id", "пользователь заблокирован")
	}
	if executor.Role == auth.RoleAdmin {
		return executorID, nil
	}
	// USER-исполнитель должен быть из здания заявки (TICKET-8).
	if executor.BuildingID == nil || *executor.BuildingID != buildingID {
		return nil, apierr.New(422, apierr.CodeExecutorWrongBuilding,
			"Исполнитель должен быть сотрудником здания заявки").
			WithDetail("executor_id", "другое здание")
	}
	return executorID, nil
}

func (s *Service) ensureProblemType(ctx context.Context, id int64) error {
	active, err := s.repo.ProblemTypeActive(ctx, id)
	if err != nil {
		if errors.Is(err, ErrProblemTypeNotFound) {
			return validation("problem_type_id", "вид проблемы не найден")
		}
		return apierr.Internal(err)
	}
	if !active {
		return validation("problem_type_id", "вид проблемы деактивирован")
	}
	return nil
}

// validateComment проверяет TICKET-14: текст ≤3000, до 5 вложений, пустой
// текст допустим только при наличии вложения.
func (s *Service) validateComment(ctx context.Context, text string, fileIDs []int64) error {
	if len([]rune(text)) > 3000 {
		return validation("text", "максимум 3000 символов")
	}
	if len(fileIDs) > 5 {
		return validation("file_ids", "не более 5 вложений")
	}
	for _, id := range fileIDs {
		if id <= 0 {
			return validation("file_ids", "некорректный идентификатор файла")
		}
	}
	if strings.TrimSpace(text) == "" && len(fileIDs) == 0 {
		return validation("text", "текст или вложение обязательны")
	}
	if len(fileIDs) > 0 {
		if s.files == nil {
			return apierr.Validation("Вложения недоступны", map[string]any{"file_ids": "не поддерживаются"})
		}
		if err := s.files.ValidateScope(ctx, fileIDs, "ticket"); err != nil {
			return err
		}
	}
	return nil
}

// attachFiles привязывает вложения; при ошибке откатывает созданный комментарий.
func (s *Service) attachFiles(ctx context.Context, fileIDs []int64, commentID int64) error {
	if len(fileIDs) == 0 {
		return nil
	}
	if s.files == nil {
		_ = s.repo.DeleteComment(ctx, commentID)
		return apierr.Validation("Вложения недоступны", map[string]any{"file_ids": "не поддерживаются"})
	}
	if err := s.files.ValidateAndAttach(ctx, fileIDs, commentID); err != nil {
		if cleanupErr := s.repo.DeleteComment(ctx, commentID); cleanupErr != nil {
			s.logger.Error("tickets: не удалось откатить комментарий после ошибки вложений",
				"error", cleanupErr, "comment_id", commentID)
		}
		return err
	}
	return nil
}

func (s *Service) calculateDeadline() *time.Time {
	days := s.settings.DeadlineDays()
	if days < 1 {
		days = 1
	}
	if days > 30 {
		days = 30
	}
	deadline := dateOnlyUTC(s.calendar().WorkingDays(s.clock.Now(), days))
	return &deadline
}

func (s *Service) publish(ctx context.Context, event platform.NotificationEvent) {
	if s.notifier == nil {
		return
	}
	if err := s.notifier.Publish(ctx, event); err != nil {
		// NOTIF-3/NOTIF-6: уведомления best-effort, не ломают бизнес-операцию.
		s.logger.Warn("tickets: не удалось опубликовать уведомление",
			"error", err, "event_type", event.Type, "ticket_id", event.TicketID)
	}
}

func (s *Service) publishAssigned(ctx context.Context, ticket *Ticket, executorID, actorID int64) {
	s.publish(ctx, platform.NotificationEvent{
		Type:        platform.EventTicketAssigned,
		TicketID:    ticket.ID,
		BuildingID:  &ticket.BuildingID,
		UserIDs:     []int64{executorID},
		ActorUserID: actorID,
		Title:       "Вам назначена заявка",
		Body:        fmt.Sprintf("%s · %s", ticket.ApplicantName, ticket.Cabinet),
		URL:         fmt.Sprintf("/tickets/%d", ticket.ID),
	})
}

// notifyComment отправляет NEW_COMMENT исполнителю и автору-сотруднику заявки,
// кроме автора комментария (NOTIF-2). Анонимный комментарий (authorID=0) не
// исключается.
func (s *Service) notifyComment(ctx context.Context, ticket *Ticket, authorID int64) {
	recipients := make([]int64, 0, 2)
	seen := map[int64]struct{}{}
	add := func(id int64) {
		if id == 0 || id == authorID {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		recipients = append(recipients, id)
	}
	if ticket.CreatedByUserID != nil {
		add(*ticket.CreatedByUserID)
	}
	if ticket.ExecutorID != nil {
		add(*ticket.ExecutorID)
	}
	if len(recipients) == 0 {
		return
	}
	s.publish(ctx, platform.NotificationEvent{
		Type:        platform.EventNewComment,
		TicketID:    ticket.ID,
		BuildingID:  &ticket.BuildingID,
		UserIDs:     recipients,
		ActorUserID: authorID,
		Title:       "Новый комментарий",
		Body:        fmt.Sprintf("%s · %s", ticket.ApplicantName, ticket.Cabinet),
		URL:         fmt.Sprintf("/tickets/%d", ticket.ID),
	})
}

func normalizeCommentPage(page, size int) (int, int, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 50
	}
	if size > 100 {
		return 0, 0, validation("size", "максимум 100")
	}
	return page, size, nil
}

// startOfLocalDayUTC переводит локальную дату в UTC-начало суток (для фильтра
// created_at по timezone приложения).
func startOfLocalDayUTC(date time.Time, loc *time.Location) time.Time {
	y, m, d := date.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc).UTC()
}

func validateTicketFields(applicant, cabinet, description string) error {
	if n := len([]rune(applicant)); n < 1 || n > 120 {
		return validation("applicant_name", "от 1 до 120 символов")
	}
	if n := len([]rune(cabinet)); n < 1 || n > 32 {
		return validation("cabinet", "от 1 до 32 символов")
	}
	if n := len([]rune(strings.TrimSpace(description))); n < 1 || n > 5000 {
		return validation("description", "от 1 до 5000 символов")
	}
	return nil
}

func sameInt64Ptr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func validation(field, message string) *apierr.Error {
	return apierr.Validation("Некорректные данные заявки", map[string]any{field: message})
}

func buildingNotAssigned() *apierr.Error {
	return apierr.Conflict(apierr.CodeBuildingNotAssigned,
		"Здание не назначено, обратитесь к администратору")
}
