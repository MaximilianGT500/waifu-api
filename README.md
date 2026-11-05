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
    <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=white" alt="React">
    <img src="https://img.shields.io/badge/TypeScript-7-3178C6?style=flat-square&logo=typescript&logoColor=white" alt="TypeScript">
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
- 🎴 **Two designs** – the *Collector* design (binder pages with nine card pockets, holo cards, card detail
  pages, in the colours of the previous design) and the previous *Classic* design; six accent colours to choose
  from, and animations and effects can be switched off. Visitors switch in the theme menu; the choice is kept in the
  `waifu_design` cookie and the frontend's nginx serves the matching build on every URL.
- ⚡ **Performance** – CDN integration with on-the-fly image resizing and single-image cache purging, short-lived
  in-memory caches for public lists with ETag/304 revalidation and cluster-wide invalidation, batched database
  queries, subsetted self-hosted fonts and responsive images.
- 🔐 **Security** – hashed API keys, strict OAuth state nonce, database-authoritative roles & bans, per-IP rate
  limiting with trusted-proxy handling, decompression-bomb protection, Content-Security-Policy, configurable CORS
  (allowlist, optionally fully open) and security headers. See [Security](#-security).

## 🏗️ Architecture

| Component  | Technology | Description |
|------------|-----------|-------------|
| Backend    | **Go 1.26+** (net/http, pgx, aws-sdk-go-v2) | REST API, single static binary, stateless – runs as several instances |
| Gateway    | **nginx** | Load balancer in front of the API instances, the only published API port |
| Valkey     | **Valkey 9** | Shared rate-limit counters of all API instances (no persistence, restricted ACL user) |
| Frontend   | **React 19 + Vite 8 + Tailwind CSS 4** | SPA served by nginx, runtime config via `/config.js`, two designs (Collector with Zen Maru Gothic, Classic with M PLUS 1, both self-hosted) |
| Database   | **PostgreSQL 18** (pgvector image) | All data; EF-Core schema (PascalCase quoted identifiers) used as-is; also coordinates the instances (advisory locks) |
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
   openssl rand -base64 48   # use the output as JWT_KEY
   openssl rand -hex 24      # use the output as VALKEY_PASSWORD
   ```
   Fill in your values (see [Configuration](#-configuration)). The backend refuses to start without a real
   `JWT_KEY`; docker compose refuses to start without `VALKEY_PASSWORD`.
3. **Launch**
   ```bash
   docker compose up -d --build
   ```
   Services: API gateway (`BACKEND_PORT`, default `:8000`) in front of `BACKEND_REPLICAS` API instances
   (default 2), frontend (`FRONTEND_PORT`, `:8001`), docs (`DOCS_PORT`, `:8002`), PostgreSQL and Valkey
   (internal only). Put a TLS-terminating reverse proxy in front of them and list its address in
   `TRUSTED_PROXIES` if it is not on a private network.

### Local development

```bash
# Database only (Docker)
docker compose up -d db

# Backend (Go 1.26+). Without REDIS_URL every instance keeps its own rate limits – fine for one instance.
cd backend
go run ./cmd/server -env ../.env -addr :8080

# Frontend (Node 22.22+; the Docker images use Node 26)
cd frontend
npm install
npm run dev        # http://localhost:5173, proxies /api to localhost:8080 (override: VITE_DEV_API_PROXY)

# Docs (Node 22.22+)
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
| Database | `POSTGRES_USER/PASSWORD/DB/HOST/PORT`, `POSTGRES_SSLMODE`, `POSTGRES_MAX_CONNS`, `POSTGRES_STATEMENT_TIMEOUT_MS` | PostgreSQL connection |
| URLs | `API_BASE_URL`, `FRONTEND_URL`, `DOCS_URL`, `STATUS_URL` | Public endpoints |
| Discord | `DISCORD_CLIENT_ID/SECRET/REDIRECT_URI/SERVER_URL` | OAuth2 credentials |
| JWT | `JWT_KEY` (≥32 random chars), `JWT_ISSUER`, `JWT_AUDIENCE`, `JWT_LIFETIME_HOURS` | Token signing/validation |
| Storage | `S3_ACCESS_KEY/SECRET_KEY/SERVICE_URL/REGION/BUCKET_NAME/FORCE_PATH_STYLE/VERIFY_SSL` | Cloudflare R2 |
| CDN | `CDN_BASE_URL`, `CLOUDFLARE_ZONE_ID/API_TOKEN` | Image URLs + cache purging |
| Review | `REVIEW_*` | Review requirements & per-type role bypasses (`"None"` = no bypass) |
| Permissions | `PERMISSIONS_*` | Minimum roles for uploads/edits/banner |
| Limits | `IMAGE_*`, `*_DEFAULT_PAGE_SIZE`, `*_MAX_PAGE_SIZE`, `MAX_PAGE_SIZE_LIMIT`, `MAX_FILTER_VALUES`, `*_MAX_PER_USER` | Upload/pagination/quota limits |
| Network | `RATE_LIMIT_*`, `TRUSTED_PROXIES`, `REAL_IP_HEADER`, `HSTS_MAX_AGE`, `CORS_*` | Rate limiting, client IPs behind proxies, CORS |
| Cluster | `BACKEND_REPLICAS`, `GATEWAY_MAX_BODY_SIZE`, `GATEWAY_MAX_CONNECTIONS_PER_CLIENT`, `VALKEY_PASSWORD`, `REDIS_URL`, `REDIS_KEY_PREFIX`, `REDIS_TIMEOUT_MS`, `CLUSTER_LOCK_CONNECTIONS`, `SHUTDOWN_DRAIN_DELAY_SECONDS`, `INSTANCE_ID` | Several API instances (see [Scaling](#-scaling)) |
| Cache | `PUBLIC_CACHE_TTL_SECONDS` | Lifetime of the per-instance caches for public lists (default 30, `0` disables them) |
| Frontend | `VITE_*`, `CSP_CONNECT_SRC`, `CSP_IMG_SRC` | Client config (links, titles, Discord client id) and extra CSP sources |

The backend fails fast at startup when a required setting is missing, a URL is not an absolute http(s) URL,
`JWT_KEY` is too short or still the example placeholder (generate one with `openssl rand -base64 48`) or the
Valkey password is still the example value.

## 📈 Scaling

The API instances are stateless and interchangeable; `docker compose` starts `BACKEND_REPLICAS` of them behind
the nginx gateway (`deploy/gateway/default.conf.template`). It spreads requests over the instances, skips an
instance that fails and re-resolves the instances when you scale (`docker compose up -d --scale backend=4`).

- **Client addresses** are decided once, at the gateway: `TRUSTED_PROXIES` and `REAL_IP_HEADER` from `.env`
  configure it (`deploy/gateway/entrypoint.sh`), it forwards exactly one address in `X-Forwarded-For` and
  `X-Real-IP` and drops `True-Client-IP` and `Forwarded` sent by clients (`CF-Connecting-IP` reaches the API at
  most as a hint for its configuration warning, never as the address); the API instances only trust the gateway. If clients can reach the gateway port directly, list only your edge (e.g. the Cloudflare ranges)
  in `TRUSTED_PROXIES`; if your reverse proxy runs on the same host, publish the port on `127.0.0.1` only.
- **Rate limits** are shared through Valkey (`REDIS_URL`): one atomic script per request, so a client gets the
  same budget no matter which instance answers. If Valkey does not answer within `REDIS_TIMEOUT_MS`, the instance
  limits locally for a few seconds instead of failing requests. Without `REDIS_URL` every instance limits on its
  own.
- **Coordination** runs through PostgreSQL advisory locks on a small dedicated pool per instance
  (`CLUSTER_LOCK_CONNECTIONS`): uploads, file replacements and deferred storage deletes of the same image never
  interleave across instances, and exactly one instance (elected through a lock that is released automatically
  if it dies) runs the daily cleanup jobs. Size PostgreSQL for `instances × (POSTGRES_MAX_CONNS +
  CLUSTER_LOCK_CONNECTIONS)` connections.
- **Statistics** are buffered per instance and flushed as increments every 10 s, so instances never overwrite each
  other's counts.
- **Caches** for public lists (image lists, tag and artist counts) live in each instance for at most
  `PUBLIC_CACHE_TTL_SECONDS`. A write clears them at once on the instance that handled it and is announced
  through Valkey (two small keys below `REDIS_KEY_PREFIX`, read with `GET`/`SET`, which the compose ACL already
  allows), so the other instances follow within about a second; without Valkey they follow within the TTL. Rows
  from a cached list are rechecked against the filter before they are served, so deleted, rejected or changed
  images never come out of a cache. Responses for anonymous requests carry an `ETag` and answer
  `If-None-Match` with `304`.
- **Rolling restarts:** on `SIGTERM` an instance first reports `503` on `/readyz` for
  `SHUTDOWN_DRAIN_DELAY_SECONDS`, then drains in-flight requests; `stop_grace_period` covers all phases.
- **Without docker compose** use any load balancer, point every instance at the same database and Valkey
  (`rediss://` for TLS) and give each a distinct `INSTANCE_ID` (shown in logs and database sessions).

## 🎨 Frontend designs

The frontend ships two builds from one Vite project: `index.html` (*Collector*, the default) and `classic.html`
(*Classic*, the previous design, kept in `frontend/src/classic/`). Both use the same API client, contexts, hooks
and utilities; only components, pages and styles differ, and each page loads only its own CSS and JavaScript.

- The theme menu (sun/moon button) has a **Design** group. Picking a design sets the cookie
  `waifu_design=collector|classic` (one year, `SameSite=Lax`, `Secure` on HTTPS) and reloads the page.
- Both designs use the same colours (zinc neutrals, an accent colour, the “API” and “Waifu” gradients); the
  Collector design adds the binder layout, cards and the holo effects.
- **Accent colours:** the menu offers six presets – Violet (default, the colour of the previous design), Pink,
  Orange, Green, Blue and Mono. A preset sets `html[data-accent]`; both stylesheets define the accent, its text
  shade, the accent button, focus rings, charts and the “Waifu” gradient per preset for light and dark mode (all
  combinations meet WCAG AA). The choice is stored in `localStorage` and applied by `public/init.js` before the
  first paint. New presets need an entry in `src/utils/accent.ts`, `public/init.js` and both `index.css` files.
- The same menu has an **Animations and effects** switch (holo sheen, tilt, card flip, animations, transitions,
  background blur). It is stored in `localStorage` and applied by `public/init.js` before the first paint; without a
  choice it follows `prefers-reduced-motion`. In CSS the state is `html[data-motion="off"]`, and the Tailwind
  variants `motion-safe:`/`motion-reduce:` follow it.
- `frontend/nginx.conf` maps the cookie to the document it serves for every SPA route; the documents are sent with
  `Cache-Control: no-cache, private` and `Vary: Cookie`, so shared caches never mix them up. Do not let a CDN cache
  HTML from the frontend container.
- `npm run dev` and `npm run preview` honour the cookie as well.
- The cookie is described on the privacy page of both designs. It is only set after a visitor picks a design.

## 🔑 Setting up an admin user

1. Log in once on the website with your Discord account (creates the user row).
2. Promote in the database:
   ```sql
   UPDATE public."Users" SET "Role" = 3 WHERE "DiscordId" = '<YOUR_DISCORD_ID>';
   ```
   Roles: `0` User, `1` TrustedUser, `2` Moderator, `3` Admin.
3. Reload the page – the API reads roles (and bans) from the database on every request, so the new role is
   active immediately; no new login is needed.

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

**Authentication & authorization**
- Roles and bans are read from the database on every request: promotions, demotions and bans take effect
  immediately, also for JWTs issued earlier. If the database is unavailable, authenticated requests fail closed
  (`503`) instead of skipping the ban check.
- JWTs are validated with pinned HMAC algorithms, issuer, audience, `exp`/`nbf` and a bounded token size
  (`golang-jwt` v5.3, CVE-2025-30204 fixed). Lifetime: `JWT_LIFETIME_HOURS`.
- API keys are stored as SHA-256 hashes and shown only once. An API key can list and revoke keys, but creating or
  changing keys, changing roles or bans and deleting files through the storage diff require a website session –
  also for administrators – so a leaked key cannot be turned into lasting access. Per-user key limit.
- Discord login: strict, single-use OAuth `state` nonce (valid for 10 minutes in this browser, so a login that
  returns in another tab works), safe same-origin redirects after login, banned accounts are rejected at login.

**Input handling & content**
- Uploads: the real format is detected from the content (JPEG, PNG, GIF, WebP); file extension and MIME type
  are never taken from the client. Metadata is removed losslessly before the file is stored (EXIF including GPS
  and camera data – only the orientation is kept –, XMP, IPTC, comments, PNG text chunks, data after the image);
  the pixels stay identical. Image dimensions and the expected decoder memory are checked from the header
  *before* decoding (decompression-bomb protection, `IMAGE_MAX_PIXELS`, `IMAGE_MAX_DECODE_MB`, at most 32 JPEG
  scans), decoding runs in a bounded worker pool.
- The file is stored outside the database transaction; if the image is removed meanwhile, the upload answers
  `409` and the stored file is deleted again. Failed storage deletes are retried in the background.
- User-supplied links (image sources, artist profiles) must be absolute http(s) URLs – on the server and when
  rendering (`javascript:` links cannot be stored or rendered).
- Visibility rules are enforced everywhere: pending content is only visible to its creator and moderators,
  albums only to their owner (fixes an IDOR on album contents); the duplicate check names an existing image only
  if the caller may see it. Each user can have at most `REVIEW_MAX_PENDING_SUBMISSIONS_PER_USER` new tags,
  artists and uploads waiting for review.
- Review/report state changes are atomic (row locks, advisory locks) – no double approvals, no deletion of
  already accepted content through stale review tasks.
- Internal errors never leak database details; every response carries an `X-Request-Id`.

**Abuse protection**
- Per-IP token-bucket rate limiting with stricter limits for login and uploads (`RATE_LIMIT_*`), shared by all
  API instances through Valkey, IPv6 grouped per /64, bounded memory. `X-RateLimit-Limit` /
  `X-RateLimit-Remaining` and `Retry-After` headers.
- `X-Forwarded-For` / `X-Forwarded-Proto` are only trusted from `TRUSTED_PROXIES` (optionally a single
  `REAL_IP_HEADER` such as `CF-Connecting-IP`), so clients cannot spoof their IP to evade limits.
  **Behind Cloudflare** set `REAL_IP_HEADER=CF-Connecting-IP` and add the Cloudflare ranges (or your reverse
  proxy) to `TRUSTED_PROXIES`; otherwise all visitors share the limit of the edge address (the API logs a hint).
- Hard caps on page sizes (`MAX_PAGE_SIZE_LIMIT`), filter values (`MAX_FILTER_VALUES`), request bodies, header
  size, statement timeouts and per-user quotas (albums, API keys, pending reports and submissions).
- Gateway: request bodies above 1 MB only for image uploads and file replacements (`GATEWAY_MAX_BODY_SIZE`),
  concurrent requests per client (`GATEWAY_MAX_CONNECTIONS_PER_CLIENT`), buffered bodies capped in memory; its own
  `413`/`429` answers use the API's problem-details format.

**Transport & browser**
- API: `nosniff`, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`, restrictive CSP, HSTS for HTTPS
  requests; the Scalar docs page is pinned to a fixed version with Subresource Integrity.
- Frontend (nginx): a Content-Security-Policy generated at container start (`script-src 'self'`, API and CDN
  as the only connect targets), HSTS behind HTTPS proxies, no inline scripts, immutable caching for hashed assets
  (error responses are never cached).
- Frontend routes accept only numeric IDs and the API client refuses paths with `..`, `?`, `#` or `\`, so a
  crafted link cannot redirect an admin's click to another endpoint (client-side path traversal).
- CORS restricted to `FRONTEND_URL` (+ `CORS_EXTRA_ORIGINS`); `CORS_ALLOW_ALL=true` opens it completely.
- Containers: non-root API user, read-only root filesystems for the API, gateway and Valkey,
  `no-new-privileges`, separate networks (PostgreSQL and Valkey only reachable from the API instances, no internet
  access; frontend and docs cannot reach the API containers), current base images (Go 1.27, Alpine 3.24, Node 26,
  nginx stable); npm packages are installed with `--ignore-scripts`.
- CI: third-party actions pinned to commit SHAs (kept current by Dependabot), least-privilege permissions; image
  tags: `latest` and `vX`/`vX.Y` only for releases (`latest` never moves backwards), `main` and `sha-<commit>`
  for pushes to main.

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

Deliberate differences (documented in [`.env.example`](.env.example) and the [changelog](CHANGELOG.md)):

- **Rate limiting** (per-IP token bucket, configurable) – the .NET backend had none.
- **CORS allowlist** (configurable; `CORS_ALLOW_ALL=true` opens it completely) instead
  of `AllowAnyOrigin`.
- **Security limits**: page sizes are capped (`MAX_PAGE_SIZE_LIMIT`, default 500), multi-value filters accept at
  most `MAX_FILTER_VALUES` (default 50) values, API keys cannot create or modify API keys, roles/bans apply
  immediately.
- Additional endpoints: `GET /healthz` (container health check) and static `/openapi/v1.json` + `/scalar`;
  `GET /users/me/permissions` additionally returns the upload limits.

## 🧪 Development

```bash
# Backend: build, vet & test (Go 1.26+). Integration tests need PostgreSQL, the cluster tests Valkey/Redis:
cd backend && go build ./... && go vet ./...
WAIFU_TEST_DATABASE_URL="postgres://postgres@localhost:5432/postgres?sslmode=disable" \
WAIFU_TEST_REDIS_URL="redis://localhost:6379/0" go test -race ./...

# Frontend: type check, lint (oxlint) and production build
cd frontend && npm install && npm run lint && npm run build

# Docs: build
cd docs && npm install && npm run build
```

The backend intentionally carries no checked-in `go.sum`; `go mod tidy` resolves the graph
(also done inside the Docker build).

## 📄 License

This project is licensed under the [MIT License](LICENSE).
