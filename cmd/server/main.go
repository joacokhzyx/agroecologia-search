package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"agroecologia-search/internal/cache"
	"agroecologia-search/internal/config"
	"agroecologia-search/internal/db"
	"agroecologia-search/internal/favicon"
	"agroecologia-search/internal/httpapi"
	"agroecologia-search/internal/openrouter"
	"agroecologia-search/internal/search"
	"agroecologia-search/internal/serpapi"
	"agroecologia-search/internal/whitelist"
)

func main() {
	cfg := config.Load()
	database, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("no se pudo conectar a la base de datos: %v", err)
	}
	defer database.Close()

	cacheClient := cache.New(cfg.UpstashRedisRestURL, cfg.UpstashRedisRestToken)
	serpClient := serpapi.New(cfg.SerpAPIKey)
	orClient := openrouter.New(cfg.OpenRouterAPIKey, cfg.OpenRouterModel)

	wl := whitelist.New(database)
	if err := wl.Refresh(context.Background()); err != nil {
		log.Printf("aviso: no se pudo cargar la whitelist todavía: %v", err)
	}
	wl.StartAutoRefresh(5 * time.Minute)

	svc := search.New(cacheClient, database, serpClient, orClient, wl, cfg.CacheTTLResults, cfg.CacheTTLSummary)
	faviconHandler := favicon.NewHandler()

	router := httpapi.NewRouter(svc, faviconHandler)

	log.Printf("agroecología-search escuchando en :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
		log.Fatal(err)
	}
}
