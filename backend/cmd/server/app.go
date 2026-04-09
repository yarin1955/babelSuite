package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/agent"
	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/cachehub"
	"github.com/babelsuite/babelsuite/internal/catalog"
	"github.com/babelsuite/babelsuite/internal/engine"
	enginewatchers "github.com/babelsuite/babelsuite/internal/engine/watchers"
	"github.com/babelsuite/babelsuite/internal/environments"
	"github.com/babelsuite/babelsuite/internal/execution"
	"github.com/babelsuite/babelsuite/internal/httpserver"
	"github.com/babelsuite/babelsuite/internal/mocking"
	"github.com/babelsuite/babelsuite/internal/platform"
	"github.com/babelsuite/babelsuite/internal/profiles"
	"github.com/babelsuite/babelsuite/internal/store"
	mongostore "github.com/babelsuite/babelsuite/internal/store/mongo"
	pgstore "github.com/babelsuite/babelsuite/internal/store/postgres"
	"github.com/babelsuite/babelsuite/internal/suites"
	"github.com/babelsuite/babelsuite/internal/telemetry"
)

type controlPlane struct {
	dbDriver           string
	handler            http.Handler
	telemetryPipeline  *telemetry.Pipeline
	cacheLayer         *cachehub.Hub
	primaryStore       store.Store
	executionService   *execution.Service
	environmentService *environments.Service
}

