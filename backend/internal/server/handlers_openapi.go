package server

import (
	"net/http"
	"os"
)

func (s *Server) RegisterOpenAPIRoutes() {
	s.Handle("GET /openapi/v1.json", http.HandlerFunc(s.handleOpenAPISpec))
	s.Handle("GET /scalar", http.HandlerFunc(s.handleScalarUI))
	s.Handle("GET /scalar/", http.HandlerFunc(s.handleScalarUI))
}

func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	path := s.cfg.OpenAPISpecPath
	if path == "" {
		path = "openapi.json"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		NotFound(w, r, "OpenAPI specification not found.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) scalarHTML() string {
	specURL := "/openapi/v1.json"
	if s.base != "" {
		specURL = "/" + s.base + specURL
	}
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>API Reference</title>
</head>
<body style="margin:0">
  <script id="api-reference" data-url="` + specURL + `"></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`
}

func (s *Server) handleScalarUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(s.scalarHTML()))
}
