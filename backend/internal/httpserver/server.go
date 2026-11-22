// Package httpserver собирает HTTP-роутер: middleware, health-check'и,
// Swagger UI и маршруты контракта. Реализованные домены подключаются своими
// handler'ами, остальные пока отдают 501 NOT_IMPLEMENTED (контракт заморожен).
package httpserver

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/config"
	"github.com/graffvishenka/polar-star-service/backend/internal/files"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/ratelimit"
)

// Лимиты AUTH-3 (значения из ТЗ).
const (
	publicTicketsLimit   = 5
	publicBuildingsLimit = 120
	rateLimitWindow      = 10 * time.Minute
)

// UsersAPI — администрирование пользователей и подбор исполнителей (Б2).
// Интерфейс объявлен в httpserver, чтобы пакет не зависел от internal/users
// и оставался тестируемым без БД.
type UsersAPI interface {
	List(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	ResetPassword(w http.ResponseWriter, r *http.Request)
	Assignable(w http.ResponseWriter, r *http.Request)
}

// BuildingsAPI — здания и склады: чтение всем авторизованным, запись,
// деактивация и физическое удаление — ADMIN (Б4, EQ-1, EQ-6).
type BuildingsAPI interface {
	ListBuildings(w http.ResponseWriter, r *http.Request)
	CreateBuilding(w http.ResponseWriter, r *http.Request)
	UpdateBuilding(w http.ResponseWriter, r *http.Request)
	DeleteBuilding(w http.ResponseWriter, r *http.Request)
	ListWarehouses(w http.ResponseWriter, r *http.Request)
	CreateWarehouse(w http.ResponseWriter, r *http.Request)
	UpdateWarehouse(w http.ResponseWriter, r *http.Request)
	DeleteWarehouse(w http.ResponseWriter, r *http.Request)
}

// EquipmentAPI — оборудование, перемещение и журнал (Б4, EQ-2, EQ-3, EQ-5,
// EQ-7). Массовое перемещение (EQ-4) реализует Б12.
type EquipmentAPI interface {
	List(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	Move(w http.ResponseWriter, r *http.Request)
	BulkMove(w http.ResponseWriter, r *http.Request)
	Movements(w http.ResponseWriter, r *http.Request)
}

// TicketsAPI — авторизованные операции с заявками (Б5).
type TicketsAPI interface {
	List(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	ListComments(w http.ResponseWriter, r *http.Request)
	CreateComment(w http.ResponseWriter, r *http.Request)
}

// PublicTicketsAPI — публичный периметр заявок (Б5).
type PublicTicketsAPI interface {
	PublicBuildings(w http.ResponseWriter, r *http.Request)
	PublicList(w http.ResponseWriter, r *http.Request)
	PublicCreate(w http.ResponseWriter, r *http.Request)
	PublicGet(w http.ResponseWriter, r *http.Request)
	PublicComments(w http.ResponseWriter, r *http.Request)
	PublicAddComment(w http.ResponseWriter, r *http.Request)
}

// KBAPI — база знаний (Б6, KB-1..KB-8): дерево, узлы, перемещение, видимость.
// Интерфейс объявлен здесь, чтобы httpserver не зависел от internal/kb и
// тестировался без БД. Публичная выдача медиа статьи идёт через
// `GET /api/public/files/{id}` (files.KBMediaAuthorizer, реализация — kb).
type KBAPI interface {
	Tree(w http.ResponseWriter, r *http.Request)
	PublicTree(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	PublicGet(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	Move(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	SetVisibility(w http.ResponseWriter, r *http.Request)
}

// SettingsAPI — настройки приложения, их публичное подмножество и праздники
// рабочего календаря (Б7, CAL-1..CAL-3).
type SettingsAPI interface {
	AdminGet(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	Public(w http.ResponseWriter, r *http.Request)
	ListHolidays(w http.ResponseWriter, r *http.Request)
	CreateHoliday(w http.ResponseWriter, r *http.Request)
	UpdateHoliday(w http.ResponseWriter, r *http.Request)
	DeleteHoliday(w http.ResponseWriter, r *http.Request)
}

// DictionariesAPI — справочники видов проблем и типов оборудования: CRUD для
// ADMIN и чтение для USER/ADMIN (Б7).
type DictionariesAPI interface {
	ListProblemTypes(w http.ResponseWriter, r *http.Request)
	CreateProblemType(w http.ResponseWriter, r *http.Request)
	UpdateProblemType(w http.ResponseWriter, r *http.Request)
	DeleteProblemType(w http.ResponseWriter, r *http.Request)
	ListEquipmentTypes(w http.ResponseWriter, r *http.Request)
	CreateEquipmentType(w http.ResponseWriter, r *http.Request)
	UpdateEquipmentType(w http.ResponseWriter, r *http.Request)
	DeleteEquipmentType(w http.ResponseWriter, r *http.Request)
	PublicProblemTypes(w http.ResponseWriter, r *http.Request)
}

// PushAPI — push-подписки и VAPID public key (Б8).
type PushAPI interface {
	PublicKey(w http.ResponseWriter, r *http.Request)
	Subscribe(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
}

// StatsAPI — статистика (Б9, STATS-1): личная и сводная для ADMIN. Выгрузка
// `.xlsx` — Б11 (DocumentsAPI).
type StatsAPI interface {
	Me(w http.ResponseWriter, r *http.Request)
	Admin(w http.ResponseWriter, r *http.Request)
}

// DocumentsAPI — серверные выгрузки Б11: Word-заявка (DOC-1), Excel-статистика
// (STATS-3) и ведомость помещения (EQ-8).
type DocumentsAPI interface {
	TicketDOCX(w http.ResponseWriter, r *http.Request)
	PublicTicketDOCX(w http.ResponseWriter, r *http.Request)
	StatsXLSX(w http.ResponseWriter, r *http.Request)
	InventoryXLSX(w http.ResponseWriter, r *http.Request)
}

// Deps — зависимости HTTP-слоя. Передаются из main, чтобы сервер не создавал
// подключения сам и оставался тестируемым.
type Deps struct {
	Config        *config.Config
	Logger        *slog.Logger
	DB            *sql.DB
	Clock         platform.Clock
	Calendar      platform.Calendar
	Notifier      platform.Notifier
	AuthService   *auth.Service
	AuthHandler   *auth.Handler
	Authenticator *auth.Authenticator
	UsersHandler  UsersAPI
	// FilesHandler — маршруты файлов и медиа (Б3). nil в тестах Б1: тогда
	// маршруты остаются заглушками 501.
	FilesHandler *files.Handler

	// Доменные handler'ы Б4. Если не переданы (unit-тесты HTTP-слоя),
	// соответствующие маршруты остаются заглушками 501.
	BuildingsHandler BuildingsAPI
	EquipmentHandler EquipmentAPI

	// Доменные handler'ы Б5; при nil маршруты заявок остаются заглушками 501.
	TicketsHandler       TicketsAPI
	PublicTicketsHandler PublicTicketsAPI

	// KBHandler — база знаний (Б6). nil — маршруты БЗ остаются заглушками 501.
	KBHandler KBAPI

	// Б7: настройки/праздники и справочники. nil — маршруты отдают 501.
	SettingsHandler     SettingsAPI
	DictionariesHandler DictionariesAPI

	// PushHandler — подписки устройств и VAPID public key (Б8); nil — 501.
	PushHandler PushAPI

	// StatsHandler — статистика (Б9). nil — маршруты статистики остаются
	// заглушками 501.
	StatsHandler StatsAPI

	// DocumentsHandler — Word/Excel-выгрузки Б11; nil — 501 на их маршрутах.
	DocumentsHandler DocumentsAPI
}

// Server — собранный HTTP-сервер.
type Server struct {
	deps                   Deps
	handler                http.Handler
	startedAt              time.Time
	publicTicketsLimiter   *ratelimit.Limiter
	publicBuildingsLimiter *ratelimit.Limiter
	// publicFilesLimiter — загрузка вложений анонимом (Б3): лимит публичных
	// заявок 5 запросов / 10 минут на IP (AUTH-3).
	publicFilesLimiter *ratelimit.Limiter
}

// New собирает роутер и middleware.
func New(deps Deps) *Server {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.Clock == nil {
		deps.Clock = platform.RealClock{}
	}
	s := &Server{
		deps:                   deps,
		startedAt:              deps.Clock.Now(),
		publicTicketsLimiter:   ratelimit.New(publicTicketsLimit, rateLimitWindow, deps.Clock),
		publicBuildingsLimiter: ratelimit.New(publicBuildingsLimit, rateLimitWindow, deps.Clock),
		publicFilesLimiter:     ratelimit.New(publicTicketsLimit, rateLimitWindow, deps.Clock),
	}
	s.handler = s.routes()
	return s
}

// Handler возвращает http.Handler для http.Server и тестов.
func (s *Server) Handler() http.Handler { return s.handler }
