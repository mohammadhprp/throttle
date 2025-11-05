.PHONY: build start stop restart logs clean seed token test fmt lint help

.DEFAULT_GOAL := help

## Available commands:
help:
	@echo "Available Commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Use 'make <command>' to run a command."

build: ## Build and start containers in detached mode
	@APP_VERSION=$(git tag | tail -n 1) docker compose -f docker-compose.yml up -d --build

start: ## Start existing containers
	@APP_VERSION=$(git tag | tail -n 1) docker compose -f docker-compose.yml up -d

stop: ## Stop and remove containers
	@APP_VERSION=$(git tag | tail -n 1) docker compose -f docker-compose.yml down

restart: ## Restart containers (stop + start)
	@$(MAKE) stop
	@$(MAKE) start

logs: ## Tail container logs (add ctrl+c to stop)
	@APP_VERSION=$(git tag | tail -n 1) docker compose -f docker-compose.yml logs -f app

clean: ## Stop containers and remove volumes, images
	@APP_VERSION=$(git tag | tail -n 1) docker compose -f docker-compose.yml down -v --rmi all

seed: ## Seed the database with initial data
	@APP_VERSION=$(git tag | tail -n 1) docker compose -f docker-compose.yml exec app go run cmd/seed/main.go

token: ## Generate and print admin user tokens
	@APP_VERSION=$(git tag | tail -n 1) docker compose -f docker-compose.yml exec app go run cmd/token/main.go

test: ## Run all tests
	@APP_VERSION=$(git tag | tail -n 1) docker compose -f docker-compose.yml exec app go test ./tests/... -v

fmt: ## Format the code
	@golangci-lint fmt

lint: ## Check lint
	@golangci-lint run