func newControlPlane(ctx context.Context) (*controlPlane, error) {
	primaryStore, dbDriver, err := buildPrimaryStore()
	if err != nil {
		return nil, err
	}

	telemetryPipeline, err := telemetry.Start(ctx)
	if err != nil {
		_ = primaryStore.Close(ctx)
		return nil, fmt.Errorf("telemetry: %w", err)
	}

	cacheLayer, err := buildCacheHub()
	if err != nil {
		_ = telemetryPipeline.Shutdown(ctx)
		_ = primaryStore.Close(ctx)
		return nil, fmt.Errorf("cache: %w", err)
	}

	cachedStore := store.WithRedis(primaryStore, cacheLayer, store.CacheConfig{
		WorkspaceTTL: durationOr("CACHE_TTL_WORKSPACE", 5*time.Minute),
		FavoritesTTL: durationOr("CACHE_TTL_FAVORITES", 2*time.Minute),
	})

	auth.Seed(ctx, cachedStore, os.Getenv("ADMIN_EMAIL"), os.Getenv("ADMIN_PASSWORD"))

	frontendURL := envOr("FRONTEND_URL", "http://localhost:5173")
	apiBaseURL := envOr("PUBLIC_API_URL", envOr("VITE_API_URL", "http://localhost:"+envOr("PORT", "8090")))
	jwtSvc := auth.NewJWT(envOr("JWT_SECRET", "change-me"))
	authHandler := auth.NewHandler(cachedStore, jwtSvc, auth.Config{
		FrontendURL:         frontendURL,
		PasswordAuthEnabled: boolEnv("AUTH_PASSWORD_LOGIN_ENABLED", true),
		SignUpEnabled:       boolEnv("AUTH_SIGNUP_ENABLED", true),
		OIDC: auth.OIDCConfig{
			Enabled:             boolEnv("OIDC_ENABLED", false),
			ProviderID:          envOr("OIDC_PROVIDER_ID", "oidc"),
			ProviderName:        envOr("OIDC_PROVIDER_NAME", "Single Sign-On"),
			IssuerURL:           strings.TrimSpace(os.Getenv("OIDC_ISSUER_URL")),
			ClientID:            strings.TrimSpace(os.Getenv("OIDC_CLIENT_ID")),
			ClientSecret:        strings.TrimSpace(os.Getenv("OIDC_CLIENT_SECRET")),
			RedirectURL:         envOr("OIDC_REDIRECT_URL", strings.TrimRight(apiBaseURL, "/")+"/api/v1/auth/oidc/callback"),
			FrontendCallbackURL: envOr("OIDC_FRONTEND_CALLBACK_URL", strings.TrimRight(frontendURL, "/")+"/auth/callback"),
			Scopes:              splitCSV(envOr("OIDC_SCOPES", "openid,profile,email,groups")),
			PKCEEnabled:         boolEnv("OIDC_PKCE_ENABLED", true),
			StateCookieName:     envOr("OIDC_STATE_COOKIE_NAME", "babelsuite_oidc_state"),
			StateSecret:         []byte(envOr("AUTH_STATE_SECRET", envOr("JWT_SECRET", "change-me"))),
			EmailClaim:          envOr("OIDC_EMAIL_CLAIM", "email"),
			NameClaim:           envOr("OIDC_NAME_CLAIM", "name"),
			GroupsClaim:         envOr("OIDC_GROUPS_CLAIM", "groups"),
			AdminGroups:         splitCSV(os.Getenv("OIDC_ADMIN_GROUPS")),
		},
	})

	suiteService := suites.NewService()

	profileBaseStore := profiles.NewFileStore(resolveWorkspacePath(envOr("PROFILES_FILE", "babelsuite-profiles.yaml")))
	var profileStore profiles.Store = profileBaseStore
	profileStore = profiles.WithRedis(profileStore, cacheLayer, durationOr("CACHE_TTL_PROFILES", 2*time.Minute))
	profileService := profiles.NewService(suiteService, profileStore)

	platformBaseStore := platform.NewFileStore(resolveWorkspacePath(envOr("PLATFORM_SETTINGS_FILE", "configuration.yaml")))
	if _, err := platformBaseStore.Load(); err != nil {
		_ = cacheLayer.Close()
		_ = telemetryPipeline.Shutdown(ctx)
		_ = primaryStore.Close(ctx)
		return nil, fmt.Errorf("platform settings: %w", err)
	}
	var platformStore platform.Store = platformBaseStore
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
	catalogHandler := catalog.NewHandler(catalogReader, cachedStore, jwtSvc)
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

	executionHandler := execution.NewHandler(executionService, engineStore, jwtSvc)
	platformHandler := platform.NewHandler(platformStore, jwtSvc)
	environmentService := environments.NewService()
	environmentHandler := environments.NewHandler(environmentService, jwtSvc)

	health := newHealthService(dbDriver, []subsystemProbe{
		requiredProbe("database", pingProbe(primaryStore)),
		optionalCacheProbe("cache", cacheLayer),
		requiredProbe("platform", loadProbe(func() error {
			_, err := platformBaseStore.Load()
			return err
		}, "settings loaded")),
		requiredProbe("profiles", loadProbe(func() error {
			_, err := profileBaseStore.Load()
			return err
		}, "profiles loaded")),
		optionalProbe("telemetry", func(context.Context) (probeCheck, error) {
			if telemetryPipeline == nil || !telemetryPipeline.Enabled() {
				return probeCheck{Status: probeDisabled, Detail: "collector not configured"}, nil
			}
			return probeCheck{Status: probeReady, Detail: "collector configured"}, nil
		}),
		optionalProbe("agents", func(context.Context) (probeCheck, error) {
			return probeCheck{Status: probeReady, Detail: fmt.Sprintf("%d agents tracked", len(agentRegistry.List()))}, nil
		}),
		optionalProbe("launch-suites", func(context.Context) (probeCheck, error) {
			return probeCheck{Status: probeReady, Detail: fmt.Sprintf("%d launchable suites", len(executionService.ListLaunchSuites()))}, nil
		}),
	})

	mux := http.NewServeMux()
	health.Register(mux)
	authHandler.Register(mux)
	catalogHandler.Register(mux)
	engineHandler.Register(mux)
	agent.RegisterGateway(mux, agentRegistry, assignmentCoordinator)
	profileHandler.Register(mux)
	suiteHandler.Register(mux)
	mockingHandler.Register(mux)
	executionHandler.Register(mux)
	platformHandler.Register(mux)
	environmentHandler.Register(mux)

	return &controlPlane{
		dbDriver:           dbDriver,
		handler:            buildHTTPHandler(frontendURL, jwtSvc, mux),
		telemetryPipeline:  telemetryPipeline,
		cacheLayer:         cacheLayer,
		primaryStore:       primaryStore,
		executionService:   executionService,
		environmentService: environmentService,
	}, nil
}

