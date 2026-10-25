<div align="center">
  <a href="https://waifu.best">
    <img src="https://waifu.best/logo.png" alt="Waifu API Logo" width="140" style="border-radius: 20px; box-shadow: 0 8px 30px rgba(0,0,0,0.12);">
  </a>

  <h1>Waifu API</h1>

  <p>
    <strong>The versatile waifu image provider.</strong><br>
    Access thousands of categorized anime-style images via a robust REST API or browse freely.
  </p>

  <p>
    <a href="https://waifu.best"><strong>Website</strong></a> •
    <a href="https://docs.waifu.best"><strong>Documentation</strong></a> •
    <a href="https://waifu.best/contact/"><strong>Support</strong></a>
  </p>

  <p>
    <img src="https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react&logoColor=white" alt="React">
    <img src="https://img.shields.io/badge/TypeScript-5-3178C6?style=flat-square&logo=typescript&logoColor=white" alt="TypeScript">
    <img src="https://img.shields.io/badge/PostgreSQL-18-4169E1?style=flat-square&logo=postgresql&logoColor=white" alt="PostgreSQL">
    <img src="https://img.shields.io/badge/Docusaurus-Docs-2ECC71?style=flat-square&logo=docusaurus&logoColor=white" alt="Docusaurus">
    <img src="https://img.shields.io/badge/Docker-Ready-blue?style=flat-square&logo=docker&logoColor=white" alt="Docker">
  </p>
</div>

## 📖 About

Waifu API is an open-source waifu image platform: a searchable, taggable anime-image archive with
a full REST API, a moderation/review workflow, albums & favorites, Discord OAuth2 login and API keys.

The backend in [`backend/`](backend/) was **rewritten in Go** as a **wire-compatible drop-in
replacement** for the original ASP.NET Core implementation (which lives in git history): the React
frontend works against either implementation without any changes. Status codes, JSON field names,
error formats, review semantics and even the quirks of the old API are reproduced 1:1.

## ✨ Features

- 🖼️ **Extensive Archive** – thousands of high-quality anime-style images with a rich REST API.
- 🔍 **Advanced Filtering** – orientation, resolution, file size, NSFW toggle, GIFs, artists, tags,
  force-include/exclude specific files (all combinable, AND/NOT semantics).
- 🏷️ **Tags & Artists** – community-curated metadata with a full moderation review queue.
- 📁 **Albums & Favorites** – organize and share collections; every account gets a default "Favorites" album.
- 👤 **User Accounts** – Discord OAuth2 login, 7-day JWTs and long-lived API keys (`X-Api-Key`).
- 🛡️ **Moderation** – report system, review queues for uploads/edits/tags/artists, user ban & role management,
  site banner.
- 👯 **Duplicate Detection** – 64-bit perceptual hashing with Hamming-distance comparison (bit-exact port of the
  original `CoenM.ImageHash` algorithm, so hashes stay comparable across the rewrite).
- 📊 **Statistics** – public counters plus an admin dashboard (requests, top users, top uploaders, history).
- ⚡ **Performance** – CDN integration with on-the-fly image resizing and single-image cache purging.
- 🔐 **Security** – hashed API keys, OAuth state nonce, blacklist middleware, per-IP rate limiting,
  CORS allowlist, security headers.

## 🏗️ Architecture

| Component  | Technology | Description |
|------------|-----------|-------------|
| Backend    | **Go 1.24** (net/http, pgx, aws-sdk-go-v2) | REST API, single static binary, no runtime dependencies |
| Frontend   | **React 18 + Vite + Tailwind** | SPA served by nginx, runtime config via `/config.js` |
| Database   | **PostgreSQL 18** (pgvector image) | All data; EF-Core schema (PascalCase quoted identifiers) used as-is |
| Storage    | **Cloudflare R2** (S3-compatible) | Original images, public-read |
| CDN        | Cloudflare | `CDN_BASE_URL` + optional image resizing (`IMG_TRANSFORM_*`) |
| Auth       | Discord OAuth2 | JWT (7 days) + API keys (`X-Api-Key`) |
| Docs       | Docusaurus + OpenAPI | Static spec at [`openapi/WaifuApi.Web.json`](openapi/WaifuApi.Web.json), generated API reference |

