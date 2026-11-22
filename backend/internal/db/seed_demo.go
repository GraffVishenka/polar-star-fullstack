package db

// Демонстрационные данные (Б13, PLAN §9): наполняют стенд так, чтобы все
// экраны были непустыми — заявки во всех статусах, оборудование с историей,
// дерево БЗ глубиной 5+, скрытые разделы, праздники и настройки.
//
// Сид идемпотентен: каждая сущность ищется по детерминированному признаку
// (имя здания, логин, регистрационный номер, маркер «[демо]» в описании
// заявки, префикс «Демо:» в заголовке узла БЗ). Повторный запуск не создаёт
// дубликаты и не перезаписывает изменённые администратором данные, кроме
// настроек: их демо-значения задаются только при первом создании строки.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
)

// DemoOptions — параметры демо-сида.
type DemoOptions struct {
	// Password — пароль всех демо-пользователей (SEED_DEMO_PASSWORD).
	Password string
	// Now — точка отсчёта дат (в тестах — фиксированная).
	Now time.Time
}

// defaultDemoPassword совпадает с config.defaultSeedDemoPassword.
const defaultDemoPassword = "Demo_12345"

// demoPasswordHint — как логируется пароль демо-учёток (без значения в логах
// уровня info он не пишется; здесь он нужен для приёмки стенда).
const demoPasswordHint = "SEED_DEMO_PASSWORD (по умолчанию Demo_12345)"

// demoBuilding — здание демо-стенда.
type demoBuilding struct {
	Name     string
	Address  string
	IsActive bool
}

// demoUser — демо-пользователь. BuildingIndex < 0 — ADMIN без здания.
type demoUser struct {
	Login         string
	FullName      string
	Role          auth.Role
	BuildingIndex int
}

// demoWarehouse — склад/кабинет здания.
type demoWarehouse struct {
	BuildingIndex int
	Name          string
}

// demoTicket — заявка демо-стенда.
type demoTicket struct {
	BuildingIndex  int
	ProblemIndex   int
	ExecutorIndex  int // индекс в списке пользователей (USER здания), -1 — без исполнителя
	CreatedByIndex int // автор из числа пользователей здания, -1 — анонимная
	Status         string
	ApplicantName  string
	Cabinet        string
	Description    string
	CreatedAt      time.Time
	Deadline       *time.Time
	CompletedAt    *time.Time
}

// demoEquipment — единица оборудования.
type demoEquipment struct {
	Name               string
	TypeIndex          int
	SerialNumber       string
	RegistrationNumber string
	WarehouseIndex     int
	CreatedAt          time.Time
}

// demoMovement — запись журнала перемещений.
type demoMovement struct {
	EquipmentIndex int
	FromIndex      int
	ToIndex        int
	ByUserIndex    int
	MovedAt        time.Time
	Comment        string
}

// demoNode — узел базы знаний. ParentIndex < 0 — корень.
type demoNode struct {
	Title         string
	ContentHTML   string
	ParentIndex   int
	SortOrder     int
	HiddenForAnon bool
}

// demoData — полный план демо-данных, построенный без обращения к БД.
type demoData struct {
	Buildings  []demoBuilding
	Users      []demoUser
	Warehouses []demoWarehouse
	Tickets    []demoTicket
	Equipment  []demoEquipment
	Movements  []demoMovement
	Nodes      []demoNode
	Holidays   []time.Time
}

// demoProblemTypes — виды проблем, согласованные с базовым сидом.
var demoProblemTypes = []string{
	"Не работает оборудование",
	"Проблемы с доступом",
	"Мебель и помещения",
	"Прочее",
}

// demoEquipmentTypes — типы оборудования из базового сида.
var demoEquipmentTypes = []string{
	"Компьютер",
	"Монитор",
	"Принтер",
	"Сканер",
	"Сетевое оборудование",
	"Прочее",
}

