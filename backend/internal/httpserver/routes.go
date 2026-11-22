package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/middleware"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// routes собирает дерево маршрутов по контракту PLAN §3.13.
//
// Реализованы в Б1: health, docs, auth (login/refresh/logout/me/password).
// Остальные маршруты зарегистрированы с правильными правами (401/403/428
// работают уже сейчас) и отдают 501 NOT_IMPLEMENTED до готовности своих блоков.
func (s *Server) routes() http.Handler {
	r := chi.NewRouter()

	// Порядок важен: request_id нужен recover/логам, CORS обрабатывает preflight
	// до rate limit, access-log видит итоговый статус и user_id.
	r.Use(middleware.RequestID)
	r.Use(middleware.Recover(s.deps.Logger))
	r.Use(middleware.AccessLog(s.deps.Logger, s.deps.Clock))
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(s.deps.Config.CORSOrigins))

	// Служебные и документация (NFR-4, Б1).
	r.Get("/health", s.health)
	r.Get("/health/ready", s.ready)
	r.Get("/docs", s.docs)
	r.Get("/docs/", s.docs)
	r.Get("/openapi.yaml", s.openapiSpec)

	r.Route("/api", func(api chi.Router) {
		// AUTH: вход, ротация сессии и выход не требуют access-токена
		// (refresh/logout работают по cookie, AUTH-2).
		api.Post("/auth/login", s.deps.AuthHandler.Login)
		api.Post("/auth/refresh", s.deps.AuthHandler.Refresh)
		api.Post("/auth/logout", s.deps.AuthHandler.Logout)

		// ПУБЛИЧНЫЙ ПЕРИМЕТР (PUB-1..PUB-7). Лимиты AUTH-3:
		// создание заявки 5/10 мин на IP, здания 120/10 мин на IP.
		api.With(middleware.RateLimit(s.publicBuildingsLimiter, middleware.ClientIPKey)).
			Get("/public/buildings", s.notImplemented)
		api.With(middleware.RateLimit(s.publicTicketsLimiter, middleware.ClientIPKey)).
			Post("/public/tickets", s.notImplemented)
		api.Get("/public/tickets", s.notImplemented)
		api.Get("/public/tickets/{token}", s.notImplemented)
		api.Post("/public/tickets/{token}/comments", s.notImplemented)
		api.Get("/public/tickets/{token}/export.docx", s.notImplemented)
		api.Get("/public/files/{id}", s.notImplemented)
		api.Get("/public/kb/tree", s.notImplemented)
		api.Get("/public/kb/nodes/{id}", s.notImplemented)

		// ЗАЩИЩЁННЫЕ МАРШРУТЫ (USER/ADMIN). requireAuth проверяет is_active в БД
		// на каждом запросе (AUTH-6), затем must_change_password (AUTH-4).
		api.Group(func(pr chi.Router) {
			pr.Use(s.deps.Authenticator.RequireAuth)
			pr.Use(auth.RequirePasswordChanged)

			// Профиль и смена пароля разрешены даже при must_change_password.
			pr.Get("/auth/me", s.deps.AuthHandler.Me)
			pr.Post("/auth/password", s.deps.AuthHandler.ChangePassword)

			s.authorizedRoutes(pr)

			// ADMIN-маршруты: requireRole("ADMIN") (AUTH-4, план §3.1).
			pr.Group(func(adm chi.Router) {
				adm.Use(s.deps.Authenticator.RequireRole(auth.RoleAdmin))
				s.adminRoutes(adm)
			})
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		apierr.Write(w, r, http.StatusNotFound, apierr.CodeNotFound, "Ресурс не найден", nil)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		apierr.Write(w, r, http.StatusMethodNotAllowed, apierr.CodeMethodNotAllowed, "Метод не поддерживается", nil)
	})

	return r
}

// authorizedRoutes — маршруты USER/ADMIN (заглушки Б1).
func (s *Server) authorizedRoutes(r chi.Router) {
	// Заявки (TICKET-*): USER видит только своё здание, ADMIN — все.
	r.Get("/tickets", s.notImplemented)
	r.Post("/tickets", s.notImplemented)
	r.Get("/tickets/{id}", s.notImplemented)
	r.Patch("/tickets/{id}", s.notImplemented)
	r.Get("/tickets/{id}/export.docx", s.notImplemented)
	r.Get("/tickets/{id}/comments", s.notImplemented)
	r.Post("/tickets/{id}/comments", s.notImplemented)

	// Файлы (Б3).
	r.Post("/files", s.notImplemented)
	r.Get("/files/{id}", s.notImplemented)

	// База знаний (Б6).
	r.Get("/kb/tree", s.notImplemented)
	r.Get("/kb/nodes/{id}", s.notImplemented)
	r.Post("/kb/nodes", s.notImplemented)
	r.Patch("/kb/nodes/{id}", s.notImplemented)
	r.Post("/kb/nodes/{id}/move", s.notImplemented)

	// Пользователи и статистика (Б2, Б9).
	r.Get("/users/assignable", s.usersAssignable)
	r.Get("/stats/me", s.notImplemented)
	r.Get("/stats/export.xlsx", s.notImplemented)

	// Здания и склады: чтение всем авторизованным (EQ-1, EQ-7, Б4).
	if bh := s.deps.BuildingsHandler; bh != nil {
		r.Get("/buildings", bh.ListBuildings)
		r.Get("/warehouses", bh.ListWarehouses)
	} else {
		r.Get("/buildings", s.notImplemented)
		r.Get("/warehouses", s.notImplemented)
	}

	// Оборудование (Б4/Б12).
	if eh := s.deps.EquipmentHandler; eh != nil {
		r.Get("/equipment", eh.List)
		r.Post("/equipment", eh.Create)
		// Статический сегмент регистрируется до параметра {id}, чтобы не было
		// неоднозначности при разборе пути. Массовое перемещение — Б12.
		r.Post("/equipment/bulk-move", s.notImplemented)
		r.Get("/equipment/{id}", eh.Get)
		r.Patch("/equipment/{id}", eh.Update)
		r.Post("/equipment/{id}/move", eh.Move)
		r.Get("/equipment/{id}/movements", eh.Movements)
	} else {
		r.Get("/equipment", s.notImplemented)
		r.Post("/equipment", s.notImplemented)
		r.Post("/equipment/bulk-move", s.notImplemented)
		r.Get("/equipment/{id}", s.notImplemented)
		r.Patch("/equipment/{id}", s.notImplemented)
		r.Post("/equipment/{id}/move", s.notImplemented)
		r.Get("/equipment/{id}/movements", s.notImplemented)
	}
	// Ведомость помещения (EQ-8) — блок Б6/Б10.
	r.Get("/warehouses/{id}/inventory.xlsx", s.notImplemented)

	// Push (Б8).
	r.Get("/push/public-key", s.notImplemented)
	r.Post("/push/subscriptions", s.notImplemented)
	r.Delete("/push/subscriptions/{id}", s.notImplemented)
}

// adminRoutes — маршруты только для ADMIN (PLAN §3.13).
func (s *Server) adminRoutes(r chi.Router) { // Заявки: удаление в корзину — только ADMIN (TICKET-12).
	r.Delete("/tickets/{id}", s.notImplemented)

	// БЗ: удаление и тумблер видимости — только ADMIN (KB-3, KB-7).
	r.Delete("/kb/nodes/{id}", s.notImplemented)
	r.Patch("/kb/nodes/{id}/visibility", s.notImplemented)

	// Пользователи (AUTH-4..AUTH-9, Б2).
	r.Get("/admin/users", s.usersList)
	r.Post("/admin/users", s.usersCreate)
	r.Patch("/admin/users/{id}", s.usersUpdate)
	r.Post("/admin/users/{id}/password", s.usersResetPassword)

	// Здания и склады: запись/деактивация — ADMIN (EQ-6, Б4).
	if bh := s.deps.BuildingsHandler; bh != nil {
		r.Post("/buildings", bh.CreateBuilding)
		r.Patch("/buildings/{id}", bh.UpdateBuilding)
		r.Delete("/buildings/{id}", bh.DeleteBuilding)
		r.Post("/warehouses", bh.CreateWarehouse)
		r.Patch("/warehouses/{id}", bh.UpdateWarehouse)
		r.Delete("/warehouses/{id}", bh.DeleteWarehouse)
	} else {
		r.Post("/buildings", s.notImplemented)
		r.Patch("/buildings/{id}", s.notImplemented)
		r.Delete("/buildings/{id}", s.notImplemented)
		r.Post("/warehouses", s.notImplemented)
		r.Patch("/warehouses/{id}", s.notImplemented)
		r.Delete("/warehouses/{id}", s.notImplemented)
	}

	// Оборудование: удаление — ADMIN (EQ-6, Б4).
	if eh := s.deps.EquipmentHandler; eh != nil {
		r.Delete("/equipment/{id}", eh.Delete)
	} else {
		r.Delete("/equipment/{id}", s.notImplemented)
	}

	// Настройки и справочники (Б7).
	r.Get("/admin/settings", s.notImplemented)
	r.Put("/admin/settings", s.notImplemented)
	r.Get("/admin/problem-types", s.notImplemented)
	r.Post("/admin/problem-types", s.notImplemented)
	r.Patch("/admin/problem-types/{id}", s.notImplemented)
	r.Delete("/admin/problem-types/{id}", s.notImplemented)
	r.Get("/admin/equipment-types", s.notImplemented)
	r.Post("/admin/equipment-types", s.notImplemented)
	r.Patch("/admin/equipment-types/{id}", s.notImplemented)
	r.Delete("/admin/equipment-types/{id}", s.notImplemented)
	r.Get("/admin/holidays", s.notImplemented)
	r.Post("/admin/holidays", s.notImplemented)
	r.Delete("/admin/holidays/{id}", s.notImplemented)

	// Статистика и корзина — ADMIN (STATS-1, TRASH-*).
	r.Get("/admin/stats", s.notImplemented)
	r.Get("/admin/trash", s.notImplemented)
	r.Post("/admin/trash/{type}/{id}/restore", s.notImplemented)
	r.Delete("/admin/trash/{type}/{id}", s.notImplemented)
}

// Обёртки Б2: пока UsersHandler не подключён (например, в тестах Б1),
// маршруты остаются замороженными заглушками 501. В проде main всегда
// передаёт реальный handler.
func (s *Server) usersList(w http.ResponseWriter, r *http.Request) {
	if s.deps.UsersHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.UsersHandler.List(w, r)
}

func (s *Server) usersCreate(w http.ResponseWriter, r *http.Request) {
	if s.deps.UsersHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.UsersHandler.Create(w, r)
}

func (s *Server) usersUpdate(w http.ResponseWriter, r *http.Request) {
	if s.deps.UsersHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.UsersHandler.Update(w, r)
}

func (s *Server) usersResetPassword(w http.ResponseWriter, r *http.Request) {
	if s.deps.UsersHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.UsersHandler.ResetPassword(w, r)
}

func (s *Server) usersAssignable(w http.ResponseWriter, r *http.Request) {
	if s.deps.UsersHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.UsersHandler.Assignable(w, r)
}
