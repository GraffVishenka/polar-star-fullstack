# Управление локальным стеком и проверками.
# Требуется GNU make (Linux/macOS/WSL/CI). На Windows без make используйте
# команды из README/PLAN напрямую.

SHELL := /bin/sh
COMPOSE ?= docker compose
BACKEND := backend

.DEFAULT_GOAL := help

.PHONY: help up down logs ps build migrate seed seed-demo demo test vet fmt openapi e2e backup acceptance

help: ## Показать список команд
	@printf 'Полярная звезда — команды:\n\n'
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

up: ## Поднять MySQL, API и фронтенд (сборка образов)
	$(COMPOSE) up -d --build

down: ## Остановить стек (данные остаются в томах)
	$(COMPOSE) down

logs: ## Логи API (следить)
	$(COMPOSE) logs -f api

ps: ## Статус контейнеров
	$(COMPOSE) ps

build: ## Компиляция backend
	cd $(BACKEND) && go build ./...

migrate: ## Применить миграции golang-migrate
	$(COMPOSE) run --rm api migrate

seed: ## Идемпотентный сидинг (настройки, справочники, ADMIN из env)
	$(COMPOSE) run --rm api seed

seed-demo: ## Демо-данные поверх сида (4 здания, 50 заявок, 25 единиц, дерево БЗ)
	$(COMPOSE) run --rm -e SEED_DEMO=true api seed --demo

demo: up seed-demo ## Поднять стек и заполнить демо-данными

test: ## Unit-тесты backend (в CI/Linux; локально на Windows — scripts/go-test-wasm.ps1)
	cd $(BACKEND) && go test ./...

vet: ## go vet
	cd $(BACKEND) && go vet ./...

fmt: ## gofmt
	cd $(BACKEND) && gofmt -w .

openapi: ## Проверка контракта (Node.js + js-yaml, без сети)
	node scripts/validate_openapi.mjs

e2e: ## Сквозная проверка API на поднятом стеке (scripts/e2e.sh)
	./scripts/e2e.sh

backup: ## Бэкап БД и медиа (docker compose, ротация 7 дней)
	BACKUP_MODE=compose ./deploy/backup.sh

acceptance: ## Памятка ручной приёмки
	@cat scripts/acceptance.md
