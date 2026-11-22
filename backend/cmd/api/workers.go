package main

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// workerSpec описывает фоновый процесс. Реализации доменов подключаются в
// cmd/api одной строкой (правило AGENTS.md: main.go дополняется аддитивно).
type workerSpec struct {
	name  string
	every time.Duration
	run   func(ctx context.Context) error
}

// startWorkers запускает воркеры-заглушки для ещё не реализованных блоков
// (корзина — Б10). Outbox и напоминания Б8 запускаются отдельно в main.
func startWorkers(ctx context.Context, logger *slog.Logger) *sync.WaitGroup {
	var wg sync.WaitGroup
	for _, spec := range stubWorkers(logger) {
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

// stubWorkers — перечень ещё не реализованных фоновых процессов (PLAN §3.10).
func stubWorkers(logger *slog.Logger) []workerSpec {
	return []workerSpec{
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
