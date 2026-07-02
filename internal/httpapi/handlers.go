package httpapi

import (
	"encoding/json"
	"log"
	"net/http"

	"agroecologia-search/internal/search"
)

func searchHandler(svc *search.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			http.Error(w, `{"error":"falta el parámetro q"}`, http.StatusBadRequest)
			return
		}

		resp, err := svc.Search(r.Context(), query)
		if err != nil {
			log.Printf("error de búsqueda: %v", err)
			http.Error(w, `{"error":"la búsqueda falló"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
