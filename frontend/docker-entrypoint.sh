#!/bin/sh
set -eu

HTML_DIR=/usr/share/nginx/html
SNIPPET=/etc/nginx/snippets/security-headers.conf

clean() {
  printf '%s' "$1" | tr -d '"\\<>`\r\n\t' | sed 's/^ *//; s/ *$//'
}

js() {
  printf '"%s"' "$(clean "$1")"
}

origin_of() {
  printf '%s\n' "$1" | head -n 1 | tr -d '\r\t' | tr 'A-Z' 'a-z' | sed 's/^ *//; s/ *$//' |
    sed -nE 's|^(https?://[][a-z0-9._:-]+)([/?#].*)?$|\1|p'
}

csp_sources() {
  printf '%s' "$1" | tr -cd 'A-Za-z0-9.:/*_ -'
}

API_ORIGIN=$(origin_of "${VITE_API_URL:-}")
CDN_ORIGIN=$(origin_of "${CDN_BASE_URL:-}")

cat > "$HTML_DIR/config.js" <<CONFIG
window.env = {
  VITE_API_URL: $(js "${VITE_API_URL:-}"),
  VITE_DISCORD_CLIENT_ID: $(js "${VITE_DISCORD_CLIENT_ID:-}"),
  VITE_DISCORD_REDIRECT_URI: $(js "${VITE_DISCORD_REDIRECT_URI:-}"),
  VITE_APP_TITLE: $(js "${VITE_APP_TITLE:-Waifu API}"),
  VITE_DOCS_URL: $(js "${VITE_DOCS_URL:-}"),
  VITE_CONTACT_EMAIL: $(js "${VITE_CONTACT_EMAIL:-}"),
  VITE_DISCORD_SERVER_URL: $(js "${VITE_DISCORD_SERVER_URL:-}"),
  VITE_STATUS_URL: $(js "${VITE_STATUS_URL:-}"),
  VITE_GITHUB_URL: $(js "${VITE_GITHUB_URL:-}"),
  VITE_TOS_URL: $(js "${VITE_TOS_URL:-}"),
  VITE_IMG_TRANSFORM_ENABLED: $(js "${VITE_IMG_TRANSFORM_ENABLED:-false}"),
  VITE_IMG_TRANSFORM_PATH: $(js "${VITE_IMG_TRANSFORM_PATH:-}"),
  VITE_IMG_WIDTH_PARAM: $(js "${VITE_IMG_WIDTH_PARAM:-width}"),
  VITE_IMG_CARD_WIDTH: $(js "${VITE_IMG_CARD_WIDTH:-640}"),
  VITE_IMG_PREVIEW_WIDTH: $(js "${VITE_IMG_PREVIEW_WIDTH:-1280}"),
  VITE_APP_VERSION: $(js "${VITE_APP_VERSION:-}"),
  VITE_CDN_ORIGIN: $(js "$CDN_ORIGIN")
};
CONFIG

APP_TITLE=$(js "${VITE_APP_TITLE:-Waifu API}")
cat > "$HTML_DIR/site.webmanifest" <<MANIFEST
{
  "name": $APP_TITLE,
  "short_name": $APP_TITLE,
  "description": "The API for your Waifu content",
  "icons": [
    {
      "src": "/android-chrome-192x192.png",
      "sizes": "192x192",
      "type": "image/png"
    },
    {
      "src": "/android-chrome-512x512.png",
      "sizes": "512x512",
      "type": "image/png"
    }
  ],
  "theme_color": "#f4f4f5",
  "background_color": "#f4f4f5",
  "display": "standalone",
  "start_url": "/"
}
MANIFEST

EXTRA_CONNECT=$(csp_sources "${CSP_CONNECT_SRC:-}")
EXTRA_IMG=$(csp_sources "${CSP_IMG_SRC:-}")

CSP="default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https: ${API_ORIGIN} ${CDN_ORIGIN} ${EXTRA_IMG}; media-src 'self' blob: https: ${CDN_ORIGIN} ${EXTRA_IMG}; font-src 'self' data:; connect-src 'self' ${API_ORIGIN} ${CDN_ORIGIN} ${EXTRA_CONNECT}; manifest-src 'self'; worker-src 'self' blob:; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
CSP=$(printf '%s' "$CSP" | tr -s ' ')

mkdir -p "$(dirname "$SNIPPET")"
cat > "$SNIPPET" <<HEADERS
add_header Content-Security-Policy "$CSP" always;
add_header X-Content-Type-Options "nosniff" always;
add_header X-Frame-Options "DENY" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
add_header Permissions-Policy "camera=(), microphone=(), geolocation=(), payment=(), usb=()" always;
add_header Cross-Origin-Opener-Policy "same-origin" always;
add_header Strict-Transport-Security \$hsts_header always;
HEADERS

exec "$@"
