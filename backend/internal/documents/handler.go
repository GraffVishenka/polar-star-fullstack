// Package documents формирует серверные выгрузки Б11: Word-заявку (DOC-1),
// Excel-статистику (STATS-3) и ведомость помещения (EQ-8). Генерация не
// зависит от сети и внешних шаблонизаторов; docx собирается из OOXML-частей,
// xlsx — через excelize.
package documents

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/equipment"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
	"github.com/graffvishenka/polar-star-service/backend/internal/stats"
	"github.com/graffvishenka/polar-star-service/backend/internal/tickets"
)

// TicketSource — чтение заявок с проверкой прав (реализует tickets.Service).
type TicketSource interface {
	Get(ctx context.Context, actor *auth.User, id int64) (*tickets.Ticket, error)
	Comments(ctx context.Context, actor *auth.User, ticketID int64, page, size int) (*tickets.CommentPage, error)
	PublicTicket(ctx context.Context, token string) (*tickets.Ticket, []*tickets.Comment, error)
	PublicComments(ctx context.Context, token string, page, size int) (*tickets.CommentPage, error)
	StatusLabel(status tickets.Status) string
}

// StatsSource — статистика и параметры периода (реализует stats.Service).
type StatsSource interface {
	Location() *time.Location
	ExportRows(ctx context.Context, actor *auth.User, query stats.Query) ([]stats.ExportRow, error)
	MyStats(ctx context.Context, actor *auth.User, query stats.Query) (*stats.Stats, error)
	AdminStats(ctx context.Context, actor *auth.User, query stats.Query) ([]*stats.Stats, error)
}

// InventorySource — склад и его оборудование (реализует equipment.Service).
type InventorySource interface {
	Warehouse(ctx context.Context, id int64) (*equipment.WarehouseInfo, error)
	List(ctx context.Context, filter equipment.ListFilter) (*equipment.ListResult, error)
}

// Options — настройки, которые зависят от Б7 (название приложения, время).
type Options struct {
	// AppName — название приложения из настроек (шапка документов).
	AppName func() string
	// Clock — источник времени для {{generated_at}} (NFR-7).
	Clock platform.Clock
	// UserFrom — источник пользователя; в API auth.UserFrom (для тестов).
	UserFrom func(context.Context) (*auth.User, bool)
}

// Handler — HTTP-слой выгрузок Б11.
type Handler struct {
	tickets   TicketSource
	stats     StatsSource
	inventory InventorySource
	appName   func() string
	clock     platform.Clock
	userFrom  func(context.Context) (*auth.User, bool)
	logger    *slog.Logger
}

// NewHandler создаёт handler выгрузок.
func NewHandler(tickets TicketSource, statsSource StatsSource, inventory InventorySource,
	opts Options, logger *slog.Logger) *Handler {
	if opts.Clock == nil {
		opts.Clock = platform.RealClock{}
	}
	if opts.AppName == nil {
		opts.AppName = func() string { return "Заявки · База знаний · Оборудование" }
	}
	if opts.UserFrom == nil {
		opts.UserFrom = auth.UserFrom
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		tickets: tickets, stats: statsSource, inventory: inventory,
		appName: opts.AppName, clock: opts.Clock, userFrom: opts.UserFrom, logger: logger,
	}
}

func (h *Handler) location() *time.Location {
	if h.stats != nil {
		if loc := h.stats.Location(); loc != nil {
			return loc
		}
	}
	return time.UTC
}

func (h *Handler) actor(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	user, ok := h.userFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return nil, false
	}
	return user, true
}
