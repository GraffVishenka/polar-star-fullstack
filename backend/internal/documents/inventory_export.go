package documents

import (
	"fmt"
	"net/http"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/graffvishenka/polar-star-service/backend/internal/equipment"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// maxInventoryItems — EQ-8: ведомость формируется не более чем на 1000 позиций.
const maxInventoryItems = 1000

// InventoryXLSX — GET /api/warehouses/{id}/inventory.xlsx (EQ-8).
func (h *Handler) InventoryXLSX(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, apierr.NotFound("Помещение не найдено"))
		return
	}
	info, err := h.inventory.Warehouse(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	warehouseID := id
	list, err := h.inventory.List(r.Context(), equipment.ListFilter{
		WarehouseID: &warehouseID, Page: 1, Size: maxInventoryItems + 1,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if list.Total > maxInventoryItems {
		h.fail(w, r, apierr.Validation("В помещении больше 1000 позиций — ведомость не формируется",
			map[string]any{"limit": maxInventoryItems, "total": list.Total}))
		return
	}
	body, err := buildInventoryXLSX(info, list.Items, list.Total, h.appName(), h.clock.Now(), h.location())
	if err != nil {
		h.fail(w, r, apierr.Internal(err))
		return
	}
	writeDownload(w, xlsxContentType, fmt.Sprintf("inventory_%d.xlsx", id), body)
}

func buildInventoryXLSX(info *equipment.WarehouseInfo, items []equipment.Equipment, total int,
	appName string, generatedAt time.Time, loc *time.Location) ([]byte, error) {
	if loc == nil {
		loc = time.UTC
	}
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	sheet := "Ведомость"
	if err := file.SetSheetName("Sheet1", sheet); err != nil {
		return nil, fmt.Errorf("documents: лист ведомости: %w", err)
	}
	bold, err := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, err
	}
	header, err := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"DDEBF7"}},
	})
	if err != nil {
		return nil, err
	}
	title, err := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	if err != nil {
		return nil, err
	}

	lines := []struct {
		text  string
		style int
	}{
		{"Ведомость помещения", title},
		{"Приложение: " + appName, 0},
		{"Здание: " + info.BuildingName, 0},
		{"Помещение: " + info.Name, 0},
		{"Дата формирования: " + generatedAt.In(loc).Format("02.01.2006 15:04"), 0},
		{fmt.Sprintf("Всего единиц: %d", total), 0},
	}
	for index, line := range lines {
		cell, _ := excelize.CoordinatesToCellName(1, index+1)
		if err := file.SetCellValue(sheet, cell, line.text); err != nil {
			return nil, err
		}
		if line.style != 0 {
			if err := file.SetCellStyle(sheet, cell, cell, line.style); err != nil {
				return nil, err
			}
		}
	}

	headerRow := len(lines) + 2
	headers := []string{"№", "Наименование", "Тип", "Серийный номер",
		"Регистрационный номер", "Примечание"}
	for index, text := range headers {
		cell, _ := excelize.CoordinatesToCellName(index+1, headerRow)
		if err := file.SetCellValue(sheet, cell, text); err != nil {
			return nil, err
		}
	}
	lastColumn, _ := excelize.ColumnNumberToName(len(headers))
	if err := file.SetCellStyle(sheet, fmt.Sprintf("A%d", headerRow),
		fmt.Sprintf("%s%d", lastColumn, headerRow), header); err != nil {
		return nil, err
	}

	for index, item := range items {
		line := headerRow + index + 1
		values := []any{
			index + 1,
			item.Name,
			item.EquipmentTypeName,
			item.SerialNumber,
			item.RegistrationNumber,
			"", // в equipment Б4 нет поля примечания; колонка сохраняется по EQ-8
		}
		for column, value := range values {
			cell, _ := excelize.CoordinatesToCellName(column+1, line)
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				return nil, err
			}
		}
	}

	signatureRow := headerRow + len(items) + 2
	signatures := []string{
		"Передал: ФИО ____________________ должность ____________________ подпись ____________ дата ____________",
		"Принял: ФИО ____________________ должность ____________________ подпись ____________ дата ____________",
		"М.П.",
	}
	for index, text := range signatures {
		cell := fmt.Sprintf("A%d", signatureRow+index)
		if err := file.SetCellValue(sheet, cell, text); err != nil {
			return nil, err
		}
		if err := file.SetCellStyle(sheet, cell, cell, bold); err != nil {
			return nil, err
		}
	}

	widths := map[string]float64{"A": 6, "B": 34, "C": 24, "D": 20, "E": 22, "F": 28}
	for column, width := range widths {
		if err := file.SetColWidth(sheet, column, column, width); err != nil {
			return nil, err
		}
	}
	if err := file.SetPanes(sheet, &excelize.Panes{
		Freeze: true, Split: false, YSplit: headerRow, TopLeftCell: fmt.Sprintf("A%d", headerRow+1),
		ActivePane: "bottomLeft",
	}); err != nil {
		return nil, err
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("documents: сборка ведомости: %w", err)
	}
	return buffer.Bytes(), nil
}
