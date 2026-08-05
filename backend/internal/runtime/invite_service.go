package runtime

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"charge-dashboard/internal/model"
)

func (m *Manager) Settings() model.RegistrationSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

func normalizeRegistrationSettings(settings model.RegistrationSettings) model.RegistrationSettings {
	if settings.StatsRetentionDays == 0 {
		settings.StatsRetentionDays = defaultStatsRetentionDays
	}
	if settings.PortHistoryRetentionDays == 0 {
		settings.PortHistoryRetentionDays = defaultHistoryRetentionDays
	}
	return settings
}

func (m *Manager) UpdateSettings(settings model.RegistrationSettings) error {
	if settings.DefaultDeviceLimit < 1 || settings.DefaultDeviceLimit > 100 {
		return fmt.Errorf("默认设备额度需要在 1 到 100 之间")
	}
	if settings.StatsRetentionDays < 1 || settings.StatsRetentionDays > 365 {
		return fmt.Errorf("统计保留天数需要在 1 到 365 之间")
	}
	if settings.PortHistoryRetentionDays < 1 || settings.PortHistoryRetentionDays > 365 {
		return fmt.Errorf("端口历史保留天数需要在 1 到 365 之间")
	}
	m.mu.Lock()
	previous := m.settings
	m.settings = settings
	m.mu.Unlock()
	if err := m.Save(); err != nil {
		m.mu.Lock()
		m.settings = previous
		m.mu.Unlock()
		return err
	}
	if _, _, err := m.runRetentionMaintenance(time.Now()); err != nil {
		return err
	}
	return nil
}

func (m *Manager) runRetentionMaintenance(now time.Time) (int64, int64, error) {
	settings := normalizeRegistrationSettings(m.Settings())
	metricRows, err := m.repository.PruneMetrics(
		now.UTC().AddDate(0, 0, -settings.StatsRetentionDays),
	)
	if err != nil {
		return 0, 0, fmt.Errorf("prune metrics: %w", err)
	}
	historyRows, err := m.repository.PrunePortStatusEvents(
		now.UTC().AddDate(0, 0, -settings.PortHistoryRetentionDays),
	)
	if err != nil {
		return metricRows, 0, fmt.Errorf("prune port history: %w", err)
	}
	return metricRows, historyRows, nil
}

func (m *Manager) InviteCodes() []model.InviteCode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.InviteCode, 0, len(m.invites))
	for _, invite := range m.invites {
		result = append(result, invite)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result
}

func (m *Manager) ListInviteCodesPage(page, pageSize int) model.InviteCodePage {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	items := m.InviteCodes()
	total := len(items)
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	pageItems := items[start:end]
	if pageItems == nil {
		pageItems = []model.InviteCode{}
	}
	return model.InviteCodePage{
		Items:      pageItems,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}

func (m *Manager) CreateInvite(code string, expiresAt *time.Time) (model.InviteCode, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		var err error
		code, err = randomInviteCode()
		if err != nil {
			return model.InviteCode{}, err
		}
	}
	if len(code) < 4 || len(code) > 64 {
		return model.InviteCode{}, fmt.Errorf("邀请码长度需要在 4 到 64 个字符之间")
	}
	m.mu.Lock()
	for _, existing := range m.invites {
		if existing.Code == code {
			m.mu.Unlock()
			return model.InviteCode{}, fmt.Errorf("邀请码已存在")
		}
	}
	invite := model.InviteCode{
		ID: randomID("inv"), Code: code, Enabled: true,
		CreatedAt: time.Now(), ExpiresAt: expiresAt,
	}
	m.invites[invite.ID] = invite
	m.mu.Unlock()
	return invite, m.Save()
}

func randomInviteCode() (string, error) {
	token := make([]byte, 8)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("generate invite code: %w", err)
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(token)
	return "CHG-" + encoded, nil
}

func (m *Manager) DeleteInvite(id string) error {
	m.mu.Lock()
	if _, ok := m.invites[id]; !ok {
		m.mu.Unlock()
		return fmt.Errorf("邀请码不存在")
	}
	delete(m.invites, id)
	m.mu.Unlock()
	return m.Save()
}