The Go backend keeps the exact database schema of the .NET original (no migration needed when
switching), and every REST endpoint, DTO, status code and error string is mirrored – including
deliberately preserved quirks (e.g. `PATCH /users/{id}/albums/{albumId}` answering `200` with a
`null` body for unknown albums).

## 🚀 Getting Started

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) & [Docker Compose](https://docs.docker.com/compose/install/)
- A [Discord Application](https://discord.com/developers/applications) (for authentication)
- An S3-compatible bucket (e.g. [Cloudflare R2](https://www.cloudflare.com/r2/))

### Production (docker compose)

1. **Clone the repository**
   ```bash
   git clone https://github.com/maximiliangt500/waifu-api.git
   cd waifu-api
   ```
2. **Configure environment**
   ```bash
   cp .env.example .env
   ```
   Fill in your values (see [Configuration](#-configuration)).
3. **Launch**
   ```bash
   docker compose up -d --build
   ```
   Services: backend (`:64533`), frontend (`:64534`), docs (`:64535`), PostgreSQL (internal).

### Local development

```bash
# Database only (Docker)
docker compose up -d db

# Backend (Go 1.24+)
cd backend
go run ./cmd/server -env ../.env -addr :8080

# Frontend (Node 20+)
cd frontend
npm install
npm run dev        # http://localhost:5173, proxies /api to localhost:8080

# Docs (Node 18+)
cd docs
npm install
npm run gen-api-docs   # generate the API reference from the OpenAPI spec (once)
npm run start          # http://localhost:3000
```

For local development set `DISCORD_REDIRECT_URI` and `VITE_DISCORD_REDIRECT_URI` to
`http://localhost:5173/auth/callback` and register that URI in the Discord developer portal.

## ⚙️ Configuration

All configuration lives in **one** `.env` file at the repository root (flat keys, shared by the
backend, frontend and docker-compose). See [`.env.example`](.env.example) for the complete,
commented template.

| Section | Keys | Description |
|---------|------|-------------|
| Ports | `BACKEND_PORT`, `FRONTEND_PORT`, `DOCS_PORT` | Host ports |
| Database | `POSTGRES_USER/PASSWORD/DB/HOST/PORT` | PostgreSQL connection |
| URLs | `API_BASE_URL`, `FRONTEND_URL`, `DOCS_URL`, `STATUS_URL` | Public endpoints |
| Discord | `DISCORD_CLIENT_ID/SECRET/REDIRECT_URI/SERVER_URL` | OAuth2 credentials |
| JWT | `JWT_KEY` (≥32 chars), `JWT_ISSUER`, `JWT_AUDIENCE` | Token signing/validation |
| Storage | `S3_ACCESS_KEY/SECRET_KEY/SERVICE_URL/REGION/BUCKET_NAME/FORCE_PATH_STYLE/VERIFY_SSL` | Cloudflare R2 |
| CDN | `CDN_BASE_URL`, `CLOUDFLARE_ZONE_ID/API_TOKEN` | Image URLs + cache purging |
| Review | `REVIEW_*` | Review requirements & per-type role bypasses (`"None"` = no bypass) |
| Permissions | `PERMISSIONS_*` | Minimum roles for uploads/edits/banner |
| Limits | `IMAGE_*`, `*_DEFAULT_PAGE_SIZE`, `*_MAX_PAGE_SIZE`, `RATE_LIMIT_*` | Upload/pagination/rate limits |
| Frontend | `VITE_*` | Client config (links, titles, Discord client id) |

The backend fails fast at startup when a required setting is missing (or `JWT_KEY` is too short).

## 🔑 Setting up an admin user

1. Log in once on the website with your Discord account (creates the user row).
2. Promote in the database:
   ```sql
   UPDATE public."Users" SET "Role" = 3 WHERE "DiscordId" = '<YOUR_DISCORD_ID>';
   ```
   Roles: `0` User, `1` TrustedUser, `2` Moderator, `3` Admin.
3. Log out and back in so the new role is included in your JWT.

## 📚 Documentation

- **Documentation**: [`docs/`](docs/) (Docusaurus) – getting started, authentication, tags, versioning,
  terms of service and a fully generated API reference from the OpenAPI spec at
  [`openapi/WaifuApi.Web.json`](openapi/WaifuApi.Web.json).
- Regenerate API docs after spec changes: `cd docs && npm run gen-api-docs`.

## 🌐 API Overview

Base path is configurable via `API_BASE_PATH` (default `/`). JSON is `camelCase`, enums are strings,
dates are ISO 8601 UTC, errors are RFC 9457 `application/problem+json`.

| Area | Endpoints |
|------|-----------|
| Auth | `POST /auth/discord`, `GET|POST /auth/api-keys`, `PATCH|DELETE /auth/api-keys/{id}` |
| Images | `GET /images`, `GET /images/{id}`, `POST /images/upload`, `PATCH|DELETE /images/{id}` |
| Tags / Artists | `GET|POST /tags[/{id}|/by-slug/{slug}]`, `PATCH|DELETE /tags/{id}` (+ `/artists` analog) |
| Review | `POST /review/tasks`, `GET /review/tasks[/{id}]`, `PATCH …/{id}`, `…/approve`, `…/reject`, `…/cancel` |
| Albums | `/users/{userId}/albums` + `/{albumId}[/images[/{imageId}]]` (`me`, `favorites` aliases) |
| Reports | `POST /reports`, `GET /reports`, `PATCH /reports/{id}` + `/validate`, `/reject`, `/cancel` |
| Banner | `GET /banner`, `GET|PUT /banner/settings` |
| Stats | `GET /stats/public`, `GET /stats/admin` (admin) |
| Storage Diff | `GET /storage-diff`, `DELETE /storage-diff/s3`, `DELETE /storage-diff/db` (admin) |

Authentication: `Authorization: Bearer <JWT>` or the `X-Api-Key` header. Anonymous access is
available for browsing (images, tags, artists, public stats, banner).

## 🔐 Security

- Per-IP rate limiting (`RATE_LIMIT_REQUESTS_PER_MINUTE` / `RATE_LIMIT_BURST`).
- CORS restricted to `FRONTEND_URL` (+ `CORS_EXTRA_ORIGINS`).
- API keys stored as SHA-256 hashes; shown only once at creation.
- JWT with issuer/audience validation; OAuth state nonce for the Discord login.
- Blacklisted users are rejected on every request; security headers on all responses.

## 🔄 .NET → Go compatibility

The Go backend reproduces the .NET API exactly, including its historical quirks:

- `GET /images` defaults to `pageSize=1` (the frontend always sends an explicit size).
- `DELETE /auth/api-keys/{id}` is idempotent (204 even for unknown ids).
- `PATCH /users/{userId}/albums/{albumId}` on an unknown album answers `200` with a `null` body.
- Album image lists ignore `includeMyPending`; `uploaderId` filters are silently dropped for non-mods.
- `GET /artists/by-name/{name}` compares case-sensitively.
- API-key 401/403 challenges use PascalCase keys; JWT challenges return an empty body with
  `WWW-Authenticate`.
- `perceptualHash` values (including the byte order of the stored `bit(64)` column) are fully
  interchangeable with the old backend.

Deliberate differences (documented in [`.env.example`](.env.example)):

- **Rate limiting** (per-IP token bucket, configurable) – the .NET backend had none.
- **CORS allowlist** instead of `AllowAnyOrigin`.
- Additional endpoints: `GET /healthz` (container health check) and static `/openapi/v1.json` + `/scalar`.

## 🧪 Development

```bash
# Backend: build & vet (Go 1.24+)
cd backend && go build ./... && go vet ./...

# Frontend: type check + production build
cd frontend && npm install && npm run build

# Docs: build
cd docs && npm install && npm run build
```

The backend intentionally carries no checked-in `go.sum`; `go mod tidy` resolves the graph
(also done inside the Docker build).

## 📄 License

This project is licensed under the [MIT License](LICENSE).
