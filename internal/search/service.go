package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"

	"agroecologia-search/internal/cache"
	"agroecologia-search/internal/db"
	"agroecologia-search/internal/models"
	"agroecologia-search/internal/openrouter"
	"agroecologia-search/internal/serpapi"
	"agroecologia-search/internal/whitelist"
)

type Service struct {
	cache      *cache.Client
	db         *db.DB
	serpapi    *serpapi.Client
	openrouter *openrouter.Client
	whitelist  *whitelist.List

	ttlResults int
	ttlSummary int

	inFlight *singleflightGroup
}

func New(
	c *cache.Client,
	database *db.DB,
	serp *serpapi.Client,
	or *openrouter.Client,
	wl *whitelist.List,
	ttlResults, ttlSummary int,
) *Service {
	return &Service{
		cache:      c,
		db:         database,
		serpapi:    serp,
		openrouter: or,
		whitelist:  wl,
		ttlResults: ttlResults,
		ttlSummary: ttlSummary,
		inFlight:   newSingleflightGroup(),
	}
}

type Response struct {
	Query           string                 `json:"query"`
	Results         []models.SearchResult  `json:"results"`
	RelatedSearches []models.RelatedSearch `json:"relatedSearches"`
	Summary         string                 `json:"summary"`
	ServedFromCache bool                   `json:"servedFromCache"`
}

type StreamEvent struct {
	Type            string                 `json:"type"`
	Results         []models.SearchResult  `json:"results,omitempty"`
	RelatedSearches []models.RelatedSearch `json:"relatedSearches,omitempty"`
	Chunk           string                 `json:"chunk,omitempty"`
}

func normalize(query string) string {
	return strings.ToLower(strings.TrimSpace(query))
}

func hashQuery(normalized, dateRange string) string {
	sum := sha256.Sum256([]byte(normalized + "|" + dateRange))
	return hex.EncodeToString(sum[:])
}

// Search es el punto de entrada único para resolver una consulta: caché ->
// SerpAPI -> whitelist -> resumen IA -> persistencia -> caché.
func (s *Service) Search(ctx context.Context, rawQuery string) (*Response, error) {
	normalized := normalize(rawQuery)
	hash := hashQuery(normalized, "")

	value, err := s.inFlight.Do(hash, func() (interface{}, error) {
		return s.searchUncached(ctx, rawQuery, normalized, hash)
	})
	if err != nil {
		return nil, err
	}
	return value.(*Response), nil
}

func (s *Service) searchUncached(ctx context.Context, rawQuery, normalized, hash string) (*Response, error) {
	if cached, ok, err := s.cache.Get(hash); err == nil && ok {
		var resp Response
		if jsonErr := json.Unmarshal([]byte(cached), &resp); jsonErr == nil {
			resp.ServedFromCache = true
			return &resp, nil
		}
	}

	results, related, err := s.serpapi.Search(rawQuery, serpapi.SearchOptions{})
	if err != nil {
		return nil, err
	}
	results = s.whitelist.Apply(results)

	summary, promptTokens, completionTokens, err := s.openrouter.Summarize(ctx, rawQuery, results)
	if err != nil {
		// Degradamos con gracia: devolvemos resultados sin resumen en vez de
		// fallar toda la respuesta si solo falló el paso de IA. Pero SIEMPRE
		// logueamos el motivo real, si no es imposible debuggear un resumen
		// vacío.
		log.Printf("aviso: no se pudo generar el resumen con IA para %q: %v", rawQuery, err)
		summary = ""
	}

	resp := &Response{
		Query:           rawQuery,
		Results:         results,
		RelatedSearches: related,
		Summary:         summary,
	}

	if payload, err := json.Marshal(resp); err == nil {
		_ = s.cache.Set(hash, string(payload), s.ttlResults)
	}

	go s.persist(context.Background(), rawQuery, normalized, "", results, summary, promptTokens, completionTokens)

	return resp, nil
}

func (s *Service) SearchStream(ctx context.Context, rawQuery, dateRange string) (<-chan StreamEvent, error) {
	normalized := normalize(rawQuery)
	hash := hashQuery(normalized, dateRange)

	if cached, ok, err := s.cache.Get(hash); err == nil && ok {
		var resp Response
		if jsonErr := json.Unmarshal([]byte(cached), &resp); jsonErr == nil {
			out := make(chan StreamEvent, 3)
			out <- StreamEvent{Type: "results", Results: resp.Results, RelatedSearches: resp.RelatedSearches}
			if resp.Summary != "" {
				out <- StreamEvent{Type: "summary_chunk", Chunk: resp.Summary}
			}
			out <- StreamEvent{Type: "done"}
			close(out)
			return out, nil
		}
	}

	results, related, err := s.serpapi.Search(rawQuery, serpapi.SearchOptions{DateRange: dateRange})
	if err != nil {
		return nil, err
	}
	results = s.whitelist.Apply(results)

	chunks, err := s.openrouter.SummarizeStream(ctx, rawQuery, results)
	if err != nil {
		log.Printf("aviso: no se pudo iniciar el streaming del resumen para %q: %v", rawQuery, err)
		out := make(chan StreamEvent, 2)
		out <- StreamEvent{Type: "results", Results: results, RelatedSearches: related}
		out <- StreamEvent{Type: "done"}
		close(out)
		return out, nil
	}

	out := make(chan StreamEvent)
	go func() {
		defer close(out)
		out <- StreamEvent{Type: "results", Results: results, RelatedSearches: related}

		var full strings.Builder
		for chunk := range chunks {
			full.WriteString(chunk.Content)
			out <- StreamEvent{Type: "summary_chunk", Chunk: chunk.Content}
		}
		out <- StreamEvent{Type: "done"}

		summary := full.String()
		resp := &Response{Query: rawQuery, Results: results, RelatedSearches: related, Summary: summary}
		if payload, err := json.Marshal(resp); err == nil {
			_ = s.cache.Set(hash, string(payload), s.ttlResults)
		}
		s.persist(context.Background(), rawQuery, normalized, dateRange, results, summary, 0, 0)
	}()

	return out, nil
}

func (s *Service) persist(ctx context.Context, rawQuery, normalized, dateRange string, results []models.SearchResult, summary string, promptTokens, completionTokens int) {
	hash := hashQuery(normalized, dateRange)
	searchID, err := s.db.SaveSearch(ctx, rawQuery, normalized, hash, len(results))
	if err != nil {
		return
	}
	_ = s.db.SaveResults(ctx, searchID, results)
	if summary != "" {
		_ = s.db.SaveSummary(ctx, searchID, "google/gemma-4-31b-it:free", summary, promptTokens, completionTokens)
	}
}
