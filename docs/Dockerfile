# Stage 1: Build documentation
FROM node:26-alpine AS build
WORKDIR /app

ARG DOCS_URL
ARG DOCS_BASE_URL
ARG DOCS_ORG_NAME
ARG DOCS_PROJECT_NAME
ARG API_BASE_URL
ARG APP_NAME
ARG APP_TAGLINE
ARG FRONTEND_URL
ARG GITHUB_URL

ENV DOCS_URL=${DOCS_URL}
ENV DOCS_BASE_URL=${DOCS_BASE_URL}
ENV DOCS_ORG_NAME=${DOCS_ORG_NAME}
ENV DOCS_PROJECT_NAME=${DOCS_PROJECT_NAME}
ENV API_BASE_URL=${API_BASE_URL}
ENV APP_NAME=${APP_NAME}
ENV APP_TAGLINE=${APP_TAGLINE}
ENV FRONTEND_URL=${FRONTEND_URL}
ENV GITHUB_URL=${GITHUB_URL}

# Copy package files. Install scripts stay off: postman-code-generators would otherwise install
# packages for its code generators outside the lockfile (no integrity check, overrides ignored).
COPY docs/package*.json ./

RUN npm ci --ignore-scripts --no-audit --no-fund

# Copy docs source
COPY docs/ .

# The OpenAPI spec is checked in (served as-is by the Go backend at
# /openapi/v1.json); the docs build consumes it directly.
COPY openapi/WaifuApi.Web.json ../openapi/

# Generate API docs from OpenAPI spec
RUN npx docusaurus gen-api-docs all

# Build static site
RUN npm run build

# Stage 2: Serve with nginx
FROM nginx:stable-alpine AS final

COPY --from=build /app/build /usr/share/nginx/html

# Custom nginx config for SPA routing (+ basic hardening headers)
RUN printf '%s\n' \
    'server_tokens off;' \
    'server {' \
    '    listen 80;' \
    '    root /usr/share/nginx/html;' \
    '    index index.html;' \
    '    gzip on;' \
    '    gzip_types text/css application/javascript application/json image/svg+xml;' \
    '    add_header X-Content-Type-Options "nosniff" always;' \
    '    add_header X-Frame-Options "SAMEORIGIN" always;' \
    '    add_header Referrer-Policy "strict-origin-when-cross-origin" always;' \
    '    add_header Permissions-Policy "camera=(), microphone=(), geolocation=()" always;' \
    '    location / {' \
    '        try_files $uri $uri/ /index.html;' \
    '    }' \
    '}' > /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
