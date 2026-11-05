# syntax=docker/dockerfile:1
FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod ./
COPY . .
# go.sum is resolved (and verified against sum.golang.org) at build time.
RUN go mod tidy && \
    go mod verify && \
    CGO_ENABLED=0 GOOS=linux go build -mod=mod -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S -G app -H -s /sbin/nologin app
WORKDIR /app
COPY --from=build /out/server /app/server
USER app:app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
