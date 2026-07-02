package db

import (
	"context"

	"agroecologia-search/internal/models"
)

func (d *DB) SaveSearch(ctx context.Context, queryText, normalizedQuery, queryHash string, resultCount int) (string, error) {
	var id string
	err := d.Pool.QueryRow(
		ctx,
		`insert into searches (query_text, normalized_query, query_hash, result_count)
		 values ($1, $2, $3, $4)
		 returning id`,
		queryText, normalizedQuery, queryHash, resultCount,
	).Scan(&id)
	return id, err
}

func (d *DB) SaveResults(ctx context.Context, searchID string, results []models.SearchResult) error {
	// TODO: agrupar en un insert multi-fila (o usar COPY) si esto se vuelve
	// un cuello de botella con mucho tráfico.
	for _, r := range results {
		_, err := d.Pool.Exec(
			ctx,
			`insert into search_results
			 (search_id, rank, url, domain, title, snippet, favicon_url, is_whitelisted, whitelist_weight)
			 values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			searchID, r.Rank, r.URL, r.Domain, r.Title, r.Snippet, r.FaviconURL, r.IsWhitelisted, r.WhitelistWeight,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) SaveSummary(ctx context.Context, searchID, model, summaryText string, inputTokens, outputTokens int) error {
	_, err := d.Pool.Exec(
		ctx,
		`insert into search_summaries (search_id, model, summary_text, input_tokens, output_tokens)
		 values ($1, $2, $3, $4, $5)
		 on conflict (search_id) do update
		 set summary_text = excluded.summary_text,
		     model = excluded.model,
		     input_tokens = excluded.input_tokens,
		     output_tokens = excluded.output_tokens`,
		searchID, model, summaryText, inputTokens, outputTokens,
	)
	return err
}