// buildDemoData строит план демо-данных относительно момента now.
//
// Чистая функция: используется и сидом, и unit-тестом инвариантов (4 здания,
// 12 пользователей, 50 заявок, 25 единиц и 25 узлов БЗ, глубина ≥5, 2 скрытых
// корня, 5 праздников).
func buildDemoData(now time.Time) demoData {
	now = now.UTC()
	day := func(offset int) time.Time {
		return now.AddDate(0, 0, offset)
	}
	dateOnly := func(t time.Time) *time.Time {
		value := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		return &value
	}
	atHour := func(t time.Time, hour int) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, time.UTC)
	}

	data := demoData{
		Buildings: []demoBuilding{
			{Name: "Демо-корпус А", Address: "ул. Ленина, 1", IsActive: true},
			{Name: "Демо-корпус Б", Address: "ул. Ленина, 2", IsActive: true},
			{Name: "Демо-корпус В", Address: "ул. Мира, 3", IsActive: true},
			{Name: "Демо-корпус Г (неактивное)", Address: "ул. Мира, 4", IsActive: false},
		},
		Users: []demoUser{
			{Login: "demo.admin1", FullName: "Демо Администратор Один", Role: auth.RoleAdmin, BuildingIndex: -1},
			{Login: "demo.admin2", FullName: "Демо Администратор Два", Role: auth.RoleAdmin, BuildingIndex: -1},
			// USER по зданиям: 4 в корпусе А, по 3 в корпусах Б и В.
			{Login: "demo.user01", FullName: "Иванов Иван", Role: auth.RoleUser, BuildingIndex: 0},
			{Login: "demo.user02", FullName: "Петров Пётр", Role: auth.RoleUser, BuildingIndex: 0},
			{Login: "demo.user03", FullName: "Сидорова Анна", Role: auth.RoleUser, BuildingIndex: 0},
			{Login: "demo.user04", FullName: "Кузнецов Олег", Role: auth.RoleUser, BuildingIndex: 0},
			{Login: "demo.user05", FullName: "Смирнова Ольга", Role: auth.RoleUser, BuildingIndex: 1},
			{Login: "demo.user06", FullName: "Волков Дмитрий", Role: auth.RoleUser, BuildingIndex: 1},
			{Login: "demo.user07", FullName: "Морозова Екатерина", Role: auth.RoleUser, BuildingIndex: 1},
			{Login: "demo.user08", FullName: "Новиков Артём", Role: auth.RoleUser, BuildingIndex: 2},
			{Login: "demo.user09", FullName: "Фёдорова Мария", Role: auth.RoleUser, BuildingIndex: 2},
			{Login: "demo.user10", FullName: "Егоров Никита", Role: auth.RoleUser, BuildingIndex: 2},
		},
		Warehouses: []demoWarehouse{
			{BuildingIndex: 0, Name: "Кабинет 101"},
			{BuildingIndex: 0, Name: "Склад ИТ"},
			{BuildingIndex: 0, Name: "Кабинет 205"},
			{BuildingIndex: 1, Name: "Кабинет 310"},
			{BuildingIndex: 1, Name: "Склад Б"},
			{BuildingIndex: 2, Name: "Кабинет 12"},
			{BuildingIndex: 2, Name: "Кабинет 14"},
			{BuildingIndex: 3, Name: "Архив (неактивное здание)"},
		},
	}

	// --- Заявки: 50 штук, все статусы, разные даты, часть done с просрочкой. ---
	cabinets := []string{"101", "102", "205", "310", "12", "14", "Склад"}
	applicants := []string{
		"Соколов А.", "Алексеева И.", "Григорьев П.", "Жукова Н.",
		"Титов С.", "Орлова Д.", "Крылов В.", "Макарова Л.",
	}
	problemTexts := []string{
		"Не включается после отключения питания",
		"Не открывается дверь по пропуску",
		"Сломан замок на шкафу",
		"Требуется консультация по использованию",
	}
	// Распределение статусов: 20 done, 12 new, 10 in_progress, 8 deferred.
	statuses := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		switch {
		case i < 20:
			statuses = append(statuses, "done")
		case i < 32:
			statuses = append(statuses, "new")
		case i < 42:
			statuses = append(statuses, "in_progress")
		default:
			statuses = append(statuses, "deferred")
		}
	}
	for i := 0; i < 50; i++ {
		buildingIndex := i % 3
		// Просроченные done: 6 из 20 завершены позже дедлайна, остальные — в срок.
		created := atHour(day(-(i%45 + 2)), 8+(i%8))
		var deadline *time.Time
		var completed *time.Time
		switch statuses[i] {
		case "done":
			completedAt := created.Add(time.Duration(20+i%40) * time.Hour)
			completed = &completedAt
			// Часть завершённых — с просрочкой: дедлайн на следующий день,
			// длительность больше суток гарантирует выход за его дату.
			if i%3 == 0 {
				deadline = dateOnly(created.AddDate(0, 0, 1))
			} else {
				deadline = dateOnly(created.AddDate(0, 0, 2))
				if completedAt.After(deadline.Add(24 * time.Hour)) {
					// Гарантируем «в срок»: завершение в пределах дедлайна.
					completedAt = deadline.Add(9 * time.Hour)
					completed = &completedAt
				}
			}
		default:
			if i%4 != 0 {
				deadline = dateOnly(created.AddDate(0, 0, 2))
			}
		}
		executor := -1
		createdBy := -1
		if statuses[i] == "done" || statuses[i] == "in_progress" || statuses[i] == "deferred" {
			executor = 2 + (i % 10) // индекс USER в списке пользователей
		}
		if i%5 != 0 {
			createdBy = 2 + (i % 10)
		}
		data.Tickets = append(data.Tickets, demoTicket{
			BuildingIndex:  buildingIndex,
			ProblemIndex:   i % len(demoProblemTypes),
			ExecutorIndex:  executor,
			CreatedByIndex: createdBy,
			Status:         statuses[i],
			ApplicantName:  applicants[i%len(applicants)],
			Cabinet:        cabinets[i%len(cabinets)],
			Description:    fmt.Sprintf("[демо] %s, №%d", problemTexts[i%len(problemTexts)], i+1),
			CreatedAt:      created,
			Deadline:       deadline,
			CompletedAt:    completed,
		})
	}

	// --- Оборудование: 25 единиц по 8 складам, у части — история перемещений.
	names := []string{
		"Ноутбук", "Монитор", "Принтер", "Сканер", "Коммутатор",
	}
	for i := 0; i < 25; i++ {
		data.Equipment = append(data.Equipment, demoEquipment{
			Name:               fmt.Sprintf("%s %d", names[i%len(names)], i+1),
			TypeIndex:          i % len(demoEquipmentTypes),
			SerialNumber:       fmt.Sprintf("DEMO-SN-%03d", i+1),
			RegistrationNumber: fmt.Sprintf("DEMO-EQ-%03d", i+1),
			WarehouseIndex:     i % len(data.Warehouses),
			CreatedAt:          day(-(60 - i)),
		})
	}
	for i := 0; i < 12; i++ {
		data.Movements = append(data.Movements, demoMovement{
			EquipmentIndex: i,
			FromIndex:      i % len(data.Warehouses),
			ToIndex:        (i + 1) % len(data.Warehouses),
			ByUserIndex:    2 + (i % 10),
			MovedAt:        day(-(30 - i)),
			Comment:        "Плановое перемещение",
		})
	}
	for i := 12; i < 16; i++ {
		data.Movements = append(data.Movements, demoMovement{
			EquipmentIndex: i,
			FromIndex:      (i + 1) % len(data.Warehouses),
			ToIndex:        (i + 2) % len(data.Warehouses),
			ByUserIndex:    2 + (i % 10),
			MovedAt:        day(-(20 - i)),
			Comment:        "Повторное перемещение",
		})
	}

	// --- База знаний: 25 узлов, 4 корня (2 скрытых), глубина до 6 уровней. ---
	// Индекс в срезе = порядковый номер узла; parent -1 — корень.
	nodes := []demoNode{
		{Title: "Демо: Инструкции", ParentIndex: -1, SortOrder: 10},
		{Title: "Демо: Скрытый раздел", ParentIndex: -1, SortOrder: 20, HiddenForAnon: true},
		{Title: "Демо: Оборудование", ParentIndex: -1, SortOrder: 30},
		{Title: "Демо: Архив (скрытый)", ParentIndex: -1, SortOrder: 40, HiddenForAnon: true},
		{Title: "Демо: Начало работы", ParentIndex: 0, SortOrder: 10},
		{Title: "Демо: Учётная запись", ParentIndex: 4, SortOrder: 10},
		{Title: "Демо: Смена пароля", ParentIndex: 5, SortOrder: 10},
		{Title: "Демо: Восстановление доступа", ParentIndex: 6, SortOrder: 10},
		{Title: "Демо: Заявки", ParentIndex: 7, SortOrder: 10},
		{Title: "Демо: Порядок обработки", ParentIndex: 1, SortOrder: 10},
		{Title: "Демо: Эскалация", ParentIndex: 9, SortOrder: 10},
		{Title: "Демо: Сроки", ParentIndex: 10, SortOrder: 10},
		{Title: "Демо: Просрочка", ParentIndex: 11, SortOrder: 10},
		{Title: "Демо: Компьютеры", ParentIndex: 2, SortOrder: 10},
		{Title: "Демо: Мониторы", ParentIndex: 13, SortOrder: 10},
		{Title: "Демо: Принтеры", ParentIndex: 14, SortOrder: 10},
		{Title: "Демо: Сканеры", ParentIndex: 3, SortOrder: 10},
		{Title: "Демо: Сеть", ParentIndex: 16, SortOrder: 10},
		{Title: "Демо: Коммутаторы", ParentIndex: 17, SortOrder: 10},
		{Title: "Демо: Кабели", ParentIndex: 18, SortOrder: 10},
		{Title: "Демо: Маркировка", ParentIndex: 19, SortOrder: 10},
		{Title: "Демо: Частые вопросы", ParentIndex: 0, SortOrder: 20},
		{Title: "Демо: Контакты", ParentIndex: 2, SortOrder: 20},
		{Title: "Демо: Регламент", ParentIndex: 1, SortOrder: 20},
		{Title: "Демо: Приложения", ParentIndex: 2, SortOrder: 30},
	}
	for i := range nodes {
		nodes[i].ContentHTML = fmt.Sprintf(
			"<h2>%s</h2><p>Демонстрационная статья %d. Текст для проверки дерева базы знаний и публичного доступа.</p>",
			nodes[i].Title, i+1)
	}
	data.Nodes = nodes

	// --- Праздники: 5 дат текущего года (идемпотентно по UNIQUE даты). ---
	year := now.Year()
	for _, date := range [][2]int{{1, 1}, {1, 7}, {3, 8}, {5, 1}, {5, 9}} {
		data.Holidays = append(data.Holidays,
			time.Date(year, time.Month(date[0]), date[1], 0, 0, 0, 0, time.UTC))
	}

	return data
}

