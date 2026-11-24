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
	Token        string
	UserID       string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	LastActiveAt time.Time
	Browser      string
	OS           string
	DeviceType   string
	IPLabel      string
}

type SessionClientInfo struct {
	Browser    string
	OS         string
	DeviceType string
	IPLabel    string
}

type SessionManager struct {
	mu                 sync.RWMutex
	sessions           map[string]Session
	revocationGen      uint64
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

func (m *SessionManager) Create(userID string, clients ...SessionClientInfo) (Session, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return Session{}, fmt.Errorf("generate session token: %w", err)
	}

	now := time.Now()
	client := SessionClientInfo{
		Browser: "未知浏览器", OS: "未知系统",
		DeviceType: "未知设备", IPLabel: "未知网络",
	}
	if len(clients) > 0 {
		client = clients[0]
	}
	session := Session{
		Token:        base64.RawURLEncoding.EncodeToString(tokenBytes),
		UserID:       userID,
		CreatedAt:    now,
		ExpiresAt:    now.Add(m.ttl),
		LastActiveAt: now,
		Browser:      client.Browser,
		OS:           client.OS,
		DeviceType:   client.DeviceType,
		IPLabel:      client.IPLabel,
	}

	if m.store != nil {
		m.mu.RLock()
		gen := m.revocationGen
		m.mu.RUnlock()
		if err := m.store.SaveSession(persistence.SessionRecord{
			TokenHash:    hashSessionToken(session.Token),
			UserID:       session.UserID,
			CreatedAt:    session.CreatedAt,
			ExpiresAt:    session.ExpiresAt,
			LastActiveAt: session.LastActiveAt,
			Browser:      session.Browser,
			OS:           session.OS,
			DeviceType:   session.DeviceType,
			IPLabel:      session.IPLabel,
		}, m.maxSessionsPerUser); err != nil {
			return Session{}, err
		}
		records, listErr := m.store.ListUserSessions(userID)
		m.mu.Lock()
		m.cleanupExpiredLocked(now)
		// Always keep in-memory cache in sync with SQLite eviction for this user,
		// even when an unrelated revocation incremented revocationGen.
		if listErr == nil {
			active := make(map[string]struct{}, len(records))
			for _, record := range records {
				active[string(record.TokenHash)] = struct{}{}
			}
			for token, cached := range m.sessions {
				if cached.UserID != userID {
					continue
				}
				if _, ok := active[string(hashSessionToken(token))]; !ok {
					delete(m.sessions, token)
				}
			}
		} else if m.revocationGen != gen {
			for token, cached := range m.sessions {
				if cached.UserID == userID {
					delete(m.sessions, token)
				}
			}
		}
		// Only cache the new session when no revocation raced with SaveSession;
		// otherwise let subsequent Get() verify the authoritative SQLite row.
		if m.revocationGen == gen {
			m.sessions[session.Token] = session
			m.limitUserSessionsLocked(userID)
		}
		// SaveSession may have evicted an older session when enforcing
		// maxSessionsPerUser; bump revocationGen so a concurrent Get() for that
		// evicted token cannot repopulate m.sessions.
		if m.maxSessionsPerUser > 0 {
			m.revocationGen++
		}
		m.mu.Unlock()
		return session, nil
	}

	m.mu.Lock()
	m.cleanupExpiredLocked(now)
	m.sessions[session.Token] = session
	m.limitUserSessionsLocked(userID)
	m.mu.Unlock()
	return session, nil
}

func (m *SessionManager) Get(token string) (Session, bool) {
	now := time.Now()
	m.mu.RLock()
	session, ok := m.sessions[token]
	gen := m.revocationGen
	m.mu.RUnlock()

	if ok {
		if now.After(session.ExpiresAt) {
			if m.store != nil {
				_ = m.store.DeleteSession(hashSessionToken(token))
			}
			m.mu.Lock()
			delete(m.sessions, token)
			m.revocationGen++
			m.mu.Unlock()
			return Session{}, false
		}
		return session, true
	}

	if m.store == nil {
		return Session{}, false
	}
	tokenHash := hashSessionToken(token)
	record, found, err := m.store.LoadSession(tokenHash)
	if err != nil || !found {
		return Session{}, false
	}
	session = Session{
		Token:        token,
		UserID:       record.UserID,
		CreatedAt:    record.CreatedAt,
		ExpiresAt:    record.ExpiresAt,
		LastActiveAt: record.LastActiveAt,
		Browser:      record.Browser,
		OS:           record.OS,
		DeviceType:   record.DeviceType,
		IPLabel:      record.IPLabel,
	}
	if now.After(session.ExpiresAt) {
		_ = m.store.DeleteSession(tokenHash)
		return Session{}, false
	}
	m.mu.Lock()
	if m.revocationGen == gen {
		m.sessions[token] = session
	}
	m.mu.Unlock()
	return session, true
}

func (m *SessionManager) Touch(token string) {
	now := time.Now()
	m.mu.RLock()
	session, ok := m.sessions[token]
	if ok && now.Sub(session.LastActiveAt) < time.Minute {
		m.mu.RUnlock()
		return
	}
	m.mu.RUnlock()

	m.mu.Lock()
	if cached, exists := m.sessions[token]; exists {
		if now.Sub(cached.LastActiveAt) < time.Minute {
			m.mu.Unlock()
			return
		}
		cached.LastActiveAt = now
		m.sessions[token] = cached
	}
	m.mu.Unlock()
	if m.store != nil {
		_ = m.store.TouchSession(hashSessionToken(token), now)
	}
}

func (m *SessionManager) Delete(token string) {
	if m.store != nil {
		_ = m.store.DeleteSession(hashSessionToken(token))
	}
	m.mu.Lock()
	delete(m.sessions, token)
	m.revocationGen++
	m.mu.Unlock()
}

func (m *SessionManager) DeleteUser(userID string) error {
	if m.store != nil {
		if err := m.store.DeleteUserSessions(userID); err != nil {
			return err
		}
	}
	m.mu.Lock()
	for token, session := range m.sessions {
		if session.UserID == userID {
			delete(m.sessions, token)
		}
	}
	m.revocationGen++
	m.mu.Unlock()
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
				LastActiveAt: record.LastActiveAt,
				Browser:      record.Browser,
				OS:           record.OS,
				DeviceType:   record.DeviceType,
				IPLabel:      record.IPLabel,
				Current:      bytes.Equal(record.TokenHash, currentHash),
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
		result = append(result, model.SessionView{
			ID:        base64.RawURLEncoding.EncodeToString(hash[:8]),
			CreatedAt: session.CreatedAt, ExpiresAt: session.ExpiresAt,
			LastActiveAt: session.LastActiveAt, Browser: session.Browser,
			OS: session.OS, DeviceType: session.DeviceType, IPLabel: session.IPLabel,
			Current: token == currentToken,
		})
	}
	return result, nil
}

func (m *SessionManager) DeleteOthers(userID, currentToken string) error {
	if m.store != nil {
		if err := m.store.DeleteOtherSessions(userID, hashSessionToken(currentToken)); err != nil {
			return err
		}
	}
	m.mu.Lock()
	for token, session := range m.sessions {
		if session.UserID == userID && token != currentToken {
			delete(m.sessions, token)
		}
	}
	m.revocationGen++
	m.mu.Unlock()
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