func (c *controlPlane) Close(ctx context.Context) error {
	var combined error
	if c.environmentService != nil {
		c.environmentService.Close()
	}
	if c.executionService != nil {
		c.executionService.Close()
	}
	if c.cacheLayer != nil {
		combined = errors.Join(combined, c.cacheLayer.Close())
	}
	if c.telemetryPipeline != nil {
		combined = errors.Join(combined, c.telemetryPipeline.Shutdown(ctx))
	}
	if c.primaryStore != nil {
		combined = errors.Join(combined, c.primaryStore.Close(ctx))
	}
	return combined
}

func buildPrimaryStore() (store.Store, string, error) {
	dbDriver := strings.ToLower(strings.TrimSpace(os.Getenv("DB_DRIVER")))
	switch dbDriver {
	case "", "mongo", "mongodb":
		st, err := mongostore.New(envOr("MONGO_URI", "mongodb://localhost:27017"), envOr("MONGO_DB", "babelsuite"))
		return st, "mongo", err
	case "postgres", "postgresql":
		postgresDSN := os.Getenv("POSTGRES_DSN")
		if postgresDSN == "" {
			return nil, "", fmt.Errorf("POSTGRES_DSN is required when DB_DRIVER=postgres")
		}
		st, err := pgstore.New(postgresDSN)
		return st, "postgres", err
	default:
		return nil, "", fmt.Errorf("unsupported DB_DRIVER %q", dbDriver)
	}
}

func buildHTTPHandler(frontendURL string, jwt *auth.JWTService, mux http.Handler) http.Handler {
	metrics := httpserver.NewHTTPMetrics()
	return httpserver.Chain(
		mux,
		corsMiddleware(frontendURL),
		httpserver.RequestIDMiddleware(),
		auth.PopulateSession(jwt, auth.VerifyOptions{AllowQueryToken: true}),
		telemetry.WrapHTTP,
		httpserver.TraceContextMiddleware(),
		metrics.Middleware(),
		httpserver.AuditMiddleware(),
	)
}

func requiredProbe(name string, check func(context.Context) (probeCheck, error)) subsystemProbe {
	return subsystemProbe{Name: name, Required: true, Check: check}
}

func optionalProbe(name string, check func(context.Context) (probeCheck, error)) subsystemProbe {
	return subsystemProbe{Name: name, Required: false, Check: check}
}

func optionalCacheProbe(name string, cache *cachehub.Hub) subsystemProbe {
	return optionalProbe(name, func(ctx context.Context) (probeCheck, error) {
		if cache == nil || !cache.Enabled() {
			return probeCheck{Status: probeDisabled, Detail: "cache disabled"}, nil
		}
		if err := cache.Ping(ctx); err != nil {
			return probeCheck{}, err
		}
		return probeCheck{Status: probeReady, Detail: "cache reachable"}, nil
	})
}

func pingProbe(target any) func(context.Context) (probeCheck, error) {
	return func(ctx context.Context) (probeCheck, error) {
		pinger, ok := target.(interface{ Ping(context.Context) error })
		if !ok {
			return probeCheck{Status: probeDisabled, Detail: "ping not supported"}, nil
		}
		if err := pinger.Ping(ctx); err != nil {
			return probeCheck{}, err
		}
		return probeCheck{Status: probeReady, Detail: "reachable"}, nil
	}
}

func loadProbe(load func() error, detail string) func(context.Context) (probeCheck, error) {
	return func(context.Context) (probeCheck, error) {
		if err := load(); err != nil {
			return probeCheck{}, err
		}
		return probeCheck{Status: probeReady, Detail: detail}, nil
	}
}

func corsMiddleware(frontendURL string) httpserver.Middleware {
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != frontendURL {
				origin = frontendURL
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-Id")
			w.Header().Set("Vary", "Origin")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
