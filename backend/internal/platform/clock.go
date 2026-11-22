// Package platform содержит платформенные абстракции, общие для всех доменов:
// часы, рабочий календарь, уведомления. Домены не должны вызывать time.Now()
// напрямую — только через Clock (NFR-7: все расчёты времени в timezone).
package platform

import "time"

// Clock — источник текущего времени. Позволяет детерминированно тестировать
// бизнес-логику, зависящую от даты/времени (CAL-6, TICKET-3, NOTIF-7).
type Clock interface {
	Now() time.Time
}

// RealClock — рабочая реализация, использующая системное время.
type RealClock struct{}

// Now возвращает текущий момент времени.
func (RealClock) Now() time.Time { return time.Now() }

// FixedClock — тестовые часы с зафиксированным моментом времени.
type FixedClock struct {
	Time time.Time
}

// Now возвращает зафиксированный момент.
func (f FixedClock) Now() time.Time { return f.Time }
