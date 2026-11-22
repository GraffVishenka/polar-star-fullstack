package documents

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/equipment"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
	"github.com/graffvishenka/polar-star-service/backend/internal/stats"
	"github.com/graffvishenka/polar-star-service/backend/internal/tickets"
)

type fakeTicketSource struct {
	ticket       *tickets.Ticket
	comments     []*tickets.Comment
	publicTicket *tickets.Ticket
	getErr       error
	publicErr    error
}

func (f *fakeTicketSource) Get(context.Context, *auth.User, int64) (*tickets.Ticket, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.ticket, nil
}

func (f *fakeTicketSource) Comments(context.Context, *auth.User, int64, int, int) (*tickets.CommentPage, error) {
	return &tickets.CommentPage{Items: f.comments, Total: len(f.comments), Page: 1, Size: 100}, nil
}

func (f *fakeTicketSource) PublicTicket(context.Context, string) (*tickets.Ticket, []*tickets.Comment, error) {
	if f.publicErr != nil {
		return nil, nil, f.publicErr
	}
	return f.publicTicket, f.comments, nil
}

func (f *fakeTicketSource) PublicComments(context.Context, string, int, int) (*tickets.CommentPage, error) {
	return &tickets.CommentPage{Items: f.comments, Total: len(f.comments), Page: 1, Size: 100}, nil
}

func (f *fakeTicketSource) StatusLabel(tickets.Status) string {
	return "В работе"
}

type fakeStatsSource struct {
	location  *time.Location
	rows      []stats.ExportRow
	summary   *stats.Stats
	exportErr error
	statsErr  error
}

func (f *fakeStatsSource) Location() *time.Location {
	if f.location == nil {
		return time.UTC
	}
	return f.location
}

func (f *fakeStatsSource) ExportRows(context.Context, *auth.User, stats.Query) ([]stats.ExportRow, error) {
	if f.exportErr != nil {
		return nil, f.exportErr
	}
	return f.rows, nil
}

func (f *fakeStatsSource) MyStats(context.Context, *auth.User, stats.Query) (*stats.Stats, error) {
	if f.statsErr != nil {
		return nil, f.statsErr
	}
	return f.summary, nil
}

func (f *fakeStatsSource) AdminStats(context.Context, *auth.User, stats.Query) ([]*stats.Stats, error) {
	if f.statsErr != nil {
		return nil, f.statsErr
	}
	return []*stats.Stats{f.summary}, nil
}

type fakeInventorySource struct {
	info  *equipment.WarehouseInfo
	items []equipment.Equipment
	total int
	err   error
}

func (f *fakeInventorySource) Warehouse(context.Context, int64) (*equipment.WarehouseInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.info, nil
}

func (f *fakeInventorySource) List(context.Context, equipment.ListFilter) (*equipment.ListResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &equipment.ListResult{Items: f.items, Total: f.total, Page: 1, Size: 1000}, nil
}

func testHandler(ticketsSource *fakeTicketSource, statsSource *fakeStatsSource,
	inventorySource *fakeInventorySource) *Handler {
	return NewHandler(ticketsSource, statsSource, inventorySource, Options{
		AppName: func() string { return "Сервис заявок" },
		Clock:   platform.FixedClock{Time: time.Date(2026, 11, 22, 23, 30, 0, 0, time.UTC)},
	}, nil)
}

func requestWithActor(method, target string, actor *auth.User) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	return request.WithContext(auth.WithUser(request.Context(), actor))
}

func withRoute(request *http.Request, name, value string) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add(name, value)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeCtx))
}

func sampleTicket() *tickets.Ticket {
	return &tickets.Ticket{
		ID: 5, BuildingName: "Здание А", ApplicantName: "Иванов И.И.", Cabinet: "101",
		ProblemTypeName: "Электрика", Description: "Не работает лампа",
		Status: tickets.StatusInProgress, CreatedAt: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
		ExecutorName: stringPtr("Петров П.П."),
	}
}

func stringPtr(value string) *string { return &value }

