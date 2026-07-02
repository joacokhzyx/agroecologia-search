package whitelist

import (
	"context"
	"sort"
	"sync"
	"time"

	"agroecologia-search/internal/db"
	"agroecologia-search/internal/models"
)

// List mantiene en memoria una copia de la whitelist, refrescada
// periódicamente, para que reordenar resultados nunca dependa de la base de
// datos en el camino crítico de una búsqueda.
type List struct {
	mu      sync.RWMutex
	weights map[string]int
	db      *db.DB
}

func New(database *db.DB) *List {
	return &List{
		weights: make(map[string]int),
		db:      database,
	}
}

// Refresh recarga la whitelist desde la base de datos. Llamala al arrancar y
// periódicamente (ver StartAutoRefresh).
func (l *List) Refresh(ctx context.Context) error {
	rows, err := l.db.Pool.Query(ctx, `select domain, weight from whitelist_domains`)
	if err != nil {
		return err
	}
	defer rows.Close()

	weights := make(map[string]int)
	for rows.Next() {
		var domain string
		var weight int
		if err := rows.Scan(&domain, &weight); err != nil {
			return err
		}
		weights[domain] = weight
	}

	l.mu.Lock()
	l.weights = weights
	l.mu.Unlock()
	return nil
}

func (l *List) StartAutoRefresh(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			_ = l.Refresh(context.Background())
		}
	}()
}

func (l *List) WeightFor(domain string) (int, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	w, ok := l.weights[domain]
	return w, ok
}

// Apply marca los resultados que pertenecen a la whitelist y los reordena
// para que los dominios de confianza floten arriba (mayor peso primero),
// preservando el orden relativo de SerpAPI para el resto.
func (l *List) Apply(results []models.SearchResult) []models.SearchResult {
	for i := range results {
		if w, ok := l.WeightFor(results[i].Domain); ok {
			results[i].IsWhitelisted = true
			results[i].WhitelistWeight = w
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].WhitelistWeight != results[j].WhitelistWeight {
			return results[i].WhitelistWeight > results[j].WhitelistWeight
		}
		return results[i].Rank < results[j].Rank
	})

	return results
}
