# Управление локальным стеком и проверками backend.
# Требуется GNU make (Linux/macOS/WSL/CI). На Windows без make используйте
# команды из README/PLAN напрямую.

SHELL := /bin/sh
COMPOSE ?= docker compose
BACKEND := backend

.DEFAULT_GOAL := help

.PHONY: help up down logs ps build migrate seed test vet fmt openapi

help: ## Показать список команд
	@printf 'Полярная звезда — команды:\n\n'
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

up: ## Поднять MySQL и API (сборка образов)
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

test: ## Unit-тесты backend (в CI/Linux; локально на Windows — scripts/go-test-wasm.ps1)
	cd $(BACKEND) && go test ./...

vet: ## go vet
	cd $(BACKEND) && go vet ./...

fmt: ## gofmt
	cd $(BACKEND) && gofmt -w .

openapi: ## Проверка контракта (нужен node + js-yaml; см. .cache скрипт)
	@echo "Скрипт проверки контракта не входит в поставку; используйте redocly lint openapi/openapi.yaml"
