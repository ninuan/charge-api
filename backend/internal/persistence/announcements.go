package persistence

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"charge-dashboard/internal/model"
)

func (s *Store) ensureAnnouncements() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS announcements (
 id TEXT PRIMARY KEY, content TEXT NOT NULL, status TEXT NOT NULL, start_at INTEGER NOT NULL, end_at INTEGER, version INTEGER NOT NULL);
 CREATE INDEX IF NOT EXISTS announcements_visibility ON announcements(status,start_at,end_at);
 CREATE TABLE IF NOT EXISTS announcement_revisions (
 announcement_id TEXT NOT NULL REFERENCES announcements(id) ON DELETE CASCADE, version INTEGER NOT NULL, content TEXT NOT NULL, PRIMARY KEY(announcement_id,version));
 CREATE TABLE IF NOT EXISTS announcement_acknowledgements (
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, announcement_id TEXT NOT NULL REFERENCES announcements(id) ON DELETE CASCADE,
 reminder_version INTEGER NOT NULL, acknowledged_at INTEGER NOT NULL, PRIMARY KEY(user_id,announcement_id,reminder_version));`)
	return err
}
func (s *Store) SaveAnnouncement(ctx context.Context, id, actor, action string, in model.AnnouncementInput, now time.Time) (model.Announcement, error) {
	var a model.Announcement
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return a, err
	}
	defer tx.Rollback()
	if id != "" {
		var raw string
		if err = tx.QueryRowContext(ctx, `SELECT content FROM announcements WHERE id=?`, id).Scan(&raw); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				err = model.ErrAnnouncementUnavailable
			}
			return a, err
		}
		if err = json.Unmarshal([]byte(raw), &a); err != nil {
			return a, err
		}
		if in.Version != a.Version {
			return a, model.ErrAnnouncementConflict
		}
	} else {
		if action != "save" {
			return a, model.ErrAnnouncementInvalid
		}
		var b [16]byte
		if _, err = rand.Read(b[:]); err != nil {
			return a, err
		}
		a.ID = hex.EncodeToString(b[:])
		a.Status = "draft"
		a.CreatedAt = now
		a.ReminderVersion = 1
	}
	switch action {
	case "save":
		if a.State(now) == "ended" || a.Status == "withdrawn" {
			return a, model.ErrAnnouncementConflict
		}
		if err = in.Validate(); err != nil {
			return a, err
		}
		if a.Status == "published" && (in.StartAt.After(now) && !a.StartAt.After(now) || in.EndAt != nil && !in.EndAt.After(now)) {
			return a, model.ErrAnnouncementInvalid
		}
		a.Title = in.Title
		a.Body = in.Body
		a.Level = in.Level
		a.LinkLabel = in.LinkLabel
		a.LinkURL = in.LinkURL
		a.StartAt = in.StartAt.UTC()
		a.EndAt = in.EndAt
		if a.Status == "published" && in.RemindAgain {
			a.ReminderVersion++
		}
	case "publish":
		if a.Status != "draft" || a.EndAt != nil && !a.EndAt.After(now) {
			return a, model.ErrAnnouncementConflict
		}
		a.Status = "published"
	case "withdraw":
		if a.Status != "published" {
			return a, model.ErrAnnouncementConflict
		}
		a.Status = "withdrawn"
	case "delete":
		if a.Status != "draft" {
			return a, model.ErrAnnouncementConflict
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM announcements WHERE id=?`, id)
		if err != nil {
			return a, err
		}
		return a, tx.Commit()
	default:
		return a, model.ErrAnnouncementInvalid
	}
	a.Version++
	a.UpdatedAt = now
	a.UpdatedBy = actor
	raw, err := json.Marshal(a)
	if err != nil {
		return a, err
	}
	var end any
	if a.EndAt != nil {
		end = a.EndAt.UnixMilli()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO announcements(id,content,status,start_at,end_at,version) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET content=excluded.content,status=excluded.status,start_at=excluded.start_at,end_at=excluded.end_at,version=excluded.version`, a.ID, string(raw), a.Status, a.StartAt.UnixMilli(), end, a.Version)
	if err != nil {
		return a, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO announcement_revisions(announcement_id,version,content) VALUES(?,?,?)`, a.ID, a.Version, string(raw))
	if err != nil {
		return a, err
	}
	return a, tx.Commit()
}
func (s *Store) Announcements(ctx context.Context, userID string, admin bool, now time.Time) ([]model.Announcement, error) {
	query := `SELECT a.content, EXISTS(SELECT 1 FROM announcement_acknowledgements k WHERE k.announcement_id=a.id AND k.user_id=? AND k.reminder_version=json_extract(a.content,'$.reminderVersion')) FROM announcements a`
	args := []any{userID}
	if !admin {
		query += ` WHERE a.status='published' AND a.start_at<=?`
		args = append(args, now.UnixMilli())
	}
	query += ` ORDER BY a.start_at DESC,a.id DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Announcement{}
	for rows.Next() {
		var raw string
		var ack bool
		if err = rows.Scan(&raw, &ack); err != nil {
			return nil, err
		}
		var a model.Announcement
		if err = json.Unmarshal([]byte(raw), &a); err != nil {
			return nil, err
		}
		a.Acknowledged = ack
		a.Status = a.State(now)
		if !admin {
			a.UpdatedBy = ""
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Store) AnnouncementBoundary(ctx context.Context, now time.Time) (*time.Time, error) {
	var stamp sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MIN(t) FROM (SELECT start_at t FROM announcements WHERE status='published' UNION ALL SELECT end_at t FROM announcements WHERE status='published') WHERE t>?`, now.UnixMilli()).Scan(&stamp)
	if err != nil || !stamp.Valid {
		return nil, err
	}
	t := time.UnixMilli(stamp.Int64).UTC()
	return &t, nil
}
func (s *Store) AcknowledgeAnnouncement(ctx context.Context, userID, id string, version, reminder int, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if err = tx.QueryRowContext(ctx, `SELECT content FROM announcements WHERE id=?`, id).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ErrAnnouncementUnavailable
		}
		return err
	}
	var a model.Announcement
	if err = json.Unmarshal([]byte(raw), &a); err != nil {
		return err
	}
	if a.State(now) != "active" {
		return model.ErrAnnouncementUnavailable
	}
	if a.Version != version || a.ReminderVersion != reminder {
		return model.ErrAnnouncementConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO announcement_acknowledgements(user_id,announcement_id,reminder_version,acknowledged_at) VALUES(?,?,?,?)`, userID, id, reminder, now.UnixMilli())
	if err != nil {
		return err
	}
	return tx.Commit()
}

// QueryAnnouncementPage limits both database reads and response payloads; archived
// bodies are never decoded just to render a list or homepage summary.
func (s *Store) QueryAnnouncementPage(ctx context.Context, user string, admin bool, state, search, id string, page int, summary bool, now time.Time) (model.AnnouncementPage, error) {
	out := model.AnnouncementPage{Items: []model.Announcement{}, Page: page}
	stateSQL := `CASE WHEN status!='published' THEN status WHEN start_at>? THEN 'scheduled' WHEN end_at IS NOT NULL AND end_at<=? THEN 'ended' ELSE 'active' END`
	base := `WITH visible AS (SELECT a.*, ` + stateSQL + ` AS current_status, EXISTS(SELECT 1 FROM announcement_acknowledgements k WHERE k.announcement_id=a.id AND k.user_id=? AND k.reminder_version=json_extract(a.content,'$.reminderVersion')) AS ack FROM announcements a) `
	args := []any{now.UnixMilli(), now.UnixMilli(), user}
	where := ` WHERE 1=1`
	if !admin {
		where += ` AND current_status IN ('active','ended')`
	}
	if state != "" {
		where += ` AND current_status=?`
		args = append(args, state)
	}
	if search != "" {
		where += ` AND instr(lower(json_extract(content,'$.title')),lower(?))>0`
		args = append(args, search)
	}
	if id != "" {
		where += ` AND id=?`
		args = append(args, id)
	}
	if summary {
		where += ` AND current_status='active' AND (ack=0 OR json_extract(content,'$.level')='important')`
	}
	if err := s.db.QueryRowContext(ctx, base+`SELECT COUNT(*) FROM visible`+where, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	content := `json_remove(content,'$.body')`
	if id != "" {
		content = "content"
	}
	order := ` ORDER BY start_at DESC,id DESC LIMIT 20 OFFSET ?`
	offset := (page - 1) * 20
	if summary {
		order = ` ORDER BY (json_extract(content,'$.level')='important') DESC,start_at DESC,id DESC LIMIT 1 OFFSET ?`
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, base+`SELECT `+content+`,current_status,ack FROM visible`+where+order, append(args, offset)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw, status string
		var ack bool
		if err = rows.Scan(&raw, &status, &ack); err != nil {
			return out, err
		}
		var a model.Announcement
		if err = json.Unmarshal([]byte(raw), &a); err != nil {
			return out, err
		}
		a.Status = status
		a.Acknowledged = ack
		if !admin {
			a.UpdatedBy = ""
		}
		out.Items = append(out.Items, a)
	}
	return out, rows.Err()
}
func (s *Store) AnnouncementUnreadCount(ctx context.Context, user string, now time.Time) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM announcements a WHERE status='published' AND start_at<=? AND (end_at IS NULL OR end_at>?) AND NOT EXISTS(SELECT 1 FROM announcement_acknowledgements k WHERE k.user_id=? AND k.announcement_id=a.id AND k.reminder_version=json_extract(a.content,'$.reminderVersion'))`, now.UnixMilli(), now.UnixMilli(), user).Scan(&count)
	return count, err
}

func (s *Store) AnnouncementRevisions(ctx context.Context, id string, page int) (model.AnnouncementPage, error) {
	out := model.AnnouncementPage{Items: []model.Announcement{}, Page: page}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM announcement_revisions WHERE announcement_id=?`, id).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT content FROM announcement_revisions WHERE announcement_id=? ORDER BY version DESC LIMIT 20 OFFSET ?`, id, (page-1)*20)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		var a model.Announcement
		if err = rows.Scan(&raw); err != nil {
			return out, err
		}
		if err = json.Unmarshal([]byte(raw), &a); err != nil {
			return out, err
		}
		out.Items = append(out.Items, a)
	}
	return out, rows.Err()
}
