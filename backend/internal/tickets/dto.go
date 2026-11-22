package tickets

import (
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// Дефолтные названия/цвета статусов (TICKET-1). Б7 может переопределить их
// через StatusStyler.
var (
	defaultStatusLabels = map[Status]string{
		StatusNew:        "Новая",
		StatusInProgress: "В работе",
		StatusDeferred:   "Отложена",
		StatusDone:       "Выполнена",
	}
	defaultStatusColors = map[Status]string{
		StatusNew:        "#1677ff",
		StatusInProgress: "#fa8c16",
		StatusDeferred:   "#8c8c8c",
		StatusDone:       "#52c41a",
	}
)

// TicketDTO — авторизованная карточка заявки (контракт Ticket).
type TicketDTO struct {
	ID              int64      `json:"id"`
	BuildingID      int64      `json:"building_id"`
	BuildingName    string     `json:"building_name"`
	ApplicantName   string     `json:"applicant_name"`
	Cabinet         string     `json:"cabinet"`
	ProblemTypeID   int64      `json:"problem_type_id"`
	ProblemTypeName string     `json:"problem_type_name"`
	Description     string     `json:"description"`
	Deadline        *string    `json:"deadline"`
	ExecutorID      *int64     `json:"executor_id"`
	ExecutorName    *string    `json:"executor_name"`
	Status          Status     `json:"status"`
	StatusLabel     string     `json:"status_label"`
	StatusColor     string     `json:"status_color"`
	IsOverdue       bool       `json:"is_overdue"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// TicketListItemDTO — элемент списка (контракт TicketListItem, без description).
type TicketListItemDTO struct {
	ID              int64     `json:"id"`
	BuildingID      int64     `json:"building_id"`
	BuildingName    string    `json:"building_name"`
	ApplicantName   string    `json:"applicant_name"`
	Cabinet         string    `json:"cabinet"`
	ProblemTypeName string    `json:"problem_type_name"`
	Status          Status    `json:"status"`
	StatusLabel     string    `json:"status_label"`
	StatusColor     string    `json:"status_color"`
	Deadline        *string   `json:"deadline"`
	ExecutorID      *int64    `json:"executor_id"`
	ExecutorName    *string   `json:"executor_name"`
	IsOverdue       bool      `json:"is_overdue"`
	CreatedAt       time.Time `json:"created_at"`
}

// PublicTicketCardDTO — строгий набор полей публичной карточки (TICKET-5):
// без description, комментариев и файлов.
type PublicTicketCardDTO struct {
	ID              int64     `json:"id"`
	BuildingID      int64     `json:"building_id"`
	BuildingName    string    `json:"building_name"`
	ApplicantName   string    `json:"applicant_name"`
	Cabinet         string    `json:"cabinet"`
	ProblemTypeName string    `json:"problem_type_name"`
	Status          Status    `json:"status"`
	StatusLabel     string    `json:"status_label"`
	StatusColor     string    `json:"status_color"`
	Deadline        *string   `json:"deadline"`
	ExecutorName    *string   `json:"executor_name"`
	CreatedAt       time.Time `json:"created_at"`
}

// PublicTicketDetailDTO — карточка по токену (TICKET-13/PUB-6): добавляет
// описание, завершение, просрочку и комментарии.
type PublicTicketDetailDTO struct {
	PublicTicketCardDTO
	Description string       `json:"description"`
	CompletedAt *time.Time   `json:"completed_at"`
	IsOverdue   bool         `json:"is_overdue"`
	Comments    []CommentDTO `json:"comments"`
}

// PublicTicketsResponseDTO — публичный дашборд одного здания или сводка.
type PublicTicketsResponseDTO struct {
	BuildingID   *int64                `json:"building_id"`
	BuildingName *string               `json:"building_name"`
	Items        []PublicTicketCardDTO `json:"items"`
}

// PublicTicketCreateResponseDTO — ответ создания анонимной заявки; токен
// выдаётся один раз (TICKET-13).
type PublicTicketCreateResponseDTO struct {
	Ticket      PublicTicketDetailDTO `json:"ticket"`
	AccessToken string                `json:"access_token"`
	AccessURL   string                `json:"access_url"`
}

// CommentFileDTO — вложение комментария. До мержа Б3 список всегда пустой.
type CommentFileDTO struct {
	ID        int64     `json:"id"`
	Filename  string    `json:"filename"`
	MimeType  string    `json:"mime_type"`
	Size      int64     `json:"size"`
	Scope     string    `json:"scope"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// CommentDTO — комментарий (контракт TicketComment).
type CommentDTO struct {
	ID           int64            `json:"id"`
	TicketID     int64            `json:"ticket_id"`
	AuthorUserID *int64           `json:"author_user_id"`
	AuthorName   string           `json:"author_name"`
	Text         string           `json:"text"`
	CreatedAt    time.Time        `json:"created_at"`
	Files        []CommentFileDTO `json:"files"`
}

// CommentListDTO — страница комментариев.
type CommentListDTO struct {
	Items []CommentDTO `json:"items"`
	Total int          `json:"total"`
	Page  int          `json:"page"`
	Size  int          `json:"size"`
}

// TicketUpdateResponseDTO — результат PATCH с флагом сброса исполнителя (TICKET-9).
type TicketUpdateResponseDTO struct {
	Ticket        TicketDTO `json:"ticket"`
	ExecutorReset bool      `json:"executor_reset"`
}

// PublicBuildingCountsDTO — счётчики незавершённых заявок здания (TICKET-6).
type PublicBuildingCountsDTO struct {
	New        int `json:"new"`
	InProgress int `json:"in_progress"`
	Deferred   int `json:"deferred"`
}

// PublicBuildingDTO — активное здание на публичном экране выбора.
type PublicBuildingDTO struct {
	ID     int64                   `json:"id"`
	Name   string                  `json:"name"`
	Counts PublicBuildingCountsDTO `json:"counts"`
}

// statusStyle возвращает настраиваемые название/цвет или дефолт (TICKET-1).
func (s *Service) statusStyle(status Status) (label, color string) {
	if styler, ok := s.settings.(StatusStyler); ok {
		label, color = styler.StatusStyle(status)
	}
	if label == "" {
		label = defaultStatusLabels[status]
	}
	if color == "" {
		color = defaultStatusColors[status]
	}
	return label, color
}

// isOverdue: дедлайн раньше сегодняшней даты в timezone и статус не done
// (TICKET-16). Сравниваются календарные даты, а не моменты времени.
func (s *Service) isOverdue(ticket *Ticket) bool {
	if ticket == nil || ticket.Status == StatusDone || ticket.Deadline == nil {
		return false
	}
	today := dateOnlyUTC(s.calendar().Today())
	return dateOnlyUTC(*ticket.Deadline).Before(today)
}

// formatDeadline форматирует DATE в YYYY-MM-DD (UTC-дата в БД).
func formatDeadline(deadline *time.Time) *string {
	if deadline == nil {
		return nil
	}
	value := dateOnlyUTC(*deadline).Format(platform.DateLayout)
	return &value
}

// TicketDTO собирает авторизованную карточку.
func (s *Service) TicketDTO(ticket *Ticket) TicketDTO {
	label, color := s.statusStyle(ticket.Status)
	return TicketDTO{
		ID:              ticket.ID,
		BuildingID:      ticket.BuildingID,
		BuildingName:    ticket.BuildingName,
		ApplicantName:   ticket.ApplicantName,
		Cabinet:         ticket.Cabinet,
		ProblemTypeID:   ticket.ProblemTypeID,
		ProblemTypeName: ticket.ProblemTypeName,
		Description:     ticket.Description,
		Deadline:        formatDeadline(ticket.Deadline),
		ExecutorID:      ticket.ExecutorID,
		ExecutorName:    ticket.ExecutorName,
		Status:          ticket.Status,
		StatusLabel:     label,
		StatusColor:     color,
		IsOverdue:       s.isOverdue(ticket),
		CompletedAt:     ticket.CompletedAt,
		CreatedAt:       ticket.CreatedAt,
		UpdatedAt:       ticket.UpdatedAt,
	}
}

// TicketListItem собирает элемент списка (без description).
func (s *Service) TicketListItem(ticket *Ticket) TicketListItemDTO {
	label, color := s.statusStyle(ticket.Status)
	return TicketListItemDTO{
		ID:              ticket.ID,
		BuildingID:      ticket.BuildingID,
		BuildingName:    ticket.BuildingName,
		ApplicantName:   ticket.ApplicantName,
		Cabinet:         ticket.Cabinet,
		ProblemTypeName: ticket.ProblemTypeName,
		Status:          ticket.Status,
		StatusLabel:     label,
		StatusColor:     color,
		Deadline:        formatDeadline(ticket.Deadline),
		ExecutorID:      ticket.ExecutorID,
		ExecutorName:    ticket.ExecutorName,
		IsOverdue:       s.isOverdue(ticket),
		CreatedAt:       ticket.CreatedAt,
	}
}

// PublicCard собирает публичную карточку без description.
func (s *Service) PublicCard(ticket *Ticket) PublicTicketCardDTO {
	label, color := s.statusStyle(ticket.Status)
	return PublicTicketCardDTO{
		ID:              ticket.ID,
		BuildingID:      ticket.BuildingID,
		BuildingName:    ticket.BuildingName,
		ApplicantName:   ticket.ApplicantName,
		Cabinet:         ticket.Cabinet,
		ProblemTypeName: ticket.ProblemTypeName,
		Status:          ticket.Status,
		StatusLabel:     label,
		StatusColor:     color,
		Deadline:        formatDeadline(ticket.Deadline),
		ExecutorName:    ticket.ExecutorName,
		CreatedAt:       ticket.CreatedAt,
	}
}

// PublicDetail собирает карточку по токену с комментариями.
func (s *Service) PublicDetail(ticket *Ticket, comments []*Comment) PublicTicketDetailDTO {
	commentDTOs := make([]CommentDTO, 0, len(comments))
	for _, comment := range comments {
		commentDTOs = append(commentDTOs, s.CommentDTO(comment))
	}
	return PublicTicketDetailDTO{
		PublicTicketCardDTO: s.PublicCard(ticket),
		Description:         ticket.Description,
		CompletedAt:         ticket.CompletedAt,
		IsOverdue:           s.isOverdue(ticket),
		Comments:            commentDTOs,
	}
}

// CommentDTO собирает комментарий. Вложения появятся после мержа Б3.
func (s *Service) CommentDTO(comment *Comment) CommentDTO {
	return CommentDTO{
		ID:           comment.ID,
		TicketID:     comment.TicketID,
		AuthorUserID: comment.AuthorUserID,
		AuthorName:   comment.AuthorName,
		Text:         comment.Body,
		CreatedAt:    comment.CreatedAt,
		Files:        []CommentFileDTO{},
	}
}
