package main

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// workerSpec описывает фоновый процесс. Реализации доменов подключаются в
// cmd/api одной строкой (правило AGENTS.md: main.go дополняется аддитивно).
type workerSpec struct {
	name  string
	every time.Duration
	run   func(ctx context.Context) error
}

// startWorkers запускает воркеры-заглушки. Они не выполняют полезной работы,
// но фиксируют каркас фоновых процессов и корректное завершение по контексту.
// Реальные outbox/напоминания/автоочистка появятся в Б8/Б10.
func startWorkers(ctx context.Context, logger *slog.Logger, calendar platform.Calendar) *sync.WaitGroup {
	var wg sync.WaitGroup
	for _, spec := range stubWorkers(logger, calendar) {
		wg.Add(1)
		go func(spec workerSpec) {
			defer wg.Done()
			ticker := time.NewTicker(spec.every)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					logger.Info("worker: остановлен", "worker", spec.name)
					return
				case <-ticker.C:
					runSafely(ctx, logger, spec)
				}
			}
		}(spec)
	}
	return &wg
}

// runSafely изолирует панику воркера, чтобы один сбой не ронял процесс.
func runSafely(ctx context.Context, logger *slog.Logger, spec workerSpec) {
	defer func() {
		if rec := recover(); rec != nil {
			logger.Error("worker: паника",
				"worker", spec.name, "panic", rec, "stack", string(debug.Stack()))
		}
	}()
	if err := spec.run(ctx); err != nil {
		logger.Error("worker: ошибка", "worker", spec.name, "error", err)
	}
}

// stubWorkers — перечень фоновых процессов из PLAN §3.7, §3.10, Б3.
func stubWorkers(logger *slog.Logger, calendar platform.Calendar) []workerSpec {
	return []workerSpec{
		{
			name:  "outbox-push",
			every: 30 * time.Second, // NOTIF-3: опрос outbox раз в 30 секунд
			run: func(context.Context) error {
				logger.Debug("worker stub: outbox пуст (реализация — Б8)")
				return nil
			},
		},
		{
			name:  "deadline-reminders",
			every: time.Minute, // NOTIF-2: ежедневно в deadline_reminder_time по timezone
			run: func(context.Context) error {
				logger.Debug("worker stub: напоминания о дедлайне (реализация — Б8)",
					"today", calendar.FormatDate(calendar.Today()))
				return nil
			},
		},
		{
			name:  "trash-cleanup",
			every: 6 * time.Hour, // TRASH-4: автоочистка старше 90 дней
			run: func(context.Context) error {
				logger.Debug("worker stub: автоочистка корзины (реализация — Б10)")
				return nil
			},
		},
		{
			name:  "orphan-files-cleanup",
			every: time.Hour, // Б3: удаление файлов без ссылок старше суток
			run: func(context.Context) error {
				logger.Debug("worker stub: очистка orphan-файлов (реализация — Б3)")
				return nil
			},
		},
	}
}
