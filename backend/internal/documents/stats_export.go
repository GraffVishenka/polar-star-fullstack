package documents

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
	"github.com/graffvishenka/polar-star-service/backend/internal/stats"
)

var filenameUnsafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// StatsXLSX — GET /api/stats/export.xlsx (STATS-3): листы «Сводка» и «Заявки».
// Свой отчёт доступен всем, чужой — только ADMIN (проверяет stats.Service).
func (h *Handler) StatsXLSX(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	query, err := stats.ParseQuery(h.location(), r.URL.Query(), true)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	// STATS-2: фильтр по зданию — только для ADMIN; у USER он игнорируется,
	// как и в JSON-статистике.
	if actor.Role != auth.RoleAdmin {
		query.BuildingID = nil
	}
	rows, err := h.stats.ExportRows(r.Context(), actor, query)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	statsItem, err := h.exportStats(r.Context(), actor, query)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	body, err := buildStatsXLSX(statsItem, rows, query, h.location())
	if err != nil {
		h.fail(w, r, apierr.Internal(err))
		return
	}
	filename := fmt.Sprintf("stat_%s_%s_%s.xlsx",
		sanitizeFilename(statsItem.Login), periodPart(query.From), periodPart(query.To))
	writeDownload(w, xlsxContentType, filename, body)
}

func (h *Handler) exportStats(ctx context.Context, actor *auth.User, query stats.Query) (*stats.Stats, error) {
	if query.UserID != nil && *query.UserID != actor.ID {
		items, err := h.stats.AdminStats(ctx, actor, query)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, apierr.NotFound("Сотрудник не найден")
		}
		return items[0], nil
	}
	// ADMIN со сводным фильтром по зданию: AdminStats с user_id учитывает
	// здание, MyStats для личной статистики его игнорирует (STATS-2).
	if actor.Role == auth.RoleAdmin && query.BuildingID != nil {
		withUser := query
		withUser.UserID = &actor.ID
		items, err := h.stats.AdminStats(ctx, actor, withUser)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, apierr.NotFound("Сотрудник не найден")
		}
		return items[0], nil
	}
	return h.stats.MyStats(ctx, actor, query)
}

func buildStatsXLSX(statsItem *stats.Stats, rows []stats.ExportRow, query stats.Query, loc *time.Location) ([]byte, error) {
	if loc == nil {
		loc = time.UTC
	}
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()

	if err := file.SetSheetName("Sheet1", "Сводка"); err != nil {
		return nil, fmt.Errorf("documents: лист Сводка: %w", err)
	}
	if _, err := file.NewSheet("Заявки"); err != nil {
		return nil, fmt.Errorf("documents: лист Заявки: %w", err)
	}
	header, err := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"DDEBF7"}},
	})
	if err != nil {
		return nil, fmt.Errorf("documents: стиль шапки: %w", err)
	}
	if err := setSummarySheet(file, header, statsItem, query, loc); err != nil {
		return nil, err
	}
	if err := setRowsSheet(file, header, rows, loc); err != nil {
		return nil, err
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("documents: сборка xlsx: %w", err)
	}
	return buffer.Bytes(), nil
}

func setSummarySheet(file *excelize.File, header int, statsItem *stats.Stats, query stats.Query, loc *time.Location) error {
	sheet := "Сводка"
	headers := []string{"ФИО", "Логин", "Период с", "Период по",
		"Всего выполнено", "За период", "Среднее время, ч", "Медиана, ч", "В срок, %", "Просрочено"}
	for index, title := range headers {
		cell, _ := excelize.CoordinatesToCellName(index+1, 1)
		if err := file.SetCellValue(sheet, cell, title); err != nil {
			return err
		}
	}
	if err := file.SetCellStyle(sheet, "A1", "J1", header); err != nil {
		return err
	}

	periodFrom := ""
	if query.From != nil {
		periodFrom = query.From.In(loc).Format(platform.DateLayout)
	}
	periodTo := ""
	if query.To != nil {
		periodTo = query.To.In(loc).Format(platform.DateLayout)
	}
	values := []any{
		statsItem.FullName,
		statsItem.Login,
		periodFrom,
		periodTo,
		statsItem.TotalDone,
		statsItem.DoneInPeriod,
		optionalFloat(statsItem.AvgResolutionHours),
		optionalFloat(statsItem.MedianResolutionHours),
		optionalFloat(statsItem.OnTimePercent),
		statsItem.OverdueCount,
	}
	for index, value := range values {
		cell, _ := excelize.CoordinatesToCellName(index+1, 2)
		if err := file.SetCellValue(sheet, cell, value); err != nil {
			return err
		}
	}
	widths := map[string]float64{"A": 32, "B": 18, "C": 12, "D": 12, "E": 16, "F": 12,
		"G": 18, "H": 14, "I": 12, "J": 12}
	for column, width := range widths {
		if err := file.SetColWidth(sheet, column, column, width); err != nil {
			return err
		}
	}
	return nil
}

func setRowsSheet(file *excelize.File, header int, rows []stats.ExportRow, loc *time.Location) error {
	sheet := "Заявки"
	headers := []string{"№", "ID", "Здание", "Создана", "Завершена", "Дедлайн",
		"В срок", "Вид проблемы", "Кабинет", "Заявитель"}
	for index, title := range headers {
		cell, _ := excelize.CoordinatesToCellName(index+1, 1)
		if err := file.SetCellValue(sheet, cell, title); err != nil {
			return err
		}
	}
	if err := file.SetCellStyle(sheet, "A1", "J1", header); err != nil {
		return err
	}
	for index, row := range rows {
		line := index + 2
		values := []any{
			index + 1,
			row.TicketID,
			row.Building,
			formatDateTime(row.CreatedAt, loc),
			formatDateTime(row.CompletedAt, loc),
			formatDeadline(row.Deadline, loc),
			onTimeLabel(row.OnTime),
			row.ProblemType,
			row.Cabinet,
			row.ApplicantName,
		}
		for column, value := range values {
			cell, _ := excelize.CoordinatesToCellName(column+1, line)
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				return err
			}
		}
	}
	widths := map[string]float64{"A": 6, "B": 8, "C": 24, "D": 18, "E": 18, "F": 12,
		"G": 9, "H": 26, "I": 12, "J": 28}
	for column, width := range widths {
		if err := file.SetColWidth(sheet, column, column, width); err != nil {
			return err
		}
	}
	if err := file.SetPanes(sheet, &excelize.Panes{Freeze: true, Split: false, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return err
	}
	return nil
}

func formatDateTime(value time.Time, loc *time.Location) string {
	if value.IsZero() {
		return ""
	}
	return value.In(loc).Format("02.01.2006 15:04")
}

func formatDeadline(value *time.Time, loc *time.Location) string {
	if value == nil {
		return ""
	}
	return value.In(loc).Format("02.01.2006")
}

func onTimeLabel(value *bool) string {
	if value == nil {
		return ""
	}
	if *value {
		return "Да"
	}
	return "Нет"
}

func optionalFloat(value *float64) any {
	if value == nil {
		return ""
	}
	return *value
}

func periodPart(value *time.Time) string {
	if value == nil {
		return "all"
	}
	return value.Format(platform.DateLayout)
}

func sanitizeFilename(value string) string {
	clean := filenameUnsafe.ReplaceAllString(strings.TrimSpace(value), "_")
	clean = strings.Trim(clean, "._-")
	if clean == "" {
		return "user"
	}
	return clean
}
