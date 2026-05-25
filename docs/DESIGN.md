# Design & Technology Choices

## Architecture

Layered, with clear boundaries:

```
HTTP Request
    → handler (parse, validate HTTP concerns, call service)
    → service (business logic, orchestration, file I/O)
    → repository (SQL queries, DB interactions)
    → model (plain structs, no behavior)
```

Services depend on **repository interfaces** (`TagRepo`, `MediaRepo`), not concrete types. This allows unit tests to run without a database.

## Technology Choices

### Go standard library + minimal deps

| Dep | Why |
|-----|-----|
| `chi/v5` | Lightweight router with clean URL params; avoids heavy frameworks |
| `lib/pq` | Mature, stable Postgres driver |
| `google/uuid` | RFC 4122 UUIDs for IDs — globally unique, no auto-increment leakage |
| `golang-migrate/migrate/v4` | Versioned, rollback-capable migrations; `iofs` source embeds SQL files in the binary |

No ORM. Raw SQL keeps queries explicit and avoids N+1 surprises. The media fetch joins `media_tags` and `tags` in a single query.

### Pagination

Both `GET /tags` and `GET /media` use **keyset (cursor) pagination** on `(created_at DESC, id DESC)`.

**Why cursor over offset:** `OFFSET n` requires a full index scan of the skipped rows — O(n). Keyset skips directly to the next row via an indexed `WHERE` clause — O(log n). Stable under concurrent inserts (no duplicate or skipped rows between pages).

**Cursor encoding:** opaque base64-encoded JSON `{t: timestamp, i: id}`. Clients treat it as a black box.

**Query params:** `limit` (default 20, max 100) and `cursor` (absent on first page).

**Response envelope** (both endpoints):
```json
{
  "items": [...],
  "next_cursor": "eyJ0IjoiMjAyNi0wNS0yMlQwMDowMDowMFoiLCJpIjoidXVpZCJ9"
}
```
`next_cursor` is `null` when no further pages exist.

**Keyset predicate** (PostgreSQL row-value comparison — uses the existing index):
```sql
WHERE (created_at, id) < ($cursor_ts::timestamptz, $cursor_id)
ORDER BY created_at DESC, id DESC
LIMIT $limit
```

For media search with tags the predicate is applied inside the intersection subquery so `idx_media_tags_tag_id` is still hit first.

### Migrations

SQL files live in `internal/migrations/sql/` and are embedded into the binary at compile time via `//go:embed`. At startup, `migrate.Up()` applies any unapplied migrations and is a no-op when the schema is already current.

File convention: `{version}_{title}.{up|down}.sql` (e.g. `000001_initial.up.sql`).

To roll back manually using the `migrate` CLI:
```bash
migrate -path internal/migrations/sql -database "$DATABASE_URL" down 1
```

### PostgreSQL

- Strong support for indexing, joins, and future full-text or tag-based search
- `media_tags` junction table with an index on `tag_id` — enables `SELECT media WHERE tag_id = $1` without a full table scan
- UUID primary keys (stored as TEXT for simplicity; could use native UUID type in a larger system)

### File storage

Files are saved to a local directory and served via `http.FileServer`. This is intentionally simple for the assignment scope. In production this would be replaced by S3 or GCS (see "What I would improve" below).

### File validation

File type is determined from **magic bytes** (`internal/filevalidation`), not the filename extension. The extension is kept for the saved filename only.

| Supported format | Magic bytes |
|---|---|
| JPEG | `FF D8 FF` |
| PNG | `89 50 4E 47 0D 0A 1A 0A` |
| GIF | `47 49 46 38` (GIF8…) |
| WebP | `52 49 46 46` + `57 45 42 50` at offset 8 |
| MP4 / MOV | `66 74 79 70` (`ftyp` box) at offset 4 |
| MKV / WebM | `1A 45 DF A3` (EBML header) |
| AVI | `52 49 46 46` + `41 56 49 20` at offset 8 |

RIFF-based formats (WebP vs AVI) are disambiguated by the four bytes at offset 8.

**Size limits** are enforced per detected type while streaming to disk — no buffering in memory:

| Type | Limit |
|---|---|
| Photo | 20 MB |
| Video | 500 MB |

An `io.LimitedReader` (N = limit + 1) detects overflow: if `written > limit`, the partial file is deleted and `ErrFileTooLarge` (→ HTTP 422) is returned.

### Authentication — clients and API keys

Every route requires a valid API key. There are no unauthenticated HTTP endpoints. Clients and keys are provisioned out-of-band via the `seed` binary.

**Schema:**
```sql
clients (id, name, created_at)
api_keys (id, client_id, key_hash, created_at)  -- key_hash indexed for O(1) lookup
```

Raw keys are never stored. On creation the service generates `sp_` + 32 random bytes (hex-encoded), returns the raw key once to the caller, and stores only its SHA-256 hash.

**Key format:** `sp_<64 hex chars>` — the `sp_` prefix makes keys identifiable in logs/config.

