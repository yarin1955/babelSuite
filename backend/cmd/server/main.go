package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/babelsuite/babelsuite/internal/agents"
	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/catalog"
	"github.com/babelsuite/babelsuite/internal/envloader"
	"github.com/babelsuite/babelsuite/internal/profiles"
	"github.com/babelsuite/babelsuite/internal/runs"
	"github.com/babelsuite/babelsuite/internal/sso"
	"github.com/babelsuite/babelsuite/internal/store"
	mongostore "github.com/babelsuite/babelsuite/internal/store/mongo"
	pgstore "github.com/babelsuite/babelsuite/internal/store/postgres"
	"github.com/babelsuite/babelsuite/internal/telemetry"
)

func main() {
	envloader.Load()

	shutdownTelemetry, telemetryErr := telemetry.Setup(context.Background(), telemetry.Config{
		ServiceName:    "babelsuite-server",
		ServiceVersion: "0.1.0",
	})
	if telemetryErr != nil {
		log.Fatalf("telemetry: %v", telemetryErr)
	}
	defer shutdownTelemetry(context.Background())

	var st store.Store
	var err error

	switch os.Getenv("DB_DRIVER") {
	case "postgres":
		dsn := os.Getenv("POSTGRES_DSN")
		if dsn == "" {
			log.Fatal("POSTGRES_DSN is required when DB_DRIVER=postgres")
		}
		st, err = pgstore.New(dsn)
	default:
		uri := os.Getenv("MONGO_URI")
		if uri == "" {
			uri = "mongodb://localhost:27017"
		}
		db := os.Getenv("MONGO_DB")
		if db == "" {
			db = "babelsuite"
		}
		st, err = mongostore.New(uri, db)
	}
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close(nil)

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "change-me"
	}

	auth.Seed(context.Background(), st, os.Getenv("ADMIN_USERNAME"), os.Getenv("ADMIN_PASSWORD"))

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:5173"
	}

	jwtSvc := auth.NewJWT(secret)
	handler := auth.NewHandler(st, jwtSvc)
	catalogHandler := catalog.NewHandler(st, jwtSvc)
	ssoHandler := sso.NewHandler(st, jwtSvc, frontendURL)
	agentsHandler := agents.NewHandler(st, jwtSvc)
	profilesHandler := profiles.NewHandler(st, jwtSvc)
	runsHandler := runs.NewHandler(st, jwtSvc)

	mux := http.NewServeMux()
	handler.Register(mux)
	catalogHandler.Register(mux)
	ssoHandler.Register(mux)
	agentsHandler.Register(mux)
	profilesHandler.Register(mux)
	runsHandler.Register(mux)

	// CORS middleware for frontend dev server
	corsed := corsMiddleware(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = ":8090"
	}
	log.Printf("babelsuite server on %s  db=%s", port, os.Getenv("DB_DRIVER"))
	if err := http.ListenAndServe(port, telemetry.WrapHandler(corsed, "babelsuite-server")); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
