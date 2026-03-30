package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/babelsuite/babelsuite/internal/auth"
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

	auth.Seed(context.Background(), st, os.Getenv("ADMIN_EMAIL"), os.Getenv("ADMIN_PASSWORD"))

	frontendURL := envOr("FRONTEND_URL", "http://localhost:5173")
	jwtSvc := auth.NewJWT(envOr("JWT_SECRET", "change-me"))
	handler := auth.NewHandler(st, jwtSvc, auth.DefaultSSOProviders(
		os.Getenv("GITHUB_OAUTH_URL"),
		os.Getenv("GITLAB_OAUTH_URL"),
	))
	suiteService := suites.NewService()
	profileStore := profiles.NewFileStore(resolveWorkspacePath(envOr("PROFILES_FILE", "babelsuite-profiles.yaml")))
	profileService := profiles.NewService(suiteService, profileStore)
	platformStore := platform.NewFileStore(resolveWorkspacePath(envOr("PLATFORM_SETTINGS_FILE", "babelsuite-config.yaml")))
	suiteHandler := suites.NewHandler(profileService, jwtSvc)
	mockingHandler := mocking.NewHandler(mocking.NewService(suiteService))
	profileHandler := profiles.NewHandler(profileService, jwtSvc)
	catalogHandler := catalog.NewHandler(catalog.NewService(suiteService, platformStore), st, jwtSvc)
	engineStore := engine.NewStore()
	engineHandler := engine.NewHandler(engineStore, jwtSvc)
	executionWatcher := enginewatchers.NewExecutionWatcher(engineStore)
	executionService := execution.NewService(profileService, executionWatcher)
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
	profileHandler.Register(mux)
	suiteHandler.Register(mux)
	mockingHandler.Register(mux)
	executionHandler.Register(mux)
	platformHandler.Register(mux)
	sandboxHandler.Register(mux)

	addr := envOr("PORT", "8090")
	if !strings.HasPrefix(addr, ":") {
		addr = ":" + addr
	}

	log.Printf("babelsuite server listening on %s using %s", addr, dbDriver)
	if err := http.ListenAndServe(addr, cors(frontendURL, mux)); err != nil {
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