**Lookup:** `X-API-Key: <key>` or `Authorization: Bearer <key>` → SHA-256 hash → `JOIN api_keys ON key_hash` → client row. One indexed query per request.

**Provisioning via seed binary:**
```bash
# Local
go run ./cmd/seed -name "Acme Corp"

# Docker (after `docker compose up`)
docker compose run --rm api ./seed -name "Acme Corp"

# → client_id : <uuid>
# → key       : sp_<64hex>   ← store this, shown once
```

`ON DELETE CASCADE` on `api_keys.client_id` — deleting a client revokes all its keys automatically.

## Database Schema Design

```sql
tags (id, name, created_at)
media (id, name, type, file_url, created_at)
media_tags (media_id, tag_id)  -- composite PK + index on tag_id
```

The `idx_media_tags_tag_id` index is used by `GET /media?tag_id=uuid&tag_id=uuid2`.

The intersection query finds media that have **all** requested tags:
```sql
SELECT mt.media_id
FROM media_tags mt
WHERE mt.tag_id = ANY($1)        -- hits idx_media_tags_tag_id
GROUP BY mt.media_id
HAVING COUNT(DISTINCT mt.tag_id) = $2   -- $2 = len(tag_ids)
```

Omitting `tag_id` returns all media. Multiple `tag_id` params are ANDed (intersection), not ORed.

A further optimization for high-cardinality multi-tag queries would be a GIN index on a `tag_ids` array column (PostgreSQL-specific).

## Integration Tests

Repository-level integration tests live in `internal/repository/` under the `//go:build integration` build tag. They spin up a real PostgreSQL container via `testcontainers-go` and run the full migration before any test, catching SQL bugs (wrong column order, bad JOIN, missing index) that in-memory fakes cannot detect.

**Coverage:**
- `tag_integration_test.go` — `Create`, paginated `List` (cursor round-trip), `ExistsByIDs` (all found, partial miss, empty)
- `media_integration_test.go` — `Create` with/without tags, `GetByID` (found + not-found), `Search` (no filter, single tag, intersection of two tags, pagination, empty), `ON DELETE CASCADE` on `media_tags`
- `client_integration_test.go` — `Create`, `CreateAPIKey`, `FindByKeyHash` (hit + miss), cascade delete (deleting client revokes all keys), multiple keys per client

**Shared setup (`integration_test.go`):**
- `TestMain` starts `postgres:16-alpine` via `testcontainers-go`, runs `database.RunMigrations`, exposes `testDB *sql.DB` to all tests.
- `truncate(t)` is called at the top of each test — it issues `TRUNCATE api_keys, media_tags, media, tags, clients` for a clean slate without restarting the container.

**Run:**
```bash
go test -tags integration ./internal/repository/...
```

Requires Docker (or OrbStack / Docker Desktop) to be running.

## Error Format

All error responses use a consistent envelope so clients can branch on `code` without parsing `message` strings:

```json
{
  "error": {
    "code": "TAG_NOT_FOUND",
    "message": "media not found"
  }
}
```

**Codes** (`internal/handler/response.go`):

| Code | HTTP | Trigger |
|---|---|---|
| `INVALID_REQUEST` | 400 | Bad JSON, missing file, invalid cursor |
| `VALIDATION_ERROR` | 422 | Empty name fields |
| `TAGS_NOT_FOUND` | 422 | One or more tag IDs don't exist |
| `UNSUPPORTED_FORMAT` | 422 | File magic bytes not recognized |
| `FILE_TOO_LARGE` | 422 | File exceeds per-type size limit |
| `MEDIA_NOT_FOUND` | 404 | `GET /media/{id}` unknown ID |
| `API_KEY_REQUIRED` | 401 | No `X-API-Key` / `Authorization` header |
| `INVALID_API_KEY` | 401 | Key hash not found |
| `INTERNAL_ERROR` | 500 | Unexpected server-side failure |

## Observability

All three pillars live in `internal/observability/`.

### Metrics (Prometheus)

`MetricsMiddleware` records three metrics per request:

| Metric | Type | Labels |
|---|---|---|
| `http_requests_total` | Counter | `method`, `route`, `status` |
| `http_request_duration_seconds` | Histogram | `method`, `route` |
| `http_requests_in_flight` | Gauge | — |

`route` uses the matched chi route pattern (`/media/{id}`) rather than the raw URL path, preventing label cardinality explosion. Scrape target: `GET /metrics` (unauthenticated).

### Distributed Tracing (OpenTelemetry)

`InitTracer` configures a global `TracerProvider`. HTTP spans are created automatically via `otelhttp.NewMiddleware` with the matched route as the span name. W3C TraceContext + Baggage propagation headers are respected, so traces chain across services.

Exporter selection at startup:
- `OTEL_EXPORTER_OTLP_ENDPOINT` set → OTLP/HTTP exporter (Jaeger, Grafana Tempo, etc.)
- Unset → noop exporter (zero overhead, nothing emitted)

