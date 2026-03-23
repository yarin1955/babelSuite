package main

import (
	"log"
	"net/http"
	"os"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/envloader"
	mongostore "github.com/babelsuite/babelsuite/internal/store/mongo"
	pgstore "github.com/babelsuite/babelsuite/internal/store/postgres"
	"github.com/babelsuite/babelsuite/internal/store"
)

func main() {
	envloader.Load()

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

	jwtSvc := auth.NewJWT(secret)
	handler := auth.NewHandler(st, jwtSvc)

	mux := http.NewServeMux()
	handler.Register(mux)

	// CORS middleware for frontend dev server
	corsed := corsMiddleware(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = ":8090"
	}
	log.Printf("babelsuite server on %s  db=%s", port, os.Getenv("DB_DRIVER"))
	if err := http.ListenAndServe(port, corsed); err != nil {
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
