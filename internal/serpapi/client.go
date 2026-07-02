package serpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"agroecologia-search/internal/models"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func New(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type serpAPIResponse struct {
	OrganicResults []struct {
		Position int    `json:"position"`
		Title    string `json:"title"`
		Link     string `json:"link"`
		Snippet  string `json:"snippet"`
	} `json:"organic_results"`
}

// Search consulta el motor de Google en SerpAPI y devuelve los resultados
// orgánicos crudos, todavía sin reordenar por la whitelist.
func (c *Client) Search(query string) ([]models.SearchResult, error) {
	endpoint := fmt.Sprintf(
		"https://serpapi.com/search.json?engine=google&q=%s&hl=es&gl=ar&api_key=%s",
		url.QueryEscape(query),
		url.QueryEscape(c.apiKey),
	)

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("serpapi: falló la request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("serpapi: status inesperado %d", resp.StatusCode)
	}

	var parsed serpAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("serpapi: falló el decode: %w", err)
	}

	results := make([]models.SearchResult, 0, len(parsed.OrganicResults))
	for _, r := range parsed.OrganicResults {
		domain := extractDomain(r.Link)
		results = append(results, models.SearchResult{
			Rank:       r.Position,
			URL:        r.Link,
			Domain:     domain,
			Title:      r.Title,
			Snippet:    r.Snippet,
			FaviconURL: "/api/favicon?domain=" + url.QueryEscape(domain),
		})
	}
	return results, nil
}

func extractDomain(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return parsed.Hostname()
}