### Structured log correlation IDs

`requestLogger` emits one JSON log line per request with:

```json
{
  "method": "GET",
  "path": "/media/abc",
  "status": 200,
  "duration_ms": 4,
  "request_id": "abc123",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7"
}
```

`request_id` comes from chi's `middleware.RequestID`. `trace_id`/`span_id` are extracted from the active OTel span — only present when tracing is enabled. The logger runs inside the OTel middleware so the span is already on the context when the log line is written.

### Unprotected endpoints

`GET /health`, `GET /metrics`, and `GET /openapi.yaml` are registered before the auth middleware and are always accessible without an API key.

## OpenAPI Spec

The spec lives at `internal/apispec/openapi.yaml` (OpenAPI 3.0.3) and is embedded into the binary at compile time via `//go:embed`.

Served at `GET /openapi.yaml` — no authentication required:

```bash
curl http://localhost:8080/openapi.yaml
```

Use with any OpenAPI-compatible tool:

```bash
# Swagger UI (Docker)
docker run -p 8081:8080 -e URL=http://localhost:8080/openapi.yaml swaggerapi/swagger-ui

# Redoc CLI
npx @redocly/cli preview-docs http://localhost:8080/openapi.yaml

# Postman — import URL directly
```

**Spec covers:** all 7 endpoints, both auth schemes (`X-API-Key` / `Authorization: Bearer`), all error codes with examples, pagination parameters, and multipart upload schema.

## Load Tests

k6 scripts live in `loadtest/k6/`. Run them against a running instance:

```bash
# Provision a key first (once)
go run ./cmd/seed -name "load-test"
export API_KEY=sp_<key>

# Smoke  — 1 VU, 30 s — run after every deployment
k6 run -e API_KEY=$API_KEY loadtest/k6/smoke.js

# Load   — 50 VUs sustained 5 min — SLO gating
k6 run -e API_KEY=$API_KEY loadtest/k6/load.js

# Stress — ramps to 200 VUs — finds breaking point
k6 run -e API_KEY=$API_KEY loadtest/k6/stress.js
```

Override base URL for a remote host: `-e BASE_URL=https://api.example.com`.

| Script | VUs | Duration | Purpose |
|---|---|---|---|
| `smoke.js` | 1 | 30 s | Verify every endpoint returns expected status; gates deploys |
| `load.js` | 0 → 50 → 0 | ~6.5 min | Realistic 80/20 read-write mix; enforces latency SLOs in CI |
| `stress.js` | 10 → 200 → 10 | ~11 min | Find the saturation point; observe recovery |

**`smoke.js` thresholds** (fail = broken deploy):
- `http_req_failed < 1 %`
- `http_req_duration p(99) < 1 s`
- `http_req_duration{type:read} p(95) < 300 ms`

**`load.js` thresholds** (SLOs):
- Read endpoints: `p(95) < 300 ms`, `p(99) < 500 ms`
- Write endpoints: `p(95) < 600 ms`, `p(99) < 1 000 ms`
- `write_error_rate < 2 %`

Intentional error requests (401, 404, 422) use `responseCallback: http.expectedStatuses(N)` so they are not counted as failures by k6.

Shared constants (base URL, auth headers, minimal JPEG/MP4 bytes) are in `loadtest/k6/helpers.js`.

## What I would improve with more time

1. **Object storage** — replace local file storage with S3/GCS; store only the object key in the DB; generate pre-signed URLs for retrieval
1. **CI integration for load tests** — run `smoke.js` as a post-deploy gate in the pipeline; run `load.js` nightly against staging and publish the k6 summary as a build artifact
1. **API key rotation** — endpoint to issue a replacement key and revoke the old one without downtime; short-lived keys with a `expires_at` column
1. **Rate limiting** — per-client token-bucket middleware (e.g. `golang.org/x/time/rate`) to protect write endpoints from abuse; expose current limits in response headers
1. **Soft delete / media lifecycle** — `deleted_at` column on `media`; `DELETE /media/{id}` tombstones the row and schedules the upload file for async cleanup rather than deleting immediately
1. **Full-text search on media name** — PostgreSQL `tsvector` + GIN index; expose as `GET /media?q=<query>` for free-text name search alongside tag filtering
1. **Async video processing** — after upload, push a job to a queue (e.g. Redis Streams); a worker transcodes to HLS, extracts a thumbnail, and updates the DB; avoids blocking the HTTP handler on heavy I/O


## Additional Notes

- The `nopFile` adapter in tests wraps `strings.Reader` to satisfy `multipart.File`. This avoids writing real files in unit tests while still exercising the file-handling path.
- The `media_tags` junction table uses `ON DELETE CASCADE` so deleting a tag or media cleans up orphan rows automatically.
- `TIMESTAMPTZ` is used throughout (not `TIMESTAMP`) to avoid timezone-naive datetime bugs if the DB server timezone ever changes.
