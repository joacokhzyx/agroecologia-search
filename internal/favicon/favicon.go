package favicon

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Handler hace de proxy para los favicons a través de nuestro propio
// dominio, para que el frontend nunca dependa directamente de un servicio
// de terceros, y agrega headers de caché fuertes para que un CDN (Cloudflare,
// etc.) delante de esta API pueda cachear cada favicon durante mucho tiempo.
type Handler struct {
	httpClient *http.Client
}

func NewHandler() *Handler {
	return &Handler{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

const faviconCacheTTLSeconds = 60 * 60 * 24 * 30 // 30 días

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		http.Error(w, "falta el parámetro domain", http.StatusBadRequest)
		return
	}

	source := "https://www.google.com/s2/favicons?sz=64&domain=" + url.QueryEscape(domain)

	resp, err := h.httpClient.Get(source)
	if err != nil || resp.StatusCode != http.StatusOK {
		http.Redirect(w, r, source, http.StatusFound)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, immutable", faviconCacheTTLSeconds))
	_, _ = io.Copy(w, resp.Body)
}
