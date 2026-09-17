package model

import (
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrAnnouncementConflict = errors.New("announcement changed")
var ErrAnnouncementUnavailable = errors.New("announcement unavailable")
var ErrAnnouncementInvalid = errors.New("invalid announcement")

type Announcement struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Body            string     `json:"body,omitempty"`
	Level           string     `json:"level"`
	LinkLabel       string     `json:"linkLabel,omitempty"`
	LinkURL         string     `json:"linkUrl,omitempty"`
	StartAt         time.Time  `json:"startAt"`
	EndAt           *time.Time `json:"endAt"`
	Status          string     `json:"status"`
	Version         int        `json:"version"`
	ReminderVersion int        `json:"reminderVersion"`
	Acknowledged    bool       `json:"acknowledged"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	UpdatedBy       string     `json:"updatedBy,omitempty"`
}
type AnnouncementInput struct {
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	Level       string     `json:"level"`
	LinkLabel   string     `json:"linkLabel"`
	LinkURL     string     `json:"linkUrl"`
	StartAt     time.Time  `json:"startAt"`
	EndAt       *time.Time `json:"endAt"`
	Version     int        `json:"version"`
	RemindAgain bool       `json:"remindAgain"`
}
type AnnouncementPage struct {
	Items []Announcement `json:"items"`
	Total int            `json:"total"`
	Page  int            `json:"page"`
}
type AnnouncementSummary struct {
	Item         *Announcement `json:"item"`
	UnreadCount  int           `json:"unreadCount"`
	ServerNow    time.Time     `json:"serverNow"`
	NextBoundary *time.Time    `json:"nextBoundary"`
}

func (a Announcement) State(now time.Time) string {
	if a.Status != "published" {
		return a.Status
	}
	if now.Before(a.StartAt) {
		return "scheduled"
	}
	if a.EndAt != nil && !now.Before(*a.EndAt) {
		return "ended"
	}
	return "active"
}
func (a AnnouncementInput) Validate() error {
	if utf8.RuneCountInString(strings.TrimSpace(a.Title)) < 1 || utf8.RuneCountInString(a.Title) > 60 || utf8.RuneCountInString(strings.TrimSpace(a.Body)) < 1 || utf8.RuneCountInString(a.Body) > 4000 || (a.Level != "normal" && a.Level != "important") || a.StartAt.IsZero() || (a.EndAt != nil && !a.EndAt.After(a.StartAt)) {
		return ErrAnnouncementInvalid
	}
	if (a.LinkURL == "") != (a.LinkLabel == "") || utf8.RuneCountInString(a.LinkLabel) > 20 || len(a.LinkURL) > 2048 {
		return ErrAnnouncementInvalid
	}
	if a.LinkURL != "" {
		u, e := url.Parse(a.LinkURL)
		if e != nil || u.User != nil || strings.ContainsAny(a.LinkURL, "\\\r\n\t") || !(u.Scheme == "https" && u.Host != "" || u.Scheme == "" && u.Host == "" && strings.HasPrefix(a.LinkURL, "/") && !strings.HasPrefix(a.LinkURL, "//")) {
			return ErrAnnouncementInvalid
		}
	}
	return nil
}
