package httpapi

import (
	"net/http"

	"agroecologia-search/internal/favicon"
	"agroecologia-search/internal/search"
)

func NewRouter(svc *search.Service, faviconHandler *favicon.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/search", searchHandler(svc))
	mux.HandleFunc("GET /api/favicon", faviconHandler.ServeHTTP)
	mux.HandleFunc("GET /health", healthHandler)

	return withCORS(mux)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
