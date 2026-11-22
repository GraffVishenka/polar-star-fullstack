package files

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// Параметры фоновой очистки (Б3, PLAN §9 п. 9).
const (
	// DefaultCleanupInterval — «раз в час» из требований Б3.
	DefaultCleanupInterval = time.Hour
	// DefaultOrphanRetention — файл, не привязанный за сутки, считается мусором.
	DefaultOrphanRetention = 24 * time.Hour
	// defaultCleanupBatch — сколько записей забирать за один прогон.
	defaultCleanupBatch = 500
)

// CleanerOptions — настройки очистки (в тестах уменьшаются).
type CleanerOptions struct {
	Interval  time.Duration
	Retention time.Duration
	Batch     int
}

// Cleaner удаляет «осиротевшие» файлы: записи files с linked_at IS NULL старше
// Retention (по умолчанию 24 часа) — сначала с диска, затем из БД. Также
// подчищает временные файлы прерванных загрузок.
type Cleaner struct {
	repo      Repository
	store     *Storage
	logger    *slog.Logger
	clock     platform.Clock
	interval  time.Duration
	retention time.Duration
	batch     int
}

// NewCleaner создаёт очистку orphan-файлов.
func NewCleaner(repo Repository, store *Storage, logger *slog.Logger, clock platform.Clock, opts CleanerOptions) *Cleaner {
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = platform.RealClock{}
	}
	if opts.Interval <= 0 {
		opts.Interval = DefaultCleanupInterval
	}
	if opts.Retention <= 0 {
		opts.Retention = DefaultOrphanRetention
	}
	if opts.Batch <= 0 {
		opts.Batch = defaultCleanupBatch
	}
	return &Cleaner{
		repo:      repo,
		store:     store,
		logger:    logger,
		clock:     clock,
		interval:  opts.Interval,
		retention: opts.Retention,
		batch:     opts.Batch,
	}
}

// Run — блокирующий цикл очистки: прогон при старте и далее раз в Interval.
// Останавливается по отмене контекста. Ошибки только логируются: сбой очистки
// не должен влиять на API (требование Б3).
func (c *Cleaner) Run(ctx context.Context) {
	c.logger.Info("files: очистка orphan-файлов запущена",
		"interval", c.interval.String(), "retention", c.retention.String())

	c.runLogged(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("files: очистка orphan-файлов остановлена")
			return
		case <-ticker.C:
			c.runLogged(ctx)
		}
	}
}

// runLogged выполняет прогон и логирует результат/ошибку.
func (c *Cleaner) runLogged(ctx context.Context) {
	deleted, err := c.RunOnce(ctx)
	if err != nil {
		// Не прерываем цикл: часть файлов могла быть удалена, остальное
		// попробуем на следующем часу.
		c.logger.Error("files: очистка orphan-файлов завершилась с ошибками",
			"deleted", deleted, "error", err)
		return
	}
	c.logger.Info("files: очистка orphan-файлов завершена", "deleted", deleted)
}

// RunOnce выполняет один прогон очистки и возвращает число удалённых файлов.
// Ошибки отдельных файлов собираются, но не прерывают обработку остальных.
func (c *Cleaner) RunOnce(ctx context.Context) (int, error) {
	cutoff := c.clock.Now().UTC().Add(-c.retention)

	candidates, err := c.repo.OrphanFiles(ctx, cutoff, c.batch)
	if err != nil {
		return 0, fmt.Errorf("files: выборка orphan-файлов: %w", err)
	}

	deleted := 0
	var errs []error
	for i := range candidates {
		f := &candidates[i]
		if ctx.Err() != nil {
			return deleted, ctx.Err()
		}
		// Сначала диск, потом запись: если удаление с диска не удалось, запись
		// сохраняется и файл попадёт в следующий прогон. Обратный порядок
		// оставил бы файл на диске навсегда.
		if err := c.store.Remove(f.StoredPath); err != nil {
			errs = append(errs, fmt.Errorf("файл %d (%s): удаление с диска: %w", f.ID, f.StoredPath, err))
			continue
		}
		if err := c.repo.Delete(ctx, f.ID); err != nil {
			errs = append(errs, fmt.Errorf("файл %d: удаление записи: %w", f.ID, err))
			continue
		}
		deleted++
	}

	// Временные файлы прерванных загрузок (обрыв соединения, рестарт). Уборка
	// вспомогательная: её сбой не должен влиять на основной результат очистки.
	if removed, err := c.store.CleanStaleStaging(cutoff); err != nil {
		c.logger.Warn("files: не удалось убрать временные файлы загрузок", "error", err)
	} else if removed > 0 {
		c.logger.Info("files: удалены временные файлы прерванных загрузок", "count", removed)
	}

	return deleted, errors.Join(errs...)
}
