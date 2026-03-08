# BabelSuite

BabelSuite is a container-native orchestrator for multi-language simulators. It acts exactly like a localized, hardware-in-the-loop (HIL) execution engine powered by **Starlark** configurations instead of complex YAML files. Wait, there's more!

BabelSuite features loosely-coupled, highly-independent architectural components. You can orchestrate deployments transitively via declarative pipelines, and it provides a centralized community package sharing registry natively embedded alongside the core engine.

## The Web App Ecosystem

BabelSuite provides a totally decoupled ecosystem of interactive React+Go architectures built explicitly for executing, managing, and sharing test engines:

### 1. BabelSuite Orchestrator (`/`)
The main orchestrator daemon is written in `Go`. It directly manages the Docker execution using the standard go client socket logic. The Starlark interpreter guarantees complex conditionals without static compilation YAML mess.
- Command: `go build -o babelsuite.exe main.go`
- Starts the API Server on `:3000`.

### 2. BabelSuite Control UI (`/ui`)
The primary UI Control Panel is a **Vite + React** app. Discover simulators, manage tests, and engage with live visual DAG node structures constructed dynamically based on real-time Docker bridge operations hooked from the Starlark pipeline engine.
- Navigate to `cd ui`
- Run `npm run dev`

### 3. Hub Registry API (`/hub-backend`)
An independent Go microservice acting strictly as your internal registry node. It natively maps metadata endpoints like `.test-python:latest`.
- Command: `cd hub-backend` then `go build -o hub-backend.exe main.go`
- Starts on `:4000`.

### 4. Hub Community UI (`/hub-ui`)
A beautifully-crafted dark-mode **Vite + React** standalone dashboard built exclusively for browsing, previewing, and installing testing suites and simulators published in your environment registry.
- Navigate to `cd hub-ui`
- Run `npm run dev`

## Getting Started

You can run BabelSuite manually, via `make`, or fully containerized with Docker Compose.

### Option 1: Using Docker Compose (Recommended)

BabelSuite comes with a preconfigured multi-stage `docker-compose.yml` that mounts the host's Docker socket, allowing the engine to orchestrate sibling simulator containers securely.

```bash
# Build and start the Orchestrator Daemon and Hub Registry
docker compose up -d
```
- Orchestrator API runs on `localhost:3000`
- Hub Registry API runs on `localhost:4000`

### Option 2: Using Make

We provided a generic `Makefile` to simplify building the binaries.

```bash
# Compile both backends
make build
make build-hub

# Start the backends
make run      # Runs BabelSuite core engine daemon
make run-hub  # Runs the Hub Registry
```

### Option 3: Manual Startup

Start the core engine daemon and hub from source:

```bash
# Terminal 1: Core Engine
go build -o babelsuite main.go
./babelsuite daemon

# Terminal 2: Registry Backend
cd hub-backend
go build -o hub-backend main.go
./hub-backend start
```

### Starting the Web UIs
Regardless of how you start the backends, start the interactive Web Applications mapping to each API backend:

```bash
# Terminal 3: Orchestrator DAG UI
cd ui
npm install
npm run dev
# Running on http://localhost:5173

# Terminal 4: Hub Marketplace UI
cd hub-ui
npm install
npm run dev
# Running on http://localhost:5174
```