// SeedDemo идемпотентно создаёт демонстрационные данные (Б13).
func SeedDemo(ctx context.Context, pool *sql.DB, logger *slog.Logger, opts DemoOptions) error {
	if logger == nil {
		logger = slog.Default()
	}
	password := opts.Password
	if password == "" {
		password = defaultDemoPassword
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	data := buildDemoData(now)

	// Настройки демо-стенда: значения по умолчанию (Пн–Сб, 2 дня, 10:00).
	// ON DUPLICATE KEY UPDATE id=id не перезаписывает настройки, изменённые
	// администратором при повторном запуске.
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO settings
			(id, app_name, deadline_days, workdays, timezone, deadline_reminder_time,
			 push_enabled, status_labels, status_colors)
		VALUES (1, ?, 2, ?, 'Asia/Yekaterinburg', '10:00', 1, ?, ?)
		ON DUPLICATE KEY UPDATE id = id`,
		defaultAppName, `[1,2,3,4,5,6]`, defaultStatusLabelsJSON, defaultStatusColorsJSON); err != nil {
		return fmt.Errorf("demo seed: настройки: %w", err)
	}

	buildingIDs := make([]int64, len(data.Buildings))
	for i, building := range data.Buildings {
		id, err := ensureBuilding(ctx, pool, building)
		if err != nil {
			return err
		}
		buildingIDs[i] = id
	}

	warehouseIDs := make([]int64, len(data.Warehouses))
	for i, warehouse := range data.Warehouses {
		id, err := ensureWarehouse(ctx, pool, buildingIDs[warehouse.BuildingIndex], warehouse.Name)
		if err != nil {
			return err
		}
		warehouseIDs[i] = id
	}

	// Пароль хешируется один раз: argon2id дорог, а пароль у демо-учёток общий.
	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("demo seed: хеширование пароля: %w", err)
	}
	userIDs := make([]int64, len(data.Users))
	createdUsers := 0
	for i, user := range data.Users {
		var buildingID any
		if user.BuildingIndex >= 0 {
			buildingID = buildingIDs[user.BuildingIndex]
		}
		id, created, err := ensureUser(ctx, pool, user, buildingID, hash)
		if err != nil {
			return err
		}
		userIDs[i] = id
		if created {
			createdUsers++
		}
	}

	if err := ensureDemoProblemTypes(ctx, pool); err != nil {
		return err
	}
	problemTypeIDs, err := lookupIDs(ctx, pool, "problem_types", demoProblemTypes)
	if err != nil {
		return err
	}
	equipmentTypeIDs, err := lookupIDs(ctx, pool, "equipment_types", demoEquipmentTypes)
	if err != nil {
		return err
	}

	ticketsCreated, err := ensureDemoTickets(ctx, pool, data, buildingIDs, userIDs, problemTypeIDs)
	if err != nil {
		return err
	}
	equipmentCreated, movementsCreated, err := ensureDemoEquipment(ctx, pool, data, warehouseIDs, userIDs, equipmentTypeIDs)
	if err != nil {
		return err
	}
	nodesCreated, err := ensureDemoNodes(ctx, pool, data.Nodes, userIDs)
	if err != nil {
		return err
	}
	holidaysCreated, err := ensureDemoHolidays(ctx, pool, data.Holidays)
	if err != nil {
		return err
	}

	logger.Info("demo seed: план", "summary", demoSummary(data))
	logger.Info("demo seed: готово",
		"buildings", len(data.Buildings),
		"users_created", createdUsers,
		"tickets_created", ticketsCreated,
		"equipment_created", equipmentCreated,
		"movements_created", movementsCreated,
		"kb_nodes_created", nodesCreated,
		"holidays_created", holidaysCreated)
	logger.Info("demo seed: учётные записи (пароль общий)",
		"password", demoPasswordHint,
		"admin_1", "demo.admin1", "admin_2", "demo.admin2",
		"users", "demo.user01 … demo.user10")
	for _, user := range data.Users {
		building := "—"
		if user.BuildingIndex >= 0 {
			building = data.Buildings[user.BuildingIndex].Name
		}
		logger.Info("demo seed: пользователь",
			"login", user.Login, "role", string(user.Role), "building", building)
	}
	return nil
}

// ensureBuilding возвращает id здания по имени, создавая отсутствующее.
func ensureBuilding(ctx context.Context, pool *sql.DB, building demoBuilding) (int64, error) {
	var id int64
	err := pool.QueryRowContext(ctx, `SELECT id FROM buildings WHERE name = ? LIMIT 1`, building.Name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("demo seed: поиск здания %q: %w", building.Name, err)
	}
	res, err := pool.ExecContext(ctx,
		`INSERT INTO buildings (name, address, is_active) VALUES (?, ?, ?)`,
		building.Name, building.Address, building.IsActive)
	if err != nil {
		return 0, fmt.Errorf("demo seed: создание здания %q: %w", building.Name, err)
	}
	return res.LastInsertId()
}

// ensureWarehouse возвращает id склада, создавая отсутствующий (имя уникально
// в рамках здания).
func ensureWarehouse(ctx context.Context, pool *sql.DB, buildingID int64, name string) (int64, error) {
	var id int64
	err := pool.QueryRowContext(ctx,
		`SELECT id FROM warehouses WHERE building_id = ? AND name = ? AND deleted_at IS NULL LIMIT 1`,
		buildingID, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("demo seed: поиск склада %q: %w", name, err)
	}
	res, err := pool.ExecContext(ctx,
		`INSERT INTO warehouses (building_id, name) VALUES (?, ?)`, buildingID, name)
	if err != nil {
		return 0, fmt.Errorf("demo seed: создание склада %q: %w", name, err)
	}
	return res.LastInsertId()
}

// ensureUser создаёт пользователя, если логин ещё не занят. Возвращает id и
// признак создания.
func ensureUser(ctx context.Context, pool *sql.DB, user demoUser, buildingID any, hash string) (int64, bool, error) {
	var id int64
	err := pool.QueryRowContext(ctx, `SELECT id FROM users WHERE login = ? LIMIT 1`, user.Login).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("demo seed: поиск пользователя %q: %w", user.Login, err)
	}
	res, err := pool.ExecContext(ctx, `
		INSERT INTO users (login, full_name, role, password_hash, building_id,
		                   is_active, must_change_password, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, 0, UTC_TIMESTAMP(), UTC_TIMESTAMP())`,
		user.Login, user.FullName, string(user.Role), hash, buildingID)
	if err != nil {
		return 0, false, fmt.Errorf("demo seed: создание пользователя %q: %w", user.Login, err)
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, false, fmt.Errorf("demo seed: id пользователя %q: %w", user.Login, err)
	}
	return id, true, nil
}

// ensureDemoProblemTypes гарантирует наличие видов проблем, если базовый сид
// не запускался.
func ensureDemoProblemTypes(ctx context.Context, pool *sql.DB) error {
	return seedDictionary(ctx, pool, "problem_types", demoProblemTypes)
}

// lookupIDs возвращает id справочника по именам (в том же порядке).
func lookupIDs(ctx context.Context, pool *sql.DB, table string, names []string) ([]int64, error) {
	ids := make([]int64, len(names))
	for i, name := range names {
		// Имя таблицы — константа из кода, не пользовательский ввод.
		query := fmt.Sprintf(`SELECT id FROM %s WHERE name = ? LIMIT 1`, table)
		if err := pool.QueryRowContext(ctx, query, name).Scan(&ids[i]); err != nil {
			return nil, fmt.Errorf("demo seed: id справочника %s(%q): %w", table, name, err)
		}
	}
	return ids, nil
}

// ensureDemoTickets создаёт 50 заявок, если демо-заявок ещё нет. Маркер
// «[демо]» в описании делает повторный запуск идемпотентным.
func ensureDemoTickets(ctx context.Context, pool *sql.DB, data demoData,
	buildingIDs, userIDs, problemTypeIDs []int64) (int, error) {
	var existing int
	if err := pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE description LIKE '%[демо]%'`).Scan(&existing); err != nil {
		return 0, fmt.Errorf("demo seed: проверка заявок: %w", err)
	}
	if existing > 0 {
		return 0, nil
	}

	created := 0
	for _, ticket := range data.Tickets {
		var executorID, createdByID any
		if ticket.ExecutorIndex >= 0 && ticket.ExecutorIndex < len(userIDs) {
			executorID = userIDs[ticket.ExecutorIndex]
		}
		if ticket.CreatedByIndex >= 0 && ticket.CreatedByIndex < len(userIDs) {
			createdByID = userIDs[ticket.CreatedByIndex]
		}
		// Проблема и здание согласованы по индексу.
		problemTypeID := problemTypeIDs[ticket.ProblemIndex%len(problemTypeIDs)]
		if _, err := pool.ExecContext(ctx, `
			INSERT INTO tickets
				(building_id, applicant_name, cabinet, problem_type_id, description,
				 deadline, executor_id, status, access_token_hash, created_by_user_id,
				 completed_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, ?)`,
			buildingIDs[ticket.BuildingIndex], ticket.ApplicantName, ticket.Cabinet,
			problemTypeID, ticket.Description, ticket.Deadline, executorID, ticket.Status,
			createdByID, ticket.CompletedAt, ticket.CreatedAt, ticket.CreatedAt); err != nil {
			return 0, fmt.Errorf("demo seed: создание заявки: %w", err)
		}
		created++
	}
	return created, nil
}

