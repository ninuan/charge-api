package api

import (
	"sync"
	"time"
)

// Only sessions created by this Charge user may be proxied using the sidecar key.
// Short-lived results allow recovery after a lost confirm response without
// confusing an older account binding with this QR's completed transaction.
type ownedQRSession struct {
	mu         sync.Mutex
	userID     string
	expires    time.Time
	confirming bool
	result     map[string]any // immutable after publication
}

func (s *Server) rememberQR(id, userID string) bool {
	s.qrMu.Lock()
	defer s.qrMu.Unlock()
	if s.qrSessions == nil {
		s.qrSessions = make(map[string]*ownedQRSession)
	}
	now := time.Now()
	for key, entry := range s.qrSessions {
		if now.After(entry.expires) {
			delete(s.qrSessions, key)
		}
	}
	// Never transfer an existing sidecar session between users, even if upstream
	// accidentally returns a duplicate ID. Bound memory without evicting live work.
	if _, exists := s.qrSessions[id]; exists || len(s.qrSessions) >= 4096 {
		return false
	}
	s.qrSessions[id] = &ownedQRSession{userID: userID, expires: now.Add(30 * time.Minute)}
	return true
}

func (s *Server) findQR(id, userID string) *ownedQRSession {
	s.qrMu.Lock()
	defer s.qrMu.Unlock()
	entry := s.qrSessions[id]
	if entry == nil || entry.userID != userID || time.Now().After(entry.expires) {
		return nil
	}
	return entry
}

// Mutations are exclusive per user, across distinct QR sessions as well as
// explicit sync/unbind. Do not hold the registry mutex across network calls.
func (s *Server) beginBindingChange(userID string) bool {
	s.qrMu.Lock()
	defer s.qrMu.Unlock()
	if s.bindingBusy[userID] {
		return false
	}
	if s.bindingBusy == nil {
		s.bindingBusy = make(map[string]bool)
	}
	s.bindingBusy[userID] = true
	return true
}

func (s *Server) endBindingChange(userID string) {
	s.qrMu.Lock()
	defer s.qrMu.Unlock()
	delete(s.bindingBusy, userID)
}

func (s *Server) invalidateUserQRs(userID, except string) {
	s.qrMu.Lock()
	defer s.qrMu.Unlock()
	for id, session := range s.qrSessions {
		if session.userID == userID && id != except {
			delete(s.qrSessions, id)
		}
	}
}
