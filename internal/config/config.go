package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                  string
	SerpAPIKey            string
	OpenRouterAPIKey      string
	OpenRouterModel       string
	DatabaseURL           string
	UpstashRedisRestURL   string
	UpstashRedisRestToken string
	CacheTTLResults       int
	CacheTTLSummary       int
}

// LoadDotEnv lee un archivo .env (si existe) y define las variables que no
// estén ya exportadas en el entorno. Nunca sobreescribe variables que ya
// existan en el shell, para que `export X=...` siempre gane.
func LoadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

func Load() Config {
	_ = LoadDotEnv(".env")

	return Config{
		Port:                  getEnv("PORT", "8080"),
		SerpAPIKey:            os.Getenv("SERPAPI_KEY"),
		OpenRouterAPIKey:      os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel:       getEnv("OPENROUTER_MODEL", "google/gemma-4-31b-it:free"),
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		UpstashRedisRestURL:   os.Getenv("UPSTASH_REDIS_REST_URL"),
		UpstashRedisRestToken: os.Getenv("UPSTASH_REDIS_REST_TOKEN"),
		CacheTTLResults:       getEnvInt("CACHE_TTL_RESULTS", 21600),
		CacheTTLSummary:       getEnvInt("CACHE_TTL_SUMMARY", 86400),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
