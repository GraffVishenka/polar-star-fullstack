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
			Get("/public/buildings", s.publicBuildings)
		api.With(middleware.RateLimit(s.publicTicketsLimiter, middleware.ClientIPKey)).
			Post("/public/tickets", s.publicCreateTicket)
		api.Get("/public/tickets", s.publicListTickets)
		api.Get("/public/tickets/{token}", s.publicGetTicket)
		api.Get("/public/tickets/{token}/comments", s.publicListComments)
		api.Post("/public/tickets/{token}/comments", s.publicAddComment)
		// Алиас загрузки вложения анонимом (Ф5): токен заявки передаётся в
		// пути, внутри запроса транслируется в ?token= для files-слоя (Б3).
		api.With(middleware.RateLimit(s.publicFilesLimiter, middleware.ClientIPKey)).
			Post("/public/tickets/{token}/files", s.publicTicketUpload)
		api.Get("/public/tickets/{token}/export.docx", s.notImplemented)

		// Файлы анонимной заявки (Б3, TICKET-13/TICKET-14).
		// Чтение — по access_token заявки; загрузка вложения анонимом — тот же
		// лимит, что у публичных заявок (AUTH-3: 5 запросов / 10 минут на IP).
		if s.deps.FilesHandler != nil {
			api.Get("/public/files/{id}", s.deps.FilesHandler.PublicGet)
			api.Head("/public/files/{id}", s.deps.FilesHandler.PublicGet)
			api.With(middleware.RateLimit(s.publicFilesLimiter, middleware.ClientIPKey)).
				Post("/public/files", s.deps.FilesHandler.PublicUpload)
		} else {
			api.Get("/public/files/{id}", s.notImplemented)
			api.Post("/public/files", s.notImplemented)
		}

		// База знаний (Б6, KB-7/KB-8): дерево и статья для анонима. Медиа
		// публичной статьи отдаётся тем же `GET /api/public/files/{id}` без
		// токена (проверка KBMediaAuthorizer в files, реализация — kb).
		if s.deps.KBHandler != nil {
			api.Get("/public/kb/tree", s.deps.KBHandler.PublicTree)
			api.Get("/public/kb/nodes/{id}", s.deps.KBHandler.PublicGet)
		} else {
			api.Get("/public/kb/tree", s.notImplemented)
			api.Get("/public/kb/nodes/{id}", s.notImplemented)
		}

		// Публичные справочники и настройки (Б7): форма анонимной заявки и
		// публичный дашборд (TICKET-4, TICKET-5). Лимиты AUTH-3 к ним не
		// применяются (они заданы только для зданий и создания заявки).
		if dh := s.deps.DictionariesHandler; dh != nil {
			api.Get("/public/problem-types", dh.PublicProblemTypes)
		} else {
			api.Get("/public/problem-types", s.notImplemented)
		}
		if sh := s.deps.SettingsHandler; sh != nil {
			api.Get("/public/settings", sh.Public)
		} else {
			api.Get("/public/settings", s.notImplemented)
		}

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