func TestHandler_TicketDOCX(t *testing.T) {
	source := &fakeTicketSource{
		ticket: sampleTicket(),
		comments: []*tickets.Comment{{
			ID: 1, Body: "Выехал", AuthorName: "Петров П.П.",
			CreatedAt: time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC),
			Files:     []*tickets.Attachment{{ID: 9, Filename: "photo.jpg"}},
		}},
	}
	handler := testHandler(source, &fakeStatsSource{location: time.UTC}, &fakeInventorySource{})
	request := withRoute(requestWithActor(http.MethodGet, "/api/tickets/5/export.docx",
		&auth.User{ID: 1, Role: auth.RoleAdmin, IsActive: true}), "id", "5")
	recorder := httptest.NewRecorder()
	handler.TicketDOCX(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "ticket_5.docx") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", recorder.Header().Get("Cache-Control"))
	}
	xml := docxDocumentXML(t, recorder.Body.Bytes())
	for _, value := range []string{"Здание А", "Иванов И.И.", "Электрика", "Выехал", "photo.jpg", "{"} {
		if value == "{" {
			if strings.Contains(xml, "{{") {
				t.Fatal("в документе остались плейсхолдеры")
			}
			continue
		}
		if !strings.Contains(xml, value) {
			t.Fatalf("в документе нет %q", value)
		}
	}
}