// ensureDemoEquipment создаёт 25 единиц и историю перемещений. Идемпотентность
// по регистрационным номерам DEMO-EQ-*.
func ensureDemoEquipment(ctx context.Context, pool *sql.DB, data demoData,
	warehouseIDs, userIDs, equipmentTypeIDs []int64) (int, int, error) {
	var existing int
	if err := pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM equipment WHERE registration_number LIKE 'DEMO-EQ-%'`).Scan(&existing); err != nil {
		return 0, 0, fmt.Errorf("demo seed: проверка оборудования: %w", err)
	}
	if existing > 0 {
		return 0, 0, nil
	}

	equipmentIDs := make([]int64, len(data.Equipment))
	for i, item := range data.Equipment {
		res, err := pool.ExecContext(ctx, `
			INSERT INTO equipment
				(name, equipment_type_id, serial_number, registration_number, warehouse_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			item.Name, equipmentTypeIDs[item.TypeIndex%len(equipmentTypeIDs)],
			item.SerialNumber, item.RegistrationNumber,
			warehouseIDs[item.WarehouseIndex], item.CreatedAt, item.CreatedAt)
		if err != nil {
			return 0, 0, fmt.Errorf("demo seed: создание оборудования: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, 0, fmt.Errorf("demo seed: id оборудования: %w", err)
		}
		equipmentIDs[i] = id
	}

	movements := 0
	for _, movement := range data.Movements {
		if movement.ByUserIndex < 0 || movement.ByUserIndex >= len(userIDs) {
			continue
		}
		if _, err := pool.ExecContext(ctx, `
			INSERT INTO equipment_movements
				(equipment_id, from_warehouse_id, to_warehouse_id, moved_by_user_id, moved_at, comment)
			VALUES (?, ?, ?, ?, ?, ?)`,
			equipmentIDs[movement.EquipmentIndex],
			warehouseIDs[movement.FromIndex], warehouseIDs[movement.ToIndex],
			userIDs[movement.ByUserIndex], movement.MovedAt, movement.Comment); err != nil {
			return 0, 0, fmt.Errorf("demo seed: журнал перемещений: %w", err)
		}
		// Итоговый склад единицы = склад последнего перемещения.
		if _, err := pool.ExecContext(ctx,
			`UPDATE equipment SET warehouse_id = ? WHERE id = ?`,
			warehouseIDs[movement.ToIndex], equipmentIDs[movement.EquipmentIndex]); err != nil {
			return 0, 0, fmt.Errorf("demo seed: обновление склада: %w", err)
		}
		movements++
	}
	return len(equipmentIDs), movements, nil
}

