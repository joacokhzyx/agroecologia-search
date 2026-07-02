package openrouter

import (
	"bufio"
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
	apiKey       string
	models       []string
	siteURL      string
	siteName     string
	httpClient   *http.Client // para la llamada no-streaming (fallback/caché)
	streamClient *http.Client // sin timeout fijo: el context gobierna la duración
}

func New(apiKey, primaryModel string, fallbackModels []string, siteURL, siteName string) *Client {
	return &Client{
		apiKey:       apiKey,
		models:       append([]string{primaryModel}, fallbackModels...),
		siteURL:      siteURL,
		siteName:     siteName,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		streamClient: &http.Client{}, // Timeout: 0, se corta por ctx
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Models   []string      `json:"models"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
}

type chatResponse struct {
	Model   string `json:"model"`
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

// StreamChunk es un delta de texto recibido en tiempo real de OpenRouter.
type StreamChunk struct {
	Content string
	Model   string
}

// Summarize (no-streaming) se mantiene para el endpoint REST /api/search y
// para servir respuestas ya cacheadas de una sola vez.
func (c *Client) Summarize(ctx context.Context, query string, results []models.SearchResult) (string, int, int, error) {
	prompt := buildPrompt(query, results)
	reqBody := chatRequest{Models: c.models, Messages: []chatMessage{{Role: "user", Content: prompt}}}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, 0, err
	}

	const maxAttempts = 3
	backoff := 500 * time.Millisecond
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		content, pt, ct, status, err := c.doRequest(ctx, payload)
		if err == nil {
			return content, pt, ct, nil
		}
		lastErr = err
		if status != http.StatusTooManyRequests || attempt == maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return "", 0, 0, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 3
	}
	return "", 0, 0, lastErr
}

// SummarizeStream pide el resumen con stream:true y devuelve un canal que
// emite cada delta de texto a medida que OpenRouter lo genera. El canal se
// cierra solo cuando termina el stream (o el ctx se cancela).
func (c *Client) SummarizeStream(ctx context.Context, query string, results []models.SearchResult) (<-chan StreamChunk, error) {
	prompt := buildPrompt(query, results)
	reqBody := chatRequest{Models: c.models, Messages: []chatMessage{{Role: "user", Content: prompt}}, Stream: true}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: falló la request de streaming: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openrouter: status %d — body: %s", resp.StatusCode, truncate(raw, 500))
	}

	out := make(chan StreamChunk)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue // líneas vacías o de keep-alive
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}

			var event struct {
				Model   string `json:"model"`
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue // chunk parcial/comentario, lo ignoramos
			}
			for _, choice := range event.Choices {
				if choice.Delta.Content == "" {
					continue
				}
				select {
				case out <- StreamChunk{Content: choice.Delta.Content, Model: event.Model}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.siteURL != "" {
		req.Header.Set("HTTP-Referer", c.siteURL)
	}
	if c.siteName != "" {
		req.Header.Set("X-Title", c.siteName)
	}
}

func (c *Client) doRequest(ctx context.Context, payload []byte) (content string, promptTokens, completionTokens, statusCode int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", 0, 0, 0, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("openrouter: falló la request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, resp.StatusCode, fmt.Errorf("openrouter: falló la lectura del body: %w", err)
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", 0, 0, resp.StatusCode, fmt.Errorf("openrouter: falló el decode (status %d): %w — body: %s", resp.StatusCode, err, truncate(raw, 500))
	}
	if parsed.Error != nil {
		return "", 0, 0, resp.StatusCode, fmt.Errorf("openrouter: (status %d) %s", resp.StatusCode, parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", 0, 0, resp.StatusCode, fmt.Errorf("openrouter: status %d — body: %s", resp.StatusCode, truncate(raw, 500))
	}
	if len(parsed.Choices) == 0 {
		return "", 0, 0, resp.StatusCode, fmt.Errorf("openrouter: no se devolvieron choices — body: %s", truncate(raw, 500))
	}
	text := parsed.Choices[0].Message.Content
	if strings.TrimSpace(text) == "" {
		return "", 0, 0, resp.StatusCode, fmt.Errorf("openrouter: el modelo no generó contenido (posible cold start)")
	}
	return text, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, resp.StatusCode, nil
}

func buildPrompt(query string, results []models.SearchResult) string {
	var sources strings.Builder
	max := len(results)
	if max > 6 {
		max = 6
	}
	for i := 0; i < max; i++ {
		r := results[i]
		fmt.Fprintf(&sources, "%d. %s — %s\n%s\n\n", i+1, r.Title, r.URL, r.Snippet)
	}
	return fmt.Sprintf(
		"Sos el asistente de búsqueda de Agroecología.ar. Respondé en español, de forma clara y concisa, "+
			"a la consulta del usuario basándote únicamente en las fuentes de abajo. "+
			"Citá las fuentes relevantes por número entre corchetes, ej. [1]. "+
			"Si las fuentes no alcanzan para responder, decilo explícitamente.\n\n"+
			"Consulta: %s\n\nFuentes:\n%s",
		query, sources.String(),
	)
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}