func TestHandler_TicketDOCX_NotFound(t *testing.T) {
	source := &fakeTicketSource{getErr: apierr.NotFound("Заявка не найдена")}
	handler := testHandler(source, &fakeStatsSource{}, &fakeInventorySource{})
	request := withRoute(requestWithActor(http.MethodGet, "/api/tickets/99/export.docx",
		&auth.User{ID: 1, Role: auth.RoleUser, IsActive: true}), "id", "99")
	recorder := httptest.NewRecorder()
	handler.TicketDOCX(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestHandler_PublicTicketDOCX(t *testing.T) {
	source := &fakeTicketSource{publicTicket: sampleTicket()}
	handler := testHandler(source, &fakeStatsSource{}, &fakeInventorySource{})
	request := withRoute(httptest.NewRequest(http.MethodGet, "/api/public/tickets/token/export.docx", nil),
		"token", "token")
	recorder := httptest.NewRecorder()
	handler.PublicTicketDOCX(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(docxDocumentXML(t, recorder.Body.Bytes()), "Здание А") {
		t.Fatal("нет данных заявки")
	}

	source.publicErr = apierr.NotFound("Заявка не найдена")
	recorder = httptest.NewRecorder()
	handler.PublicTicketDOCX(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("битый токен: status = %d", recorder.Code)
	}
}

func TestHandler_StatsXLSX(t *testing.T) {
	summary := &stats.Stats{
		Employee:  stats.Employee{ID: 7, Login: "ivanov", FullName: "Иванов Иван"},
		TotalDone: 12,
		Metrics: stats.Metrics{DoneInPeriod: 3, OverdueCount: 1,
			AvgResolutionHours: floatPtr(4.5), MedianResolutionHours: floatPtr(3.0),
			OnTimePercent: floatPtr(66.7)},
	}
	source := &fakeStatsSource{
		location: time.UTC,
		summary:  summary,
		rows: []stats.ExportRow{{
			TicketID: 5, Building: "Здание А", CreatedAt: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
			CompletedAt: time.Date(2026, 11, 2, 12, 30, 0, 0, time.UTC),
			ProblemType: "Электрика", Cabinet: "101", ApplicantName: "Иванов",
		}},
	}
	handler := testHandler(&fakeTicketSource{}, source, &fakeInventorySource{})
	request := requestWithActor(http.MethodGet,
		"/api/stats/export.xlsx?from=2026-11-01&to=2026-11-30",
		&auth.User{ID: 7, Login: "ivanov", Role: auth.RoleUser, IsActive: true})
	recorder := httptest.NewRecorder()
	handler.StatsXLSX(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "stat_ivanov_2026-11-01_2026-11-30.xlsx") {
		t.Fatalf("filename = %q", got)
	}

	file, err := excelize.OpenReader(bytes.NewReader(recorder.Body.Bytes()))
	if err != nil {
		t.Fatalf("xlsx: %v", err)
	}
	defer func() { _ = file.Close() }()
	sheets := file.GetSheetList()
	if len(sheets) != 2 || sheets[0] != "Сводка" || sheets[1] != "Заявки" {
		t.Fatalf("sheets = %v", sheets)
	}
	if value, _ := file.GetCellValue("Сводка", "B2"); value != "ivanov" {
		t.Fatalf("login = %q", value)
	}
	if value, _ := file.GetCellValue("Сводка", "E2"); value != "12" {
		t.Fatalf("total_done = %q", value)
	}
	if value, _ := file.GetCellValue("Заявки", "B2"); value != "5" {
		t.Fatalf("ticket_id = %q", value)
	}
}

func TestHandler_StatsXLSX_Forbidden(t *testing.T) {
	source := &fakeStatsSource{exportErr: apierr.Forbidden("Нет прав")}
	handler := testHandler(&fakeTicketSource{}, source, &fakeInventorySource{})
	request := requestWithActor(http.MethodGet, "/api/stats/export.xlsx?user_id=99",
		&auth.User{ID: 7, Login: "ivanov", Role: auth.RoleUser, IsActive: true})
	recorder := httptest.NewRecorder()
	handler.StatsXLSX(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestHandler_InventoryXLSX(t *testing.T) {
	source := &fakeInventorySource{
		info: &equipment.WarehouseInfo{ID: 3, Name: "Кабинет 101", BuildingID: 1, BuildingName: "Здание А"},
		items: []equipment.Equipment{
			{ID: 1, Name: "Лампа", EquipmentTypeName: "Освещение", SerialNumber: "SN-1", RegistrationNumber: "REG-1"},
			{ID: 2, Name: "Стол", EquipmentTypeName: "Мебель", SerialNumber: "SN-2", RegistrationNumber: "REG-2"},
		},
		total: 2,
	}
	handler := testHandler(&fakeTicketSource{}, &fakeStatsSource{location: time.UTC}, source)
	request := withRoute(httptest.NewRequest(http.MethodGet, "/api/warehouses/3/inventory.xlsx", nil), "id", "3")
	recorder := httptest.NewRecorder()
	handler.InventoryXLSX(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "inventory_3.xlsx") {
		t.Fatalf("filename = %q", got)
	}
	file, err := excelize.OpenReader(bytes.NewReader(recorder.Body.Bytes()))
	if err != nil {
		t.Fatalf("xlsx: %v", err)
	}
	defer func() { _ = file.Close() }()
	if sheets := file.GetSheetList(); len(sheets) != 1 || sheets[0] != "Ведомость" {
		t.Fatalf("sheets = %v", sheets)
	}
	rows, err := file.GetRows("Ведомость")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(flattenRows(rows), " | ")
	for _, value := range []string{"Здание А", "Кабинет 101", "Лампа", "Стол", "SN-1", "REG-1",
		"Передал", "Принял", "М.П."} {
		if !strings.Contains(text, value) {
			t.Fatalf("в ведомости нет %q: %s", value, text)
		}
	}
}

func TestHandler_InventoryXLSX_TooMany(t *testing.T) {
	source := &fakeInventorySource{
		info:  &equipment.WarehouseInfo{ID: 3, Name: "Кабинет 101", BuildingName: "Здание А"},
		total: 1001,
	}
	handler := testHandler(&fakeTicketSource{}, &fakeStatsSource{}, source)
	request := withRoute(httptest.NewRequest(http.MethodGet, "/api/warehouses/3/inventory.xlsx", nil), "id", "3")
	recorder := httptest.NewRecorder()
	handler.InventoryXLSX(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "1000") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandler_InventoryXLSX_NotFound(t *testing.T) {
	source := &fakeInventorySource{err: apierr.NotFound("Помещение не найдено")}
	handler := testHandler(&fakeTicketSource{}, &fakeStatsSource{}, source)
	request := withRoute(httptest.NewRequest(http.MethodGet, "/api/warehouses/99/inventory.xlsx", nil), "id", "99")
	recorder := httptest.NewRecorder()
	handler.InventoryXLSX(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func docxDocumentXML(t *testing.T, body []byte) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = stream.Close() }()
		data, err := io.ReadAll(stream)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	t.Fatal("нет word/document.xml")
	return ""
}

func flattenRows(rows [][]string) []string {
	var result []string
	for _, row := range rows {
		result = append(result, row...)
	}
	return result
}

func floatPtr(value float64) *float64 { return &value }
