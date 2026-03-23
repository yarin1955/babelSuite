dev-server:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev

dev: ## run both in parallel (requires bash)
	$(MAKE) -j2 dev-server dev-frontend
