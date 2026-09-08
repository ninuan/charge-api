package runtime

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"charge-dashboard/internal/auth"
	"charge-dashboard/internal/model"
	"charge-dashboard/internal/security"
)

// ConfigureInitialPasswordFile is called on every startup, including restarts
// between initialization and the first password change.
func (m *Manager) ConfigureInitialPasswordFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 256 {
		return fmt.Errorf("initial password file must be a small regular file")
	}
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return fmt.Errorf("initial password file changed while opening")
	}
	body, err := security.ReadLimited(file, 256)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, user := range m.users {
		if user.Username != "admin" || user.Role != model.RoleAdmin {
			continue
		}
		if valid, _ := auth.VerifyPassword(strings.TrimSpace(string(body)), user.PasswordHash); valid {
			m.initialPasswordFile = path
			m.initialPasswordUserID = user.ID
			return nil
		}
	}
	// The password was already changed (or the initial account was removed).
	return os.Remove(path)
}

func (m *Manager) cleanupInitialPasswordFile(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.initialPasswordFile == "" || m.initialPasswordUserID != userID {
		return
	}
	if err := os.Remove(m.initialPasswordFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Password persistence succeeded; report cleanup separately, and retry
		// on the next startup rather than presenting a failed password change.
		log.Printf("initial password file cleanup failed: %s", security.SanitizeLogText(err.Error(), 512))
		return
	}
	m.initialPass = ""
	m.initialPasswordFile = ""
	m.initialPasswordUserID = ""
}
