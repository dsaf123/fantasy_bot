// Package portal serves the web config portal: a small, login-gated
// dashboard for editing the settings.Settings overrides that take priority
// over .env at runtime (which scheduled messages are enabled, the waiver
// report's days, the timezone, and the AI weekly recap's prompt). It's
// stdlib-only (net/http, html/template, embed, crypto) to match the rest
// of this codebase's dependency-light style.
package portal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"fantasy_bot/internal/config"
	"fantasy_bot/internal/settings"
)

// Server holds the portal's dependencies: the base (.env-derived) config,
// the settings manager it reads/writes, and in-memory login sessions.
type Server struct {
	cfg      *config.Config
	mgr      *settings.Manager
	sessions *sessionStore
}

func newServer(cfg *config.Config, mgr *settings.Manager) *Server {
	return &Server{cfg: cfg, mgr: mgr, sessions: newSessionStore()}
}

// mux builds the portal's routes. Split out from Serve so tests can drive
// it directly (via httptest) without binding a real network port.
func (s *Server) mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", s.handleLoginForm)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("POST /logout", s.requireAuth(s.handleLogout))
	mux.HandleFunc("GET /{$}", s.requireAuth(s.handleDashboard))
	mux.HandleFunc("POST /settings", s.requireAuth(s.handleSaveSettings))
	return mux
}

// Serve starts the portal on cfg.PortalPort and blocks until ctx is done,
// then shuts the HTTP server down gracefully. Callers should run it in its
// own goroutine: a portal failure (e.g. the configured port is already in
// use) is not fatal to the bot's core scheduling function, so this returns
// its error for the caller to log rather than crash the process.
func Serve(ctx context.Context, cfg *config.Config, mgr *settings.Manager) error {
	if cfg.PortalPassword == "" {
		return errors.New("portal: PORTAL_PASSWORD is not set, refusing to start an unauthenticated portal")
	}

	s := newServer(cfg, mgr)

	httpServer := &http.Server{
		Addr:              ":" + cfg.PortalPort,
		Handler:           s.mux(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("portal: starting on :%s", cfg.PortalPort)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("portal: %w", err)
	}
}

// requireAuth redirects to the login page unless the request carries a
// valid, unexpired session cookie, and otherwise passes that session's
// info through to next (handlers use it for the CSRF token they must
// render into every form).
func (s *Server) requireAuth(next func(w http.ResponseWriter, r *http.Request, sess sessionInfo)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		sess, ok := s.sessions.lookup(cookie.Value)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r, sess)
	}
}
