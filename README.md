# Scoreplay Media API

HTTP API for creating and retrieving media (photos and videos) with tags.

## Quick Start

### With Docker (recommended)

```bash
docker compose up --build
```

API available at `http://localhost:8080`.

Then provision a client to get an API key:

```bash
docker compose run --rm api ./seed -name "My Client"
# client_id : <uuid>
# key       : sp_<64hex>   ← store this, shown once
```

### Without Docker

Requires PostgreSQL running locally.

```bash
# Set env vars (or use defaults)
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=scoreplay
export DB_PASSWORD=scoreplay
export DB_NAME=scoreplay
export BASE_URL=http://localhost:8080

go run ./cmd/api
```

Then provision a client:

```bash
go run ./cmd/seed -name "My Client"
# client_id : <uuid>
# key       : sp_<64hex>   ← store this, shown once
```

## API Reference

### Create a Tag

```
POST /tags
Content-Type: application/json

{"name": "Lionel Messi"}
```

Response `201`:
```json
{"id": "uuid", "name": "Lionel Messi", "created_at": "..."}
```

---

### List All Tags

```
GET /tags
GET /tags?limit=20&cursor=<next_cursor>
```

Response `200`:
```json
{
  "items": [{"id": "uuid", "name": "Lionel Messi", "created_at": "..."}],
  "next_cursor": "<opaque>"
}
```
`next_cursor` is `null` on the last page.

---

### List / Search Media

```
GET /media
GET /media?limit=20&cursor=<next_cursor>
GET /media?tag_id=<uuid>
GET /media?tag_id=<uuid1>&tag_id=<uuid2>&limit=10&cursor=<next_cursor>
```

Returns paginated media. With `tag_id` params, returns only media that have **all** specified tags (intersection). `limit` defaults to 20, max 100.

Response `200`:
```json
{
  "items": [
    {
      "id": "uuid",
      "name": "Goal Highlight",
      "type": "video",
      "file_url": "http://localhost:8080/uploads/uuid.mp4",
      "tags": [{"id": "uuid", "name": "Messi", "created_at": "..."}],
      "created_at": "..."
    }
  ],
  "next_cursor": "<opaque>"
}
```
`next_cursor` is `null` on the last page.

---

### Create a Media

```
POST /media
Content-Type: multipart/form-data

Fields:
  name   string   (required) — media name
  tags   string   (optional) — comma-separated tag IDs, e.g. "id1,id2"
  file   binary   (required) — photo or video file
```

Supported photo formats: `.jpg`, `.jpeg`, `.png`, `.gif`, `.webp`  
Supported video formats: `.mp4`, `.mov`, `.avi`, `.mkv`, `.webm`

Response `201`:
```json
{
  "id": "uuid",
  "name": "Goal Highlight",
  "type": "video",
  "file_url": "http://localhost:8080/uploads/uuid.mp4",
  "tags": [{"id": "uuid", "name": "Messi", "created_at": "..."}],
  "created_at": "..."
}
```

---

### Retrieve a Media

```
GET /media/{id}
```

Response `200`:
```json
{
  "id": "uuid",
  "name": "Goal Highlight",
  "type": "video",
  "file_url": "http://localhost:8080/uploads/uuid.mp4",
  "tags": [{"id": "uuid", "name": "Messi", "created_at": "..."}],
  "created_at": "..."
}
```

---

### Serve Uploaded Files

```
GET /uploads/{filename}
```

---

## Error Responses

All errors return a machine-readable envelope:
```json
{"error": {"code": "TAG_NOT_FOUND", "message": "tag not found"}}
```

| Status | Code | Trigger |
|--------|------|---------|
| 400 | `INVALID_REQUEST` | Malformed JSON, missing file, invalid cursor |
| 401 | `API_KEY_REQUIRED` | No `X-API-Key` / `Authorization` header |
| 401 | `INVALID_API_KEY` | Key not recognised |
| 404 | `MEDIA_NOT_FOUND` | Unknown media ID |
| 422 | `VALIDATION_ERROR` | Empty name fields |
| 422 | `TAGS_NOT_FOUND` | One or more tag IDs don't exist |
| 422 | `UNSUPPORTED_FORMAT` | File magic bytes not recognised |
| 422 | `FILE_TOO_LARGE` | File exceeds per-type size limit |
| 500 | `INTERNAL_ERROR` | Unexpected server-side failure |

---

## Running Tests

**Unit tests** (no dependencies):
```bash
go test ./...
```

**Integration tests** (requires Docker / OrbStack running):
```bash
go test -tags integration ./internal/repository/...
```

Integration tests spin up a real `postgres:16-alpine` container via `testcontainers-go`, run migrations, and exercise the full SQL layer. See `docs/DESIGN.md` → *Integration Tests* for details.

## Load Tests

Requires [k6](https://k6.io/docs/getting-started/installation/) and a running instance.

```bash
export API_KEY=sp_<your-key>

# Smoke — 1 VU, 30 s — run after every deployment
k6 run -e API_KEY=$API_KEY loadtest/k6/smoke.js

# Load — 50 VUs, ~6.5 min — realistic traffic, SLO gating
k6 run -e API_KEY=$API_KEY loadtest/k6/load.js

# Stress — ramps to 200 VUs — find breaking point
k6 run -e API_KEY=$API_KEY loadtest/k6/stress.js
```

Override target: `-e BASE_URL=https://api.example.com`. See `docs/DESIGN.md` → *Load Tests* for thresholds and traffic mix details.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | — | Full Postgres DSN (overrides individual DB_* vars) |
| `DB_HOST` | `localhost` | |
| `DB_PORT` | `5432` | |
| `DB_USER` | `scoreplay` | |
| `DB_PASSWORD` | `scoreplay` | |
| `DB_NAME` | `scoreplay` | |
| `PORT` | `8080` | HTTP listen port |
| `UPLOAD_DIR` | `./uploads` | Local directory for uploaded files |
| `BASE_URL` | `http://localhost:8080` | Prefix used to build file URLs |

---

## cURL Examples

```bash
export API_KEY=sp_<your-key>

# Create tag
curl -X POST http://localhost:8080/tags \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"Lionel Messi"}'

# List tags
curl http://localhost:8080/tags \
  -H "X-API-Key: $API_KEY"

# Create media (photo, with tags)
curl -X POST http://localhost:8080/media \
  -H "X-API-Key: $API_KEY" \
  -F "name=Training Session" \
  -F "tags=<tag-id-here>" \
  -F "file=@/path/to/photo.jpg"

# Retrieve media
curl http://localhost:8080/media/<media-id> \
  -H "X-API-Key: $API_KEY"
```
