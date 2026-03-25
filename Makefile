dev-server:
	cd backend && go run ./cmd/server

build-cli:
	cd cli && go build ./cmd/babel

dev-frontend:
	cd frontend && npm run dev

dev: ## run both in parallel (requires bash)
	$(MAKE) -j2 dev-server dev-frontend
