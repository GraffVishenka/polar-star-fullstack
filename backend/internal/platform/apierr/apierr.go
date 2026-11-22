// Package apierr реализует единый формат ошибок API
// {"code","message","details","request_id"} и типизированные коды из ТЗ.
package apierr

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/reqid"
)

// Code — машиночитаемый код ошибки (SNAKE_CASE), см. PLAN §3.13.
type Code string

const (
	// Общие.
	CodeValidation         Code = "VALIDATION_ERROR"
	CodeInvalidJSON        Code = "INVALID_JSON"
	CodeUnauthorized       Code = "UNAUTHORIZED"
	CodeForbidden          Code = "FORBIDDEN"
	CodeNotFound           Code = "NOT_FOUND"
	CodeConflict           Code = "CONFLICT"
	CodeMethodNotAllowed   Code = "METHOD_NOT_ALLOWED"
	CodePayloadTooLarge    Code = "PAYLOAD_TOO_LARGE"
	CodeUnsupportedMedia   Code = "UNSUPPORTED_MEDIA_TYPE"
	CodeTooManyRequests    Code = "RATE_LIMITED"
	CodeInternal           Code = "INTERNAL_ERROR"
	CodeNotImplemented     Code = "NOT_IMPLEMENTED"
	CodeServiceUnavailable Code = "SERVICE_UNAVAILABLE"

	// AUTH (PLAN §3.2).
	CodeInvalidCredentials      Code = "INVALID_CREDENTIALS"
	CodeInvalidRefreshToken     Code = "INVALID_REFRESH_TOKEN"
	CodePasswordChangeRequired  Code = "PASSWORD_CHANGE_REQUIRED"
	CodeInvalidCurrentPassword  Code = "INVALID_CURRENT_PASSWORD"
	CodeBuildingRequiredForUser Code = "BUILDING_REQUIRED_FOR_USER"
	CodeBuildingInactive        Code = "BUILDING_INACTIVE"
	CodeBuildingNotAssigned     Code = "BUILDING_NOT_ASSIGNED"
	CodeCannotModifySelf        Code = "CANNOT_MODIFY_SELF"
	CodeLastAdmin               Code = "LAST_ADMIN"
	CodeDuplicateLogin          Code = "DUPLICATE_LOGIN"

	// TICKET (PLAN §3.3).
	CodeAssignOtherUser       Code = "ASSIGN_OTHER_USER"
	CodeExecutorWrongBuilding Code = "EXECUTOR_WRONG_BUILDING"
	CodeNotExecutor           Code = "NOT_EXECUTOR"
	CodeInvalidStatus         Code = "INVALID_STATUS"

	// KB (PLAN §3.5).
	CodeCycleDetected   Code = "CYCLE_DETECTED"
	CodeInvalidMediaRef Code = "INVALID_MEDIA_REF"

	// EQ (PLAN §3.6).
	CodeSameWarehouse               Code = "SAME_WAREHOUSE"
	CodeDuplicateRegistrationNumber Code = "DUPLICATE_REGISTRATION_NUMBER"
	CodeWarehouseNotEmpty           Code = "WAREHOUSE_NOT_EMPTY"
	CodeBuildingInUse               Code = "BUILDING_IN_USE"
)

// Error — типизированная ошибка API. Все слои возвращают *Error; handler
// отдаёт её клиенту в едином формате, не раскрывая внутренних деталей.
type Error struct {
	Status  int
	Code    Code
	Message string
	Details map[string]any
	Err     error
}

// New создаёт ошибку с HTTP-статусом и кодом.
func New(status int, code Code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message, Details: map[string]any{}}
}

// WithDetails добавляет/заменяет поля details (например, имена невалидных полей).
func (e *Error) WithDetails(details map[string]any) *Error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	for k, v := range details {
		e.Details[k] = v
	}
	return e
}

// WithDetail добавляет одно поле details.
func (e *Error) WithDetail(key string, value any) *Error {
	return e.WithDetails(map[string]any{key: value})
}

// WithCause сохраняет внутреннюю причину для логов (клиенту не отдаётся).
func (e *Error) WithCause(err error) *Error {
	e.Err = err
	return e
}

// Error реализует error.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap позволяет errors.Is/As работать с причиной.
func (e *Error) Unwrap() error { return e.Err }

// Конструкторы типовых ошибок.

// Validation — 422 VALIDATION_ERROR.
func Validation(message string, details map[string]any) *Error {
	return New(http.StatusUnprocessableEntity, CodeValidation, message).WithDetails(details)
}

// BadRequest — 400 для нечитаемого тела/параметров.
func BadRequest(code Code, message string) *Error {
	return New(http.StatusBadRequest, code, message)
}

// Unauthorized — 401.
func Unauthorized(code Code, message string) *Error {
	return New(http.StatusUnauthorized, code, message)
}

// Forbidden — 403.
func Forbidden(message string) *Error {
	return New(http.StatusForbidden, CodeForbidden, message)
}

// NotFound — 404 (используется и вместо 403, чтобы не раскрывать существование
// чужих заявок — TICKET-7).
func NotFound(message string) *Error {
	return New(http.StatusNotFound, CodeNotFound, message)
}

// Conflict — 409 с уточняющим кодом.
func Conflict(code Code, message string) *Error {
	return New(http.StatusConflict, code, message)
}

// TooManyRequests — 429, details содержат retry_after_seconds.
func TooManyRequests(retryAfterSeconds int) *Error {
	if retryAfterSeconds < 1 {
		retryAfterSeconds = 1
	}
	return New(http.StatusTooManyRequests, CodeTooManyRequests, "Слишком много запросов, попробуйте позже").
		WithDetail("retry_after_seconds", retryAfterSeconds)
}

// NotImplemented — 501 для замороженных маршрутов контракта.
func NotImplemented() *Error {
	return New(http.StatusNotImplemented, CodeNotImplemented, "Раздел находится в разработке")
}

// Internal — 500.
func Internal(err error) *Error {
	e := New(http.StatusInternalServerError, CodeInternal, "Внутренняя ошибка сервера")
	e.Err = err
	return e
}

// ServiceUnavailable — 503.
func ServiceUnavailable(message string) *Error {
	return New(http.StatusServiceUnavailable, CodeServiceUnavailable, message)
}

// Resolve приводит любую ошибку к *Error; неизвестные ошибки становятся 500.
func Resolve(err error) *Error {
	if err == nil {
		return nil
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return Internal(err)
}

// Body — тело ответа с ошибкой (единый формат).
type Body struct {
	Code      Code           `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details"`
	RequestID string         `json:"request_id"`
}

// Respond пишет ошибку в едином формате. request_id берётся из контекста,
// details всегда объект (не null), чтобы клиент не делал лишних проверок.
func Respond(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := Resolve(err)
	if apiErr == nil {
		return
	}
	details := apiErr.Details
	if details == nil {
		details = map[string]any{}
	}
	body := Body{
		Code:      apiErr.Code,
		Message:   apiErr.Message,
		Details:   details,
		RequestID: reqid.From(r.Context()),
	}
	writeJSON(w, apiErr.Status, body)
}

// Write пишет ошибку с явным статусом/кодом (для мест без error-значения,
// например 404 NotFound роутера).
func Write(w http.ResponseWriter, r *http.Request, status int, code Code, message string, details map[string]any) {
	Respond(w, r, &Error{Status: status, Code: code, Message: message, Details: details})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}
