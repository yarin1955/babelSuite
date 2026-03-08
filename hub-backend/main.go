package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/urfave/cli/v2"
)

// enableCORS adds simple CORS headers
func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loggerMiddleware logs incoming requests
func loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func main() {
	app := &cli.App{
		Name:  "hub-backend",
		Usage: "BabelSuite Hub Registry backend server",
		Commands: []*cli.Command{
			{
				Name:    "start",
				Aliases: []string{"s"},
				Usage:   "Start the Registry API server",
				Action: func(c *cli.Context) error {
					port := ":4000"
					log.Printf("Starting BabelSuite Hub Registry server on %s", port)

					mux := http.NewServeMux()

					// Get all shared simulators/test suites
					mux.HandleFunc("/api/v1/packages", func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/api/v1/packages" && r.URL.Path != "/api/v1/packages/" {
							http.NotFound(w, r)
							return
						}
						if r.Method != http.MethodGet {
							http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
							return
						}

						w.Header().Set("Content-Type", "application/json")
						json.NewEncoder(w).Encode(map[string]interface{}{
							"packages": []map[string]interface{}{
								{
									"name":        "engine-sim",
									"version":     "1.2.0",
									"description": "Python HIL engine simulation environment",
									"provider":    "babelsuite-official",
									"downloads":   421,
								},
								{
									"name":        "test-python",
									"version":     "latest",
									"description": "Standard python unit testing suite",
									"provider":    "babelsuite-community",
									"downloads":   890,
								},
							},
						})
					})

					// Publish a new simulator/test suite
					mux.HandleFunc("/api/v1/packages/publish", func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodPost {
							http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
							return
						}

						w.Header().Set("Content-Type", "application/json")
						json.NewEncoder(w).Encode(map[string]interface{}{
							"status":  "success",
							"message": "Package successfully shared to the BabelSuite registry.",
						})
					})

					handler := loggerMiddleware(enableCORS(mux))
					return http.ListenAndServe(port, handler)
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
