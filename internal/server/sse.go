package server

import (
	"net/http"
	"time"

	"github.com/gr1m0h/mdv/internal/watch"
)

const ssePingInterval = 25 * time.Second

// handleEvents streams file-change notifications as Server-Sent Events. The
// watcher is released when the client disconnects (spec §5.2, §7).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("path")
	if q == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	abs, ok := s.safeResolve(q)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	watcher, err := watch.New(abs, s.watchMode)
	if err != nil {
		http.Error(w, "watch error", http.StatusInternalServerError)
		return
	}
	defer watcher.Close()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte(": connected\n\n")); err != nil {
		return
	}
	flusher.Flush()

	ping := time.NewTicker(ssePingInterval)
	defer ping.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case _, ok := <-watcher.Events():
			if !ok {
				return
			}
			if _, err := w.Write([]byte("data: reload\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
