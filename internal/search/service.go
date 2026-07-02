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
	Query           string                `json:"query"`
	Results         []models.SearchResult `json:"results"`
	Summary         string                `json:"summary"`
	ServedFromCache bool                  `json:"servedFromCache"`
}

func normalize(query string) string {
	return strings.ToLower(strings.TrimSpace(query))
}

func hashQuery(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// Search es el punto de entrada único para resolver una consulta: caché ->
// SerpAPI -> whitelist -> resumen IA -> persistencia -> caché.
func (s *Service) Search(ctx context.Context, rawQuery string) (*Response, error) {
	normalized := normalize(rawQuery)
	hash := hashQuery(normalized)

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

	results, err := s.serpapi.Search(rawQuery)
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
		Query:   rawQuery,
		Results: results,
		Summary: summary,
	}

	if payload, err := json.Marshal(resp); err == nil {
		_ = s.cache.Set(hash, string(payload), s.ttlResults)
	}

	go s.persist(context.Background(), rawQuery, normalized, results, summary, promptTokens, completionTokens)

	return resp, nil
}

func (s *Service) persist(ctx context.Context, rawQuery, normalized string, results []models.SearchResult, summary string, promptTokens, completionTokens int) {
	hash := hashQuery(normalized)
	searchID, err := s.db.SaveSearch(ctx, rawQuery, normalized, hash, len(results))
	if err != nil {
		return
	}
	_ = s.db.SaveResults(ctx, searchID, results)
	if summary != "" {
		_ = s.db.SaveSummary(ctx, searchID, "google/gemma-4-31b-it:free", summary, promptTokens, completionTokens)
	}
}
