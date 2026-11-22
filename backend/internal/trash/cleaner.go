package trash

import (
	"context"
	"log/slog"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// defaultCleanHour — TRASH-4: автоочистка запускается раз в сутки ночью по
// часовому поясу приложения.
const defaultCleanHour = 3

// Cleaner — ежедневная автоочистка корзины (TRASH-4). Удаляет элементы,
// пролежавшие дольше срока хранения (по умолчанию 90 дней), и пишет лог.
type Cleaner struct {
	service  *Service
	logger   *slog.Logger
	clock    platform.Clock
	location *time.Location
	hour     int
	minute   int
}

// NewCleaner создаёт очистку. location — часовой пояс приложения (CAL-2):
// время запуска интерпретируется в нём.
func NewCleaner(service *Service, logger *slog.Logger, clock platform.Clock, location *time.Location) *Cleaner {
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = platform.RealClock{}
	}
	if location == nil {
		location = time.UTC
	}
	return &Cleaner{
		service:  service,
		logger:   logger,
		clock:    clock,
		location: location,
		hour:     defaultCleanHour,
	}
}

// Run блокируется до отмены контекста: каждый день в настроенное время
// запускает очистку.
func (c *Cleaner) Run(ctx context.Context) {
	c.logger.Info("trash: автоочистка запущена",
		"at", formatClock(c.hour, c.minute),
		"timezone", c.location.String(),
		"retention", c.service.Retention().String())
	for {
		now := c.clock.Now().In(c.location)
		next := NextDailyRun(now, c.hour, c.minute)
		timer := time.NewTimer(next.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			c.logger.Info("trash: автоочистка остановлена")
			return
		case <-timer.C:
			c.RunOnce(ctx)
		}
	}
}

// RunOnce выполняет очистку один раз (используется джобой и тестами).
func (c *Cleaner) RunOnce(ctx context.Context) (int, error) {
	deleted, err := c.service.Cleanup(ctx)
	if err != nil {
		c.logger.Error("trash: автоочистка завершилась с ошибками",
			"deleted", deleted, "error", err)
		return deleted, err
	}
	c.logger.Info("trash: автоочистка выполнена", "deleted", deleted)
	return deleted, nil
}

// NextDailyRun — ближайшее наступление hour:minute в поясе now: сегодня, если
// время ещё не прошло, иначе завтра. Экспортирован для тестов и повторного
// использования ежедневных джоб (например, напоминаний в Б8).
func NextDailyRun(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func formatClock(hour, minute int) string {
	return time.Date(0, time.January, 1, hour, minute, 0, 0, time.UTC).Format("15:04")
}
