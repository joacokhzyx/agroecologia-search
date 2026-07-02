package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"fmt"
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
func searchStreamHandler(svc *search.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		dateRange := r.URL.Query().Get("range") // "", "w", "m", "y"

		if query == "" {
			http.Error(w, `{"error":"falta el parámetro q"}`, http.StatusBadRequest)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, `{"error":"streaming no soportado"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		events, err := svc.SearchStream(r.Context(), query, dateRange)
		if err != nil {
			log.Printf("error de streaming: %v", err)
			fmt.Fprintf(w, "event: error\ndata: {\"message\":\"la búsqueda falló\"}\n\n")
			flusher.Flush()
			return
		}

		for event := range events {
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, payload)
			flusher.Flush()

			select {
			case <-r.Context().Done():
				return
			default:
			}
		}
	}
}
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
