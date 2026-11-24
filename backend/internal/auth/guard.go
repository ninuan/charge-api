package auth

import (
	"strings"
	"sync"
	"time"
)

const maxAuthGuardEntries = 10_000

type AuthGuard struct {
	mu sync.Mutex

	requestWindow time.Duration
	requestLimit  int
	blockDuration time.Duration

	failureWindow time.Duration
	failureLimit  int
	captchaAfter  int

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
		captchaAfter:  2,
		requests:      make(map[string]*guardEntry),
		failures:      make(map[string]*guardEntry),
		lastCleanup:   time.Now(),
	}
}

// RequiresCaptcha reports whether recent failures for either the source IP or
// the submitted username have reached the adaptive challenge threshold.
func (g *AuthGuard) RequiresCaptcha(ip string, username string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	g.cleanupLocked(now, false)
	for _, key := range captchaKeys(ip, username) {
		entry := g.failures[key]
		if entry != nil && now.Sub(entry.WindowStart) < g.failureWindow && entry.Count >= g.captchaAfter {
			return true
		}
	}
	return false
}

func (g *AuthGuard) AllowRequest(ip string) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	g.cleanupLocked(now, false)
	key := normalizeKey(ip)
	entry := g.requests[key]
	if entry == nil {
		g.ensureCapacityLocked(g.requests, now, g.requestWindow)
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
	g.cleanupLocked(now, false)
	for _, key := range lockKeys(ip, username) {
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
	g.cleanupLocked(now, false)
	var longest time.Duration
	locked := false
	for _, key := range lockKeys(ip, username) {
		entry := g.bumpFailureEntryLocked(key, now)
		if entry.Count >= g.failureLimit {
			entry.BlockedUntil = now.Add(g.blockDuration)
			locked = true
			if g.blockDuration > longest {
				longest = g.blockDuration
			}
		}
	}
	// Track username-wide failures for adaptive CAPTCHA without enabling a
	// cross-IP global account lockout DoS (especially against admin).
	if strings.TrimSpace(username) != "" {
		g.bumpFailureEntryLocked("user:"+normalizeKey(username), now)
	}
	return locked, longest
}

func (g *AuthGuard) bumpFailureEntryLocked(key string, now time.Time) *guardEntry {
	entry := g.failures[key]
	if entry == nil {
		g.ensureCapacityLocked(g.failures, now, g.failureWindow)
		entry = &guardEntry{WindowStart: now}
		g.failures[key] = entry
	}
	if now.Sub(entry.WindowStart) >= g.failureWindow {
		entry.Count = 0
		entry.WindowStart = now
		entry.BlockedUntil = time.Time{}
	}
	entry.Count++
	return entry
}

func (g *AuthGuard) RecordSuccess(ip string, username string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	ipKey := "ip:" + normalizeKey(ip)
	userDelta := 0
	if strings.TrimSpace(username) != "" {
		normUser := normalizeKey(username)
		ipUserKey := "ipuser:" + normalizeKey(ip) + ":" + normUser
		if pairEntry := g.failures[ipUserKey]; pairEntry != nil && now.Sub(pairEntry.WindowStart) < g.failureWindow {
			userDelta = pairEntry.Count
		}
		delete(g.failures, ipUserKey)
		delete(g.failures, "user:"+normUser)
	}
	if userDelta > 0 {
		if ipEntry := g.failures[ipKey]; ipEntry != nil {
			ipEntry.Count -= userDelta
			if ipEntry.Count <= 0 {
				delete(g.failures, ipKey)
			}
		}
	}
}

func lockKeys(ip string, username string) []string {
	normIP := normalizeKey(ip)
	keys := []string{"ip:" + normIP}
	if strings.TrimSpace(username) != "" {
		keys = append(keys, "ipuser:"+normIP+":"+normalizeKey(username))
	}
	return keys
}

func captchaKeys(ip string, username string) []string {
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

func (g *AuthGuard) ensureCapacityLocked(target map[string]*guardEntry, now time.Time, window time.Duration) {
	if len(target) < maxAuthGuardEntries {
		return
	}
	g.cleanupLocked(now, true)
	if len(target) < maxAuthGuardEntries {
		return
	}
	var oldestKey string
	var oldestTime time.Time
	for key, entry := range target {
		if oldestKey == "" || entry.WindowStart.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.WindowStart
		}
	}
	if oldestKey != "" {
		delete(target, oldestKey)
	}
}

func (g *AuthGuard) cleanupLocked(now time.Time, force bool) {
	if !force && now.Sub(g.lastCleanup) < 10*time.Minute {
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
