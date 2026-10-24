.PHONY: help test lint up down kind-deploy

help: ## List targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

test: ## Run every test suite (Go, Python, web type-check)
	go test -race ./...
	cd analytics && pytest -q
	cd simulator && pytest -q
	cd web && npm run typecheck

lint: ## Static analysis
	gofmt -l . && go vet ./...
	golangci-lint run
	ruff check analytics simulator

up: ## Start the full stack locally (http://localhost:3000)
	@test -f deploy/compose/.env || (echo "Create deploy/compose/.env first: cp deploy/compose/.env.example deploy/compose/.env" && exit 1)
	docker compose -f deploy/compose/docker-compose.yml up --build -d

down: ## Stop the local stack
	docker compose -f deploy/compose/docker-compose.yml down

kind-deploy: ## Build images, load them into kind and deploy
	for s in api ingestor rules-engine edge-agent; do docker build --build-arg SERVICE=$$s -t plantorq/$$s:dev .; done
	docker build -t plantorq/analytics:dev analytics
	docker build -t plantorq/simulator:dev simulator
	docker build -t plantorq/web:dev web
	for s in api ingestor rules-engine edge-agent analytics simulator web; do kind load docker-image plantorq/$$s:dev; done
	kubectl apply -k deploy/k8s/overlays/kind