// authorizedRoutes — маршруты USER/ADMIN.
func (s *Server) authorizedRoutes(r chi.Router) {
	// Заявки (TICKET-*): USER видит только своё здание, ADMIN — все.
	r.Get("/tickets", s.ticketsList)
	r.Post("/tickets", s.ticketsCreate)
	r.Get("/tickets/{id}", s.ticketsGet)
	r.Patch("/tickets/{id}", s.ticketsUpdate)
	r.Get("/tickets/{id}/export.docx", s.notImplemented)
	r.Get("/tickets/{id}/comments", s.ticketsListComments)
	r.Post("/tickets/{id}/comments", s.ticketsCreateComment)

	// Файлы (Б3): загрузка и выдача под аутентификацией; HEAD нужен для
	// корректной работы плееров и http.ServeContent.
	if s.deps.FilesHandler != nil {
		r.Post("/files", s.deps.FilesHandler.Upload)
		r.Get("/files/{id}", s.deps.FilesHandler.Get)
		r.Head("/files/{id}", s.deps.FilesHandler.Get)
	} else {
		r.Post("/files", s.notImplemented)
		r.Get("/files/{id}", s.notImplemented)
	}

	// База знаний (Б6): дерево, узлы, правка и перемещение — USER/ADMIN (KB-3).
	// Удаление и видимость — только ADMIN (см. adminRoutes).
	if kbHandler := s.deps.KBHandler; kbHandler != nil {
		r.Get("/kb/tree", kbHandler.Tree)
		r.Get("/kb/nodes/{id}", kbHandler.Get)
		r.Post("/kb/nodes", kbHandler.Create)
		r.Patch("/kb/nodes/{id}", kbHandler.Update)
		r.Post("/kb/nodes/{id}/move", kbHandler.Move)
	} else {
		r.Get("/kb/tree", s.notImplemented)
		r.Get("/kb/nodes/{id}", s.notImplemented)
		r.Post("/kb/nodes", s.notImplemented)
		r.Patch("/kb/nodes/{id}", s.notImplemented)
		r.Post("/kb/nodes/{id}/move", s.notImplemented)
	}

	// Пользователи и статистика (Б2, Б9).
	r.Get("/users/assignable", s.usersAssignable)
	r.Get("/stats/me", s.notImplemented)
	r.Get("/stats/export.xlsx", s.notImplemented)

	// Справочники: чтение активных — всем авторизованным (Б7). include_inactive
	// учитывается только для ADMIN (проверка в handler'е).
	if dh := s.deps.DictionariesHandler; dh != nil {
		r.Get("/problem-types", dh.ListProblemTypes)
		r.Get("/equipment-types", dh.ListEquipmentTypes)
	} else {
		r.Get("/problem-types", s.notImplemented)
		r.Get("/equipment-types", s.notImplemented)
	}

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
func (s *Server) adminRoutes(r chi.Router) {
	// Заявки: удаление в корзину — только ADMIN (TICKET-12).
	r.Delete("/tickets/{id}", s.ticketsDelete)

	// БЗ: удаление и тумблер видимости — только ADMIN (KB-3, KB-7, Б6).
	if kbHandler := s.deps.KBHandler; kbHandler != nil {
		r.Delete("/kb/nodes/{id}", kbHandler.Delete)
		r.Patch("/kb/nodes/{id}/visibility", kbHandler.SetVisibility)
	} else {
		r.Delete("/kb/nodes/{id}", s.notImplemented)
		r.Patch("/kb/nodes/{id}/visibility", s.notImplemented)
	}

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

	// Настройки и праздники (Б7, CAL-1..CAL-3). PATCH праздника — аддитивное
	// дополнение контракта: правка даты вместо удаления и создания заново.
	if sh := s.deps.SettingsHandler; sh != nil {
		r.Get("/admin/settings", sh.AdminGet)
		r.Put("/admin/settings", sh.Update)
		r.Get("/admin/holidays", sh.ListHolidays)
		r.Post("/admin/holidays", sh.CreateHoliday)
		r.Patch("/admin/holidays/{id}", sh.UpdateHoliday)
		r.Delete("/admin/holidays/{id}", sh.DeleteHoliday)
	} else {
		r.Get("/admin/settings", s.notImplemented)
		r.Put("/admin/settings", s.notImplemented)
		r.Get("/admin/holidays", s.notImplemented)
		r.Post("/admin/holidays", s.notImplemented)
		r.Patch("/admin/holidays/{id}", s.notImplemented)
		r.Delete("/admin/holidays/{id}", s.notImplemented)
	}

	// Справочники (Б7): удаление = деактивация (история сохраняется).
	if dh := s.deps.DictionariesHandler; dh != nil {
		r.Get("/admin/problem-types", dh.ListProblemTypes)
		r.Post("/admin/problem-types", dh.CreateProblemType)
		r.Patch("/admin/problem-types/{id}", dh.UpdateProblemType)
		r.Delete("/admin/problem-types/{id}", dh.DeleteProblemType)
		r.Get("/admin/equipment-types", dh.ListEquipmentTypes)
		r.Post("/admin/equipment-types", dh.CreateEquipmentType)
		r.Patch("/admin/equipment-types/{id}", dh.UpdateEquipmentType)
		r.Delete("/admin/equipment-types/{id}", dh.DeleteEquipmentType)
	} else {
		r.Get("/admin/problem-types", s.notImplemented)
		r.Post("/admin/problem-types", s.notImplemented)
		r.Patch("/admin/problem-types/{id}", s.notImplemented)
		r.Delete("/admin/problem-types/{id}", s.notImplemented)
		r.Get("/admin/equipment-types", s.notImplemented)
		r.Post("/admin/equipment-types", s.notImplemented)
		r.Patch("/admin/equipment-types/{id}", s.notImplemented)
		r.Delete("/admin/equipment-types/{id}", s.notImplemented)
	}

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

// Обёртки Б5: при отсутствии handler'ов (тесты других блоков) маршруты
// остаются заглушками 501; в проде main всегда передаёт tickets.Handler.
func (s *Server) ticketsList(w http.ResponseWriter, r *http.Request) {
	if s.deps.TicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.TicketsHandler.List(w, r)
}

func (s *Server) ticketsCreate(w http.ResponseWriter, r *http.Request) {
	if s.deps.TicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.TicketsHandler.Create(w, r)
}

func (s *Server) ticketsGet(w http.ResponseWriter, r *http.Request) {
	if s.deps.TicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.TicketsHandler.Get(w, r)
}

func (s *Server) ticketsUpdate(w http.ResponseWriter, r *http.Request) {
	if s.deps.TicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.TicketsHandler.Update(w, r)
}

func (s *Server) ticketsDelete(w http.ResponseWriter, r *http.Request) {
	if s.deps.TicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.TicketsHandler.Delete(w, r)
}

func (s *Server) ticketsListComments(w http.ResponseWriter, r *http.Request) {
	if s.deps.TicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.TicketsHandler.ListComments(w, r)
}

func (s *Server) ticketsCreateComment(w http.ResponseWriter, r *http.Request) {
	if s.deps.TicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.TicketsHandler.CreateComment(w, r)
}

func (s *Server) publicBuildings(w http.ResponseWriter, r *http.Request) {
	if s.deps.PublicTicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.PublicTicketsHandler.PublicBuildings(w, r)
}

func (s *Server) publicListTickets(w http.ResponseWriter, r *http.Request) {
	if s.deps.PublicTicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.PublicTicketsHandler.PublicList(w, r)
}

func (s *Server) publicCreateTicket(w http.ResponseWriter, r *http.Request) {
	if s.deps.PublicTicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.PublicTicketsHandler.PublicCreate(w, r)
}

func (s *Server) publicGetTicket(w http.ResponseWriter, r *http.Request) {
	if s.deps.PublicTicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.PublicTicketsHandler.PublicGet(w, r)
}

func (s *Server) publicListComments(w http.ResponseWriter, r *http.Request) {
	if s.deps.PublicTicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.PublicTicketsHandler.PublicComments(w, r)
}

func (s *Server) publicAddComment(w http.ResponseWriter, r *http.Request) {
	if s.deps.PublicTicketsHandler == nil {
		s.notImplemented(w, r)
		return
	}
	s.deps.PublicTicketsHandler.PublicAddComment(w, r)
}

// publicTicketUpload — алиас POST /api/public/tickets/{token}/files (Ф5):
// files-слой принимает токен в query, поэтому переносим {token} из пути.
func (s *Server) publicTicketUpload(w http.ResponseWriter, r *http.Request) {
	if s.deps.FilesHandler == nil {
		s.notImplemented(w, r)
		return
	}
	if token := chi.URLParam(r, "token"); token != "" {
		query := r.URL.Query()
		query.Set("token", token)
		r.URL.RawQuery = query.Encode()
	}
	s.deps.FilesHandler.PublicUpload(w, r)
}
