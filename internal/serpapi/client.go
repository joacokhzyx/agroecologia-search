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




type SearchOptions struct {
	// DateRange: "" (cualquier momento), "w" (semana), "m" (mes), "y" (año).
	DateRange string
}

type serpAPIResponse struct {
	OrganicResults []struct {
		Position int    `json:"position"`
		Title    string `json:"title"`
		Link     string `json:"link"`
		Snippet  string `json:"snippet"`
	} `json:"organic_results"`
	RelatedSearches []struct {
		Query string `json:"query"`
		Link  string `json:"link"`
	} `json:"related_searches"`
}

func (c *Client) Search(query string, opts SearchOptions) ([]models.SearchResult, []models.RelatedSearch, error) {
	params := url.Values{}
	params.Set("engine", "google")
	params.Set("q", query)
	params.Set("hl", "es")
	params.Set("gl", "ar")
	params.Set("api_key", c.apiKey)
	if opts.DateRange != "" {
		params.Set("tbs", "qdr:"+opts.DateRange)
	}

	resp, err := c.httpClient.Get("https://serpapi.com/search.json?" + params.Encode())
	if err != nil {
		return nil, nil, fmt.Errorf("serpapi: falló la request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("serpapi: status inesperado %d", resp.StatusCode)
	}

	var parsed serpAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, nil, fmt.Errorf("serpapi: falló el decode: %w", err)
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

	related := make([]models.RelatedSearch, 0, len(parsed.RelatedSearches))
	for _, r := range parsed.RelatedSearches {
		if r.Query == "" {
			continue
		}
		related = append(related, models.RelatedSearch{Query: r.Query, Link: r.Link})
	}

	return results, related, nil
}

func extractDomain(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return parsed.Hostname()
}
