package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/store"
)

type Server struct {
	store          *store.DashboardStore
	refreshFn      func(bool) error
	addRemoteFn    func(model.PileUpsertRequest) (model.Pile, error)
	deleteRemoteFn func(string)
	updateCookieFn func(string) error
}

func NewServer(
	s *store.DashboardStore,
	refreshFn func(bool) error,
	addRemoteFn func(model.PileUpsertRequest) (model.Pile, error),
	deleteRemoteFn func(string),
	updateCookieFn func(string) error,
) *Server {
	return &Server{
		store:          s,
		refreshFn:      refreshFn,
		addRemoteFn:    addRemoteFn,
		deleteRemoteFn: deleteRemoteFn,
		updateCookieFn: updateCookieFn,
	}
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/piles", s.handlePiles)
	mux.HandleFunc("/api/piles/", s.handlePileActions)
	mux.HandleFunc("/api/refresh", s.handleRefresh)
	mux.HandleFunc("/api/session/cookie", s.handleCookieUpdate)
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/healthz", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.Snapshot())
	case http.MethodPost:
		var req model.PileUpsertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		pile, err := s.upsertPile(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, pile)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handlePileActions(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	pileID := parts[2]
	if len(parts) == 3 {
		if r.Method != http.MethodDelete {
			methodNotAllowed(w)
			return
		}
		if !s.store.DeletePile(pileID) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "pile not found"})
			return
		}
		if s.deleteRemoteFn != nil {
			s.deleteRemoteFn(pileID)
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.refreshFn == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "refresh function is not configured"})
		return
	}
	if err := s.refreshFn(false); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s.store.Snapshot())
}

func (s *Server) handleCookieUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.updateCookieFn == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cookie update function is not configured"})
		return
	}

	var req struct {
		Cookie string `json:"cookie"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if strings.TrimSpace(req.Cookie) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cookie is required"})
		return
	}

	if err := s.updateCookieFn(req.Cookie); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, s.store.Snapshot())
}

func (s *Server) upsertPile(req model.PileUpsertRequest) (model.Pile, error) {
	if s.addRemoteFn != nil {
		return s.addRemoteFn(req)
	}
	return s.store.UpsertPile(req)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	ch := s.store.Subscribe()
	defer s.store.Unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case snapshot, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(snapshot)
			if err != nil {
				log.Printf("marshal snapshot: %v", err)
				continue
			}
			if _, err = fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}
