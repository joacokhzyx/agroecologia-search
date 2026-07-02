package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"agroecologia-search/internal/models"
)

type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func New(apiKey, model string) *Client {
	return &Client{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Summarize arma un resumen fundamentado a partir de los mejores resultados
// de búsqueda y devuelve el texto generado junto con el uso de tokens (útil
// para loggear costos, aunque este modelo puntual sea gratuito).
func (c *Client) Summarize(ctx context.Context, query string, results []models.SearchResult) (string, int, int, error) {
	var sources strings.Builder
	max := len(results)
	if max > 6 {
		max = 6
	}
	for i := 0; i < max; i++ {
		r := results[i]
		fmt.Fprintf(&sources, "%d. %s — %s\n%s\n\n", i+1, r.Title, r.URL, r.Snippet)
	}

	prompt := fmt.Sprintf(
		"Sos el asistente de búsqueda de Agroecología.ar. Respondé en español, de forma clara y concisa, "+
			"a la consulta del usuario basándote Únicamente en las fuentes de abajo. "+
			"Citá las fuentes relevantes por número entre corchetes, ej. [1]. "+
			"Si las fuentes no alcanzan para responder, decilo explícitamente.\n\n"+
			"Consulta: %s\n\nFuentes:\n%s",
		query, sources.String(),
	)

	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, 0, fmt.Errorf("openrouter: falló la request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, fmt.Errorf("openrouter: falló la lectura del body: %w", err)
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", 0, 0, fmt.Errorf("openrouter: falló el decode (status %d): %w — body: %s", resp.StatusCode, err, truncate(raw, 500))
	}

	// OpenRouter puede devolver 200 con un cuerpo de error, o un status != 200
	// directamente (401 sin API key, 402 sin créditos, 404 "no endpoints
	// found" por configuración de privacidad, 429 rate limit del modelo free).
	if parsed.Error != nil {
		return "", 0, 0, fmt.Errorf("openrouter: (status %d) %s", resp.StatusCode, parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", 0, 0, fmt.Errorf("openrouter: status %d — body: %s", resp.StatusCode, truncate(raw, 500))
	}
	if len(parsed.Choices) == 0 {
		return "", 0, 0, fmt.Errorf("openrouter: no se devolvieron choices — body: %s", truncate(raw, 500))
	}

	content := parsed.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		// El modelo respondió 200 OK pero sin contenido: suele pasar por
		// cold start del proveedor free. Lo tratamos como error para que se
		// loguee y no quede un resumen vacío sin explicación.
		return "", 0, 0, fmt.Errorf("openrouter: el modelo no generó contenido (posible cold start o rate limit del proveedor free)")
	}

	return content, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
