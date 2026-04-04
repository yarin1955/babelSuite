package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/agent"
	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/cachehub"
	"github.com/babelsuite/babelsuite/internal/catalog"
	"github.com/babelsuite/babelsuite/internal/engine"
	enginewatchers "github.com/babelsuite/babelsuite/internal/engine/watchers"
	"github.com/babelsuite/babelsuite/internal/envloader"
	"github.com/babelsuite/babelsuite/internal/execution"
	"github.com/babelsuite/babelsuite/internal/mocking"
	"github.com/babelsuite/babelsuite/internal/platform"
	"github.com/babelsuite/babelsuite/internal/profiles"
	"github.com/babelsuite/babelsuite/internal/sandbox"
	"github.com/babelsuite/babelsuite/internal/store"
	mongostore "github.com/babelsuite/babelsuite/internal/store/mongo"
	pgstore "github.com/babelsuite/babelsuite/internal/store/postgres"
	"github.com/babelsuite/babelsuite/internal/suites"
	"github.com/babelsuite/babelsuite/internal/telemetry"
)

func main() {
	envloader.Load(".env", "../.env")

	var (
		st  store.Store
		err error
	)

	dbDriver := strings.ToLower(strings.TrimSpace(os.Getenv("DB_DRIVER")))
	switch dbDriver {
	case "", "mongo", "mongodb":
		mongoURI := envOr("MONGO_URI", "mongodb://localhost:27017")
		mongoDB := envOr("MONGO_DB", "babelsuite")
		st, err = mongostore.New(mongoURI, mongoDB)
		dbDriver = "mongo"
	case "postgres", "postgresql":
		postgresDSN := os.Getenv("POSTGRES_DSN")
		if postgresDSN == "" {
			log.Fatal("POSTGRES_DSN is required when DB_DRIVER=postgres")
		}
		st, err = pgstore.New(postgresDSN)
		dbDriver = "postgres"
	default:
		log.Fatalf("unsupported DB_DRIVER %q", dbDriver)
	}
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close(context.Background())
	primaryStore := st

	telemetryPipeline, err := telemetry.Start(context.Background())
	if err != nil {
		log.Fatalf("otel: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := telemetryPipeline.Shutdown(shutdownCtx); shutdownErr != nil {
			log.Printf("otel shutdown: %v", shutdownErr)
		}
	}()

	cacheLayer, err := buildCacheHub()
	if err != nil {
		log.Fatalf("redis cache: %v", err)
	}
	defer cacheLayer.Close()

	st = store.WithRedis(st, cacheLayer, store.CacheConfig{
		WorkspaceTTL: durationOr("CACHE_TTL_WORKSPACE", 5*time.Minute),
		FavoritesTTL: durationOr("CACHE_TTL_FAVORITES", 2*time.Minute),
	})

	auth.Seed(context.Background(), st, os.Getenv("ADMIN_EMAIL"), os.Getenv("ADMIN_PASSWORD"))

	frontendURL := envOr("FRONTEND_URL", "http://localhost:5173")
	jwtSvc := auth.NewJWT(envOr("JWT_SECRET", "change-me"))
	handler := auth.NewHandler(st, jwtSvc, auth.DefaultSSOProviders(
		os.Getenv("GITHUB_OAUTH_URL"),
		os.Getenv("GITLAB_OAUTH_URL"),
	))
	suiteService := suites.NewService()
	var profileStore profiles.Store = profiles.NewFileStore(resolveWorkspacePath(envOr("PROFILES_FILE", "babelsuite-profiles.yaml")))
	profileStore = profiles.WithRedis(profileStore, cacheLayer, durationOr("CACHE_TTL_PROFILES", 2*time.Minute))
	profileService := profiles.NewService(suiteService, profileStore)
	var platformStore platform.Store = platform.NewFileStore(resolveWorkspacePath(envOr("PLATFORM_SETTINGS_FILE", "babelsuite-config.yaml")))
	platformStore = platform.WithRedis(platformStore, cacheLayer, durationOr("CACHE_TTL_PLATFORM", 2*time.Minute))
	var agentRuntimeStore agent.RuntimeStore = agent.NewFileRuntimeStore(resolveWorkspacePath(envOr("AGENT_RUNTIME_FILE", "babelsuite-agents.yaml")))
	if repository, ok := primaryStore.(agent.RuntimeRepository); ok {
		agentRuntimeStore = agent.NewDBRuntimeStore(repository)
	}
	suiteHandler := suites.NewHandler(profileService, jwtSvc)
	mockingHandler := mocking.NewHandler(mocking.NewService(suiteService))
	profileHandler := profiles.NewHandler(profileService, jwtSvc)
	catalogReader := catalog.WithRedis(
		catalog.NewService(suiteService, platformStore),
		cacheLayer,
		durationOr("CACHE_TTL_CATALOG", 45*time.Second),
	)
	catalogHandler := catalog.NewHandler(catalogReader, st, jwtSvc)
	engineStore := engine.NewStore()
	engineHandler := engine.NewHandler(engineStore, jwtSvc)
	agentRegistry := agent.NewRegistry(agentRuntimeStore)
	executionWatcher := enginewatchers.NewExecutionWatcher(engineStore)
	executionService := execution.NewServiceWithPlatform(profileService, platformStore, executionWatcher)
	if runtimeStore, ok := primaryStore.(execution.RuntimeStore); ok {
		executionService.ConfigureRuntimeStore(runtimeStore)
	}
	executionService.ConfigureRuntimeCache(cacheLayer, durationOr("CACHE_TTL_EXECUTION_RUNTIME", 24*time.Hour))
	assignmentCoordinator := agent.NewCoordinator(agentRegistry, executionService)
	if assignmentStore, ok := primaryStore.(agent.AssignmentStore); ok {
		assignmentCoordinator.ConfigureStore(assignmentStore)
	}
	assignmentCoordinator.ConfigureRuntimeCache(cacheLayer, durationOr("CACHE_TTL_EXECUTION_RUNTIME", 24*time.Hour))
	executionService.ConfigureRemoteWorkers(agentRegistry, assignmentCoordinator)
	defer executionService.Close()
	executionHandler := execution.NewHandler(executionService, engineStore, jwtSvc)
	platformHandler := platform.NewHandler(platformStore, jwtSvc)
	sandboxService := sandbox.NewService()
	defer sandboxService.Close()
	sandboxHandler := sandbox.NewHandler(sandboxService, jwtSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","dbDriver":"` + dbDriver + `"}`))
	})
	handler.Register(mux)
	catalogHandler.Register(mux)
	engineHandler.Register(mux)
	agent.RegisterGateway(mux, agentRegistry, assignmentCoordinator)
	profileHandler.Register(mux)
	suiteHandler.Register(mux)
	mockingHandler.Register(mux)
	executionHandler.Register(mux)
	platformHandler.Register(mux)
	sandboxHandler.Register(mux)

	addr := envOr("PORT", "8090")
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	log.Printf("babelsuite server listening on %s using %s", addr, dbDriver)
	if err := http.ListenAndServe(addr, telemetry.WrapHTTP(cors(frontendURL, mux))); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func durationOr(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func buildCacheHub() (*cachehub.Hub, error) {
	address := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	if address == "" {
		return &cachehub.Hub{}, nil
	}

	index, err := strconv.Atoi(envOr("REDIS_DB", "0"))
	if err != nil {
		return nil, err
	}

	return cachehub.New(cachehub.Options{
		Address:  address,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       index,
		Prefix:   envOr("REDIS_PREFIX", "babelsuite"),
	})
}

func resolveWorkspacePath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	parentPath := filepath.Join("..", path)
	if _, err := os.Stat(parentPath); err == nil {
		return parentPath
	}
	return path
}

func cors(frontendURL string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != frontendURL {
			origin = frontendURL
		}

		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