// ensureDemoNodes создаёт дерево БЗ, если узлов с префиксом «Демо:» ещё нет.
func ensureDemoNodes(ctx context.Context, pool *sql.DB, nodes []demoNode, userIDs []int64) (int, error) {
	var existing int
	if err := pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM kb_nodes WHERE title LIKE 'Демо:%'`).Scan(&existing); err != nil {
		return 0, fmt.Errorf("demo seed: проверка узлов БЗ: %w", err)
	}
	if existing > 0 {
		return 0, nil
	}

	authorID := any(nil)
	if len(userIDs) > 0 {
		authorID = userIDs[0]
	}
	ids := make([]int64, len(nodes))
	for i, node := range nodes {
		var parentID any
		if node.ParentIndex >= 0 {
			parentID = ids[node.ParentIndex]
		}
		res, err := pool.ExecContext(ctx, `
			INSERT INTO kb_nodes
				(title, content_html, parent_id, sort_order, is_hidden_for_anon,
				 created_by, updated_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP(), UTC_TIMESTAMP())`,
			node.Title, node.ContentHTML, parentID, node.SortOrder, node.HiddenForAnon,
			authorID, authorID)
		if err != nil {
			return 0, fmt.Errorf("demo seed: создание узла БЗ %q: %w", node.Title, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("demo seed: id узла БЗ: %w", err)
		}
		ids[i] = id
	}
	return len(ids), nil
}

// ensureDemoHolidays вставляет праздники (UNIQUE по дате — INSERT IGNORE).
func ensureDemoHolidays(ctx context.Context, pool *sql.DB, dates []time.Time) (int, error) {
	created := 0
	for _, date := range dates {
		res, err := pool.ExecContext(ctx,
			`INSERT IGNORE INTO holidays (holiday_date) VALUES (?)`, date)
		if err != nil {
			return 0, fmt.Errorf("demo seed: праздник %s: %w", date.Format("2006-01-02"), err)
		}
		if affected, err := res.RowsAffected(); err == nil && affected > 0 {
			created++
		}
	}
	return created, nil
}

// demoSummary — краткое описание демо-данных для логов приёмки.
func demoSummary(data demoData) string {
	statuses := map[string]int{}
	for _, ticket := range data.Tickets {
		statuses[ticket.Status]++
	}
	hidden := 0
	for _, node := range data.Nodes {
		if node.ParentIndex < 0 && node.HiddenForAnon {
			hidden++
		}
	}
	return strings.Join([]string{
		fmt.Sprintf("зданий=%d", len(data.Buildings)),
		fmt.Sprintf("пользователей=%d", len(data.Users)),
		fmt.Sprintf("складов=%d", len(data.Warehouses)),
		fmt.Sprintf("заявок=%d", len(data.Tickets)),
		fmt.Sprintf("new=%d/in_progress=%d/deferred=%d/done=%d",
			statuses["new"], statuses["in_progress"], statuses["deferred"], statuses["done"]),
		fmt.Sprintf("оборудования=%d", len(data.Equipment)),
		fmt.Sprintf("перемещений=%d", len(data.Movements)),
		fmt.Sprintf("узлов_БЗ=%d", len(data.Nodes)),
		fmt.Sprintf("скрытых_корней=%d", hidden),
		fmt.Sprintf("праздников=%d", len(data.Holidays)),
	}, " ")
}
