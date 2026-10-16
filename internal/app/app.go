package app

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/pltanton/kena/internal/config"
	"github.com/pltanton/kena/internal/mcpserver"
	"github.com/pltanton/kena/internal/oauth"
	"github.com/pltanton/kena/internal/store"
	"github.com/pltanton/kena/internal/web"
)

const maxBodyBytes = 2 << 20

func New(cfg config.Config, st *store.Store) http.Handler {
	userAgent := "kena"
	if cfg.Version != "" {
		userAgent = "kena/" + cfg.Version
	}
	oauthSrv := &oauth.Server{Store: st, PublicURL: cfg.PublicURL, UserAgent: userAgent}

	mux := http.NewServeMux()
	oauthSrv.Register(mux)

	mcpAuth := auth.RequireBearerToken(oauthSrv.Verify, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: oauthSrv.ResourceMetadataURL(),
	})
	mux.Handle("/mcp", mcpAuth(mcpserver.Handler(mcpserver.New(st, cfg.PublicURL))))
	mux.Handle("/mcp/", http.RedirectHandler("/mcp", http.StatusPermanentRedirect))

	mux.Handle("/", web.Handler(st, cfg.PublicURL))

	return http.MaxBytesHandler(withLogging(mux), maxBodyBytes)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start),
		)
	})
}
