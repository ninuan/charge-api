package auth

import (
	"strings"
	"sync"
	"time"
)

type AuthGuard struct {
	mu sync.Mutex

	requestWindow time.Duration
	requestLimit  int
	blockDuration time.Duration

	failureWindow time.Duration
	failureLimit  int

	requests    map[string]*guardEntry
	failures    map[string]*guardEntry
	lastCleanup time.Time
}

type guardEntry struct {
	Count        int
	WindowStart  time.Time
	BlockedUntil time.Time
}

func NewAuthGuard() *AuthGuard {
	return &AuthGuard{
		requestWindow: 5 * time.Minute,
		requestLimit:  20,
		blockDuration: 15 * time.Minute,
		failureWindow: 15 * time.Minute,
		failureLimit:  5,
		requests:      make(map[string]*guardEntry),
		failures:      make(map[string]*guardEntry),
		lastCleanup:   time.Now(),
	}
}

func (g *AuthGuard) AllowRequest(ip string) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	g.cleanupLocked(now)
	key := normalizeKey(ip)
	entry := g.requests[key]
	if entry == nil {
		entry = &guardEntry{WindowStart: now}
		g.requests[key] = entry
	}
	if now.Before(entry.BlockedUntil) {
		return false, time.Until(entry.BlockedUntil)
	}
	if now.Sub(entry.WindowStart) >= g.requestWindow {
		entry.Count = 0
		entry.WindowStart = now
	}
	entry.Count++
	if entry.Count > g.requestLimit {
		entry.BlockedUntil = now.Add(g.blockDuration)
		return false, g.blockDuration
	}
	return true, 0
}

func (g *AuthGuard) Locked(ip string, username string) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	g.cleanupLocked(now)
	for _, key := range failureKeys(ip, username) {
		entry := g.failures[key]
		if entry != nil && now.Before(entry.BlockedUntil) {
			return true, time.Until(entry.BlockedUntil)
		}
	}
	return false, 0
}

func (g *AuthGuard) RecordFailure(ip string, username string) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	g.cleanupLocked(now)
	var longest time.Duration
	locked := false
	for _, key := range failureKeys(ip, username) {
		entry := g.failures[key]
		if entry == nil {
			entry = &guardEntry{WindowStart: now}
			g.failures[key] = entry
		}
		if now.Sub(entry.WindowStart) >= g.failureWindow {
			entry.Count = 0
			entry.WindowStart = now
			entry.BlockedUntil = time.Time{}
		}
		entry.Count++
		if entry.Count >= g.failureLimit {
			entry.BlockedUntil = now.Add(g.blockDuration)
			locked = true
			if g.blockDuration > longest {
				longest = g.blockDuration
			}
		}
	}
	return locked, longest
}

func (g *AuthGuard) RecordSuccess(ip string, username string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.failures, "ip:"+normalizeKey(ip))
	delete(g.failures, "user:"+normalizeKey(username))
}

func failureKeys(ip string, username string) []string {
	keys := []string{"ip:" + normalizeKey(ip)}
	if strings.TrimSpace(username) != "" {
		keys = append(keys, "user:"+normalizeKey(username))
	}
	return keys
}

func normalizeKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	return value
}

func (g *AuthGuard) cleanupLocked(now time.Time) {
	if now.Sub(g.lastCleanup) < 10*time.Minute {
		return
	}
	for key, entry := range g.requests {
		if now.After(entry.BlockedUntil) && now.Sub(entry.WindowStart) >= g.requestWindow {
			delete(g.requests, key)
		}
	}
	for key, entry := range g.failures {
		if now.After(entry.BlockedUntil) && now.Sub(entry.WindowStart) >= g.failureWindow {
			delete(g.failures, key)
		}
	}
	g.lastCleanup = now
}
