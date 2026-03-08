package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
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

func Start(port string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/dag", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"nodes": []map[string]interface{}{
				{"id": "1", "type": "input", "data": map[string]string{"label": "Clone Repo"}},
				{"id": "2", "type": "default", "data": map[string]string{"label": "Run Python Sim"}},
				{"id": "3", "type": "output", "data": map[string]string{"label": "Run Tests"}},
			},
			"edges": []map[string]interface{}{
				{"id": "e1-2", "source": "1", "target": "2", "animated": true},
				{"id": "e2-3", "source": "2", "target": "3", "animated": true},
			},
		})
	})

	mux.HandleFunc("/api/logs/", func(w http.ResponseWriter, r *http.Request) {
		container := strings.TrimPrefix(r.URL.Path, "/api/logs/")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"container": container,
			"logs": []string{
				"Starting execution...",
				"Environment ready.",
			},
		})
	})

	handler := loggerMiddleware(enableCORS(mux))

	log.Printf("BabelSuite API Listening on %s", port)
	return http.ListenAndServe(port, handler)
}
