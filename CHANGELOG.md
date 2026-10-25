# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

For older changes see the version history in git (`git log`, e.g. the
`Update 8.0.0` commit containing the Go backend rewrite).

## [Unreleased]

### Added

- **CORS open switch**: `CORS_ALLOW_ALL=true` in the `.env` opens CORS
  completely (any origin may call the API). The allowlist
  (`FRONTEND_URL` + `CORS_EXTRA_ORIGINS`) remains the default.
- **Version display**: the running version is shown at the bottom of the
  sidebar. Set `VERSION=…` in the `.env` (passed through docker-compose as
  `VITE_APP_VERSION`); no rebuild needed.
- **Gallery page-size selector**: the gallery pagination can now change the
  page size (1–100, default 30; changing it jumps back to page 1).

### Changed

- Pagination: the "Pagination disabled" state is gone – pagination is now
  always active, including for random ordering.
