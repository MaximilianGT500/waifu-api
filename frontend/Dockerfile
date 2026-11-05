# Build stage
FROM node:26-alpine AS build
WORKDIR /app

# Copy package files. Install scripts stay off: no dependency needs one, and they could fetch
# packages outside the lockfile.
COPY package*.json ./
RUN npm ci --ignore-scripts --no-audit --no-fund

# Copy source code
COPY . .

# Build the application
RUN npm run build

# Runtime stage
FROM nginx:stable-alpine
COPY --from=build /app/dist /usr/share/nginx/html

# Copy Nginx configuration
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Copy the entrypoint script (generates config.js, the manifest and the security headers at startup)
COPY docker-entrypoint.sh /docker-entrypoint.sh

# Fix line endings (CRLF -> LF) for files that might have Windows line endings
RUN sed -i 's/\r$//' /docker-entrypoint.sh && \
    sed -i 's/\r$//' /etc/nginx/conf.d/default.conf && \
    chmod +x /docker-entrypoint.sh && \
    mkdir -p /etc/nginx/snippets

EXPOSE 80
ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
