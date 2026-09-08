package api

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const maxStreamsPerUser = 4
const maxStreamsTotal = 128
const streamWriteTimeout = 10 * time.Second
const streamHeartbeatInterval = 20 * time.Second

type streamLimits struct {
	mu    sync.Mutex
	users map[string]int
	total int
}

func (l *streamLimits) acquire(user string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total >= maxStreamsTotal || l.users[user] >= maxStreamsPerUser {
		return false
	}
	if l.users == nil {
		l.users = make(map[string]int)
	}
	l.users[user]++
	l.total++
	return true
}

func (l *streamLimits) release(user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.users[user] == 0 {
		return
	}
	l.users[user]--
	l.total--
	if l.users[user] == 0 {
		delete(l.users, user)
	}
}

func writeStreamEvent(w http.ResponseWriter, event string, payload []byte) error {
	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Now().Add(streamWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	if event == "" {
		if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return err
	}
	return controller.Flush()
}

func (s *Server) streamSessionValid(r *http.Request, userID string) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	session, ok := s.sessions.Get(cookie.Value)
	if !ok || session.UserID != userID {
		return false
	}
	user, ok := s.manager.User(userID)
	return ok && user.Role == "user"
}
