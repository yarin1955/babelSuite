.PHONY: all build build-hub clean run run-hub docker-build docker-up docker-down

# Binary names
BIN_BABELSUITE = babelsuite
BIN_HUB = hub-backend

# OS specific binary extension
ifeq ($(OS),Windows_NT)
    EXT = .exe
else
    EXT =
endif

all: build build-hub

build:
	@echo "Building BabelSuite orchestrator..."
	go build -o $(BIN_BABELSUITE)$(EXT) main.go

build-hub:
	@echo "Building Hub Backend registry..."
	cd hub-backend && go build -o $(BIN_HUB)$(EXT) main.go

clean:
	@echo "Cleaning binaries..."
	rm -f $(BIN_BABELSUITE)$(EXT)
	rm -f hub-backend/$(BIN_HUB)$(EXT)

run: build
	@echo "Starting BabelSuite daemon..."
	./$(BIN_BABELSUITE)$(EXT) daemon

run-hub: build-hub
	@echo "Starting Hub Backend registry..."
	cd hub-backend && ./$(BIN_HUB)$(EXT) start

docker-build:
	@echo "Building Docker images..."
	docker compose build

docker-up:
	@echo "Starting services via Docker Compose..."
	docker compose up -d

docker-down:
	@echo "Stopping Docker Compose services..."
	docker compose down
