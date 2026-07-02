# Agroecología / Motor de búsqueda

Servicio independiente (Go + Postgres + Redis) que resuelve búsquedas combinando SerpAPI, una whitelist de dominios de confianza y un resumen generado por IA vía OpenRouter.

No usa Docker: corre como un binario nativo de Go, y la base de datos / caché son servicios administrados gratuitos (Supabase + Upstash), así que no hay nada que instalar aparte de Go.

## Stack

- **Go** (estándar `net/http`, sin frameworks web) — un solo binario, cross-compila a Windows y Linux sin drama.
- **PostgreSQL vía Supabase** — hosteado, capa gratuita, listo para sumar `pgvector` el día que quieran búsqueda semántica.
- **Redis vía Upstash** — REST API, sin instalar Redis en ningún lado.
- **SerpAPI** — resultados de Google.
- **OpenRouter** (`google/gemma-4-31b-it:free`) — resumen generado por IA.

## 1. Crear los servicios (una sola vez)

1. **Supabase**: creá un proyecto en supabase.com (plan gratis). Copiá el "Connection string" en modo *Session pooler* (puerto 6543).
2. En el **SQL Editor** de Supabase, corré en orden:
   - `migrations/0001_init.sql`
   - `migrations/0002_seed_whitelist.sql`
3. **Upstash**: creá una base de Redis gratis en upstash.com. Copiá `UPSTASH_REDIS_REST_URL` y `UPSTASH_REDIS_REST_TOKEN` desde el tab "REST API".
4. **SerpAPI**: obteneé tu API key en serpapi.com (tiene un free tier de 100 búsquedas/mes).
5. **OpenRouter**: obteneé tu API key en openrouter.ai.

## 2. Configurar el proyecto

```bash
cp .env.example .env
# completá .env con las claves del paso anterior
```

## 3. Instalar Go (si no lo tenés)

Descargalo de https://go.dev/dl/ (instalador para Windows, o el paquete para tu distro de Linux). Con eso alcanza, no hace falta nada más.

## 4. Correr en desarrollo

```bash
go mod tidy
go run ./cmd/server
```

Esto descarga las dependencias (solo el driver de Postgres, `pgx`) y levanta el server en `http://localhost:8080`.

Probá:

```bash
curl "http://localhost:8080/api/search?q=control+de+plagas+organico"
```

## 5. Compilar un binario (para producción, o para probar en la otra plataforma)

```bash
# Windows (desde Linux o Windows)
GOOS=windows GOARCH=amd64 go build -o bin/server.exe ./cmd/server

# Linux (desde Windows o Linux)
GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/server
```

En PowerShell, seteo de variables antes del build:

```powershell
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o bin/server ./cmd/server
```

El binario resultante no necesita Go instalado para correr, ni en Windows ni en Linux.

## Estructura

```
cmd/server/main.go       # arranque: wiring de config, db, caché, clientes externos
internal/config/         # carga de .env + variables de entorno
internal/models/         # tipos compartidos (SearchResult, etc.)
internal/cache/          # cliente REST de Upstash Redis
internal/db/             # pool de Postgres + queries de persistencia
internal/serpapi/        # cliente de SerpAPI
internal/openrouter/     # cliente de OpenRouter (resumen IA)
internal/whitelist/      # whitelist en memoria + reordenamiento de resultados
internal/favicon/        # proxy de favicons con headers de caché largos
internal/search/         # orquestador: caché -> SerpAPI -> whitelist -> IA -> persistencia
internal/httpapi/        # rutas HTTP (net/http estándar, sin framework)
migrations/              # SQL para correr en el SQL Editor de Supabase
```

## Flujo de una búsqueda (`GET /api/search?q=...`)

1. Se normaliza la consulta y se calcula un hash.
2. Se deduplican consultas idénticas concurrentes (`internal/search/singleflight.go`) para no pegarle dos veces a SerpAPI si dos usuarios buscan lo mismo al mismo tiempo.
3. Se busca en Redis (Upstash). Si hay hit, se devuelve directo.
4. Si no hay hit: se consulta SerpAPI, se reordenan/marcan los resultados según `whitelist_domains`, se genera un resumen con OpenRouter, se cachea la respuesta completa, y en paralelo (goroutine) se persiste todo en Postgres.

## Pendientes conocidos (a propósito, para no sobre-construir de entrada)

- Rate limiting de salida hacia SerpAPI/OpenRouter (para no comerte la cuota si hay un pico de tráfico).
- Autenticación/limitación por IP o usuario en el propio endpoint.
- Logging estructurado y métricas (hoy es `log.Printf` simple).
- Insert por lote en `search_results` en vez de fila por fila.
- Endpoint para administrar `whitelist_domains` (hoy se edita a mano en Supabase).
