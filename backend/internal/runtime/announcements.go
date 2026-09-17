package runtime

import (
	"context"
	"time"

	"charge-dashboard/internal/model"
)

func (m *Manager) SaveAnnouncement(ctx context.Context, id, actor, action string, in model.AnnouncementInput) (model.Announcement, error) {
	return m.repository.SaveAnnouncement(ctx, id, actor, action, in, time.Now().UTC())
}
func (m *Manager) AcknowledgeAnnouncement(ctx context.Context, user, id string, version, reminder int) error {
	return m.repository.AcknowledgeAnnouncement(ctx, user, id, version, reminder, time.Now().UTC())
}
func (m *Manager) AnnouncementList(ctx context.Context, user string, admin bool, state, search string, page int) (model.AnnouncementPage, error) {
	return m.repository.QueryAnnouncementPage(ctx, user, admin, state, search, "", page, false, time.Now().UTC())
}
func (m *Manager) AnnouncementDetail(ctx context.Context, user, id string, admin bool) (model.Announcement, error) {
	list, err := m.repository.QueryAnnouncementPage(ctx, user, admin, "", "", id, 1, false, time.Now().UTC())
	if err != nil {
		return model.Announcement{}, err
	}
	if len(list.Items) == 0 {
		return model.Announcement{}, model.ErrAnnouncementUnavailable
	}
	return list.Items[0], nil
}
func (m *Manager) AnnouncementSummary(ctx context.Context, user string) (model.AnnouncementSummary, error) {
	now := time.Now().UTC()
	out := model.AnnouncementSummary{ServerNow: now}
	list, err := m.repository.QueryAnnouncementPage(ctx, user, false, "", "", "", 1, true, now)
	if err != nil {
		return out, err
	}
	if len(list.Items) > 0 {
		a := list.Items[0]
		a.LinkURL = ""
		a.LinkLabel = ""
		out.Item = &a
	}
	out.UnreadCount, err = m.repository.AnnouncementUnreadCount(ctx, user, now)
	if err != nil {
		return out, err
	}
	out.NextBoundary, err = m.repository.AnnouncementBoundary(ctx, now)
	return out, err
}
func (m *Manager) AnnouncementRevisions(ctx context.Context, id string, page int) (model.AnnouncementPage, error) {
	return m.repository.AnnouncementRevisions(ctx, id, page)
}
