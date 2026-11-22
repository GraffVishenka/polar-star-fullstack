package stats

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
)

// MySQLRepository — источник статистики на MySQL 8 (STATS-1, STATS-2).
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository создаёт репозиторий статистики.
func NewMySQLRepository(db *sql.DB) *MySQLRepository { return &MySQLRepository{db: db} }

// completedWhere собирает условие выборки завершённых заявок (корзина
// исключается, STATS-2).
func completedWhere(filter Filter) (string, []any) {
	where := []string{"t.deleted_at IS NULL", "t.completed_at IS NOT NULL"}
	args := make([]any, 0, 5)
	if filter.ExecutorID != nil {
		where = append(where, "t.executor_id = ?")
		args = append(args, *filter.ExecutorID)
	}
	if filter.BuildingID != nil {
		where = append(where, "t.building_id = ?")
		args = append(args, *filter.BuildingID)
	}
	if filter.From != nil {
		where = append(where, "t.completed_at >= ?")
		args = append(args, *filter.From)
	}
	if filter.ToExclusive != nil {
		where = append(where, "t.completed_at < ?")
		args = append(args, *filter.ToExclusive)
	}
	return strings.Join(where, " AND "), args
}

// CountCompleted — число завершённых заявок по фильтру (total_done, STATS-1).
func (r *MySQLRepository) CountCompleted(ctx context.Context, filter Filter) (int, error) {
	where, args := completedWhere(filter)
	var total int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tickets t WHERE `+where, args...).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("stats: подсчёт завершённых заявок: %w", err)
	}
	return total, nil
}

// CompletedTickets — завершённые заявки периода с именами здания и вида
// проблемы. Заказ по completed_at: медиана и месяцы считаются в Go.
func (r *MySQLRepository) CompletedTickets(ctx context.Context, filter Filter) ([]CompletedTicket, error) {
	where, args := completedWhere(filter)
	rows, err := r.db.QueryContext(ctx, `SELECT
			t.id, t.building_id, b.name,
			t.problem_type_id, pt.name,
			t.cabinet, t.applicant_name,
			t.created_at, t.completed_at, t.deadline
		FROM tickets t
		JOIN buildings b ON b.id = t.building_id
		LEFT JOIN problem_types pt ON pt.id = t.problem_type_id
		WHERE `+where+`
		ORDER BY t.completed_at, t.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("stats: выборка завершённых заявок: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tickets := make([]CompletedTicket, 0, 64)
	for rows.Next() {
		var (
			ticket        CompletedTicket
			problemTypeID sql.NullInt64
			problemName   sql.NullString
			deadline      sql.NullTime
		)
		if err := rows.Scan(&ticket.TicketID, &ticket.BuildingID, &ticket.BuildingName,
			&problemTypeID, &problemName, &ticket.Cabinet, &ticket.ApplicantName,
			&ticket.CreatedAt, &ticket.CompletedAt, &deadline); err != nil {
			return nil, fmt.Errorf("stats: чтение завершённой заявки: %w", err)
		}
		if problemTypeID.Valid {
			id := problemTypeID.Int64
			ticket.ProblemTypeID = &id
		}
		ticket.ProblemTypeName = problemName.String
		if deadline.Valid {
			date := deadline.Time
			ticket.Deadline = &date
		}
		tickets = append(tickets, ticket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("stats: чтение завершённых заявок: %w", err)
	}
	return tickets, nil
}

// Users — активные сотрудники для сводной статистики. При фильтре по зданию
// возвращаются только закреплённые за ним USER (ADMIN без здания не попадают:
// в разрезе «сотрудники здания» их метрики не запрашиваются).
func (r *MySQLRepository) Users(ctx context.Context, buildingID *int64) ([]Employee, error) {
	query := `SELECT u.id, u.login, u.full_name, u.role, u.building_id, b.name
		FROM users u
		LEFT JOIN buildings b ON b.id = u.building_id
		WHERE u.is_active = 1`
	args := make([]any, 0, 1)
	if buildingID != nil {
		query += ` AND u.role = 'USER' AND u.building_id = ?`
		args = append(args, *buildingID)
	}
	query += ` ORDER BY u.full_name, u.id`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("stats: список сотрудников: %w", err)
	}
	defer func() { _ = rows.Close() }()

	employees := make([]Employee, 0, 16)
	for rows.Next() {
		employee, err := scanEmployee(rows)
		if err != nil {
			return nil, err
		}
		employees = append(employees, *employee)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("stats: чтение сотрудников: %w", err)
	}
	return employees, nil
}

// UserByID — сотрудник по id, включая заблокированных: история выполненных
// заявок не должна исчезать из отчёта.
func (r *MySQLRepository) UserByID(ctx context.Context, id int64) (*Employee, error) {
	row := r.db.QueryRowContext(ctx, `SELECT u.id, u.login, u.full_name, u.role, u.building_id, b.name
		FROM users u
		LEFT JOIN buildings b ON b.id = u.building_id
		WHERE u.id = ?
		LIMIT 1`, id)
	employee, err := scanEmployee(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return employee, nil
}

// scanner — общий интерфейс *sql.Row и *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanEmployee читает строку сотрудника.
func scanEmployee(src scanner) (*Employee, error) {
	var (
		employee   Employee
		role       string
		buildingID sql.NullInt64
		buildingNm sql.NullString
	)
	err := src.Scan(&employee.ID, &employee.Login, &employee.FullName, &role,
		&buildingID, &buildingNm)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stats: чтение сотрудника: %w", err)
	}
	employee.Role = auth.Role(role)
	if buildingID.Valid {
		id := buildingID.Int64
		employee.BuildingID = &id
	}
	if buildingNm.Valid {
		name := buildingNm.String
		employee.BuildingName = &name
	}
	return &employee, nil
}
