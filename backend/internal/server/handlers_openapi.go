package server

import (
	"html"
	"net/http"
	"os"
	"sync"
	"time"
)

func (s *Server) RegisterOpenAPIRoutes() {
	s.Handle("GET /openapi/v1.json", http.HandlerFunc(s.handleOpenAPISpec))
	s.Handle("GET /scalar", http.HandlerFunc(s.handleScalarUI))
	s.Handle("GET /scalar/", http.HandlerFunc(s.handleScalarUI))
}

const (
	scalarScriptURL = "https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.73.0/dist/browser/standalone.js"
	scalarScriptSRI = "sha384-OKyMdsDX84ypSZEhVun8YElXk5c2GQaH3EXPOc6ItmVcLDUAvKHYwvDLvAgsqVtB"
	scalarCSP       = "default-src 'none'; script-src https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; worker-src 'self' blob:; " +
		"base-uri 'none'; form-action 'none'"
)

type openAPICache struct {
	mu      sync.Mutex
	data    []byte
	modTime time.Time
	size    int64
}

func (c *openAPICache) load(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data != nil && st.ModTime().Equal(c.modTime) && st.Size() == c.size {
		return c.data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c.data, c.modTime, c.size = data, st.ModTime(), st.Size()
	return data, nil
}

func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	path := s.cfg.OpenAPISpecPath
	if path == "" {
		path = "openapi.json"
	}
	data, err := s.openapi.load(path)
	if err != nil {
		NotFound(w, r, "OpenAPI specification not found.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
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
  <meta name="referrer" content="no-referrer" />
  <title>API Reference</title>
</head>
<body style="margin:0">
  <script id="api-reference" type="application/json" data-url="` + html.EscapeString(specURL) + `" data-configuration="{&quot;withDefaultFonts&quot;:false}"></script>
  <script src="` + scalarScriptURL + `" integrity="` + scalarScriptSRI + `" crossorigin="anonymous" referrerpolicy="no-referrer"></script>
</body>
</html>`
}

func (s *Server) handleScalarUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(s.scalarHTML()))
}
