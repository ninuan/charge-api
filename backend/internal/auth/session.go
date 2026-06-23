package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sort"
	"sync"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/persistence"
)

const defaultMaxSessionsPerUser = 5

type Session struct {
	Token     string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type SessionManager struct {
	mu                 sync.RWMutex
	sessions           map[string]Session
	ttl                time.Duration
	maxSessionsPerUser int
	stopCleanup        chan struct{}
	cleanupDone        chan struct{}
	closeOnce          sync.Once
	store              *persistence.Store
}

func NewSessionManager(ttl time.Duration) *SessionManager {
	return newSessionManager(ttl, nil)
}

func NewPersistentSessionManager(ttl time.Duration, store *persistence.Store) *SessionManager {
	return newSessionManager(ttl, store)
}

func newSessionManager(ttl time.Duration, store *persistence.Store) *SessionManager {
	manager := &SessionManager{
		sessions:           make(map[string]Session),
		ttl:                ttl,
		maxSessionsPerUser: defaultMaxSessionsPerUser,
		stopCleanup:        make(chan struct{}),
		cleanupDone:        make(chan struct{}),
		store:              store,
	}
	go manager.cleanupLoop()
	return manager
}

func (m *SessionManager) Close() {
	m.closeOnce.Do(func() {
		close(m.stopCleanup)
		<-m.cleanupDone
	})
}

func (m *SessionManager) Create(userID string) (Session, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return Session{}, fmt.Errorf("generate session token: %w", err)
	}

	now := time.Now()
	session := Session{
		Token:     base64.RawURLEncoding.EncodeToString(tokenBytes),
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(m.ttl),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked(now)
	m.sessions[session.Token] = session
	m.limitUserSessionsLocked(userID)
	if m.store != nil {
		if err := m.store.SaveSession(persistence.SessionRecord{
			TokenHash: hashSessionToken(session.Token),
			UserID:    session.UserID,
			CreatedAt: session.CreatedAt,
			ExpiresAt: session.ExpiresAt,
		}, m.maxSessionsPerUser); err != nil {
			delete(m.sessions, session.Token)
			return Session{}, err
		}
	}
	return session, nil
}

func (m *SessionManager) Get(token string) (Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[token]
	if !ok {
		if m.store == nil {
			return Session{}, false
		}
		record, found, err := m.store.LoadSession(hashSessionToken(token))
		if err != nil || !found {
			return Session{}, false
		}
		session = Session{
			Token:     token,
			UserID:    record.UserID,
			CreatedAt: record.CreatedAt,
			ExpiresAt: record.ExpiresAt,
		}
		m.sessions[token] = session
	}

	if time.Now().After(session.ExpiresAt) {
		delete(m.sessions, token)
		if m.store != nil {
			_ = m.store.DeleteSession(hashSessionToken(token))
		}
		return Session{}, false
	}
	return session, true
}

func (m *SessionManager) Delete(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
	if m.store != nil {
		_ = m.store.DeleteSession(hashSessionToken(token))
	}
}

func (m *SessionManager) DeleteUser(userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for token, session := range m.sessions {
		if session.UserID == userID {
			delete(m.sessions, token)
		}
	}
	if m.store != nil {
		if err := m.store.DeleteUserSessions(userID); err != nil {
			return err
		}
	}
	return nil
}

func (m *SessionManager) List(userID, currentToken string) ([]model.SessionView, error) {
	currentHash := hashSessionToken(currentToken)
	if m.store != nil {
		records, err := m.store.ListUserSessions(userID)
		if err != nil {
			return nil, err
		}
		result := make([]model.SessionView, 0, len(records))
		for _, record := range records {
			result = append(result, model.SessionView{
				ID:        base64.RawURLEncoding.EncodeToString(record.TokenHash[:8]),
				CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt,
				Current: bytes.Equal(record.TokenHash, currentHash),
			})
		}
		return result, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []model.SessionView
	for token, session := range m.sessions {
		if session.UserID != userID {
			continue
		}
		hash := hashSessionToken(token)
		result = append(result, model.SessionView{ID: base64.RawURLEncoding.EncodeToString(hash[:8]), CreatedAt: session.CreatedAt, ExpiresAt: session.ExpiresAt, Current: token == currentToken})
	}
	return result, nil
}

func (m *SessionManager) DeleteOthers(userID, currentToken string) error {
	m.mu.Lock()
	for token, session := range m.sessions {
		if session.UserID == userID && token != currentToken {
			delete(m.sessions, token)
		}
	}
	m.mu.Unlock()
	if m.store != nil {
		return m.store.DeleteOtherSessions(userID, hashSessionToken(currentToken))
	}
	return nil
}

func (m *SessionManager) cleanupLoop() {
	interval := m.ttl / 2
	if interval < time.Minute {
		interval = time.Minute
	}
	if interval > time.Hour {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer func() {
		ticker.Stop()
		close(m.cleanupDone)
	}()

	for {
		select {
		case <-ticker.C:
			m.mu.Lock()
			m.cleanupExpiredLocked(time.Now())
			m.mu.Unlock()
			if m.store != nil {
				_ = m.store.DeleteExpiredSessions(time.Now())
			}
		case <-m.stopCleanup:
			return
		}
	}
}

func hashSessionToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

func (m *SessionManager) cleanupExpiredLocked(now time.Time) {
	for token, session := range m.sessions {
		if !now.Before(session.ExpiresAt) {
			delete(m.sessions, token)
		}
	}
}

func (m *SessionManager) limitUserSessionsLocked(userID string) {
	sessions := make([]Session, 0)
	for _, session := range m.sessions {
		if session.UserID == userID {
			sessions = append(sessions, session)
		}
	}
	if len(sessions) <= m.maxSessionsPerUser {
		return
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt.Before(sessions[j].CreatedAt)
	})
	for _, session := range sessions[:len(sessions)-m.maxSessionsPerUser] {
		delete(m.sessions, session.Token)
	}
}
