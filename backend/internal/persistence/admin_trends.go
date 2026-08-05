package persistence

import (
	"fmt"
	"time"

	"charge-dashboard/internal/model"
)

// MetricAggregate returns counter totals for a half-open time range. Keeping
// the range explicit lets the runtime build calendar buckets in any IANA
// timezone, including days affected by daylight-saving changes.
func (s *Store) MetricAggregate(start, end time.Time) (model.MetricPoint, error) {
	if !end.After(start) {
		return model.MetricPoint{}, fmt.Errorf("metric aggregate end must be after start")
	}
	rows, err := s.db.Query(`
		SELECT kind, SUM(count), COUNT(DISTINCT user_id)
		FROM metrics
		WHERE created_at >= ? AND created_at < ?
		GROUP BY kind
	`, start.Unix(), end.Unix())
	if err != nil {
		return model.MetricPoint{}, err
	}
	defer rows.Close()

	point := model.MetricPoint{Time: start.UTC()}
	for rows.Next() {
		var kind string
		var count, active int
		if err := rows.Scan(&kind, &count, &active); err != nil {
			return model.MetricPoint{}, err
		}
		switch kind {
		case "request":
			point.Requests = count
			point.ActiveUsers = active
		case "remote":
			point.Remote = count
		case "cache":
			point.CacheHits = count
		case "remote_ok":
			point.RemoteOK = count
		case "remote_failed":
			point.RemoteFailed = count
		case "cookie_error":
			point.CookieErrors = count
		}
	}
	return point, rows.Err()
}

// OfflinePortCountAt reconstructs the latest known state of every port at the
// requested instant. This is a gauge, so summing transition rows would report
// "offline observations" instead of the number of offline ports.
func (s *Store) OfflinePortCountAt(at time.Time) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM port_status_events current
		WHERE current.changed_at < ?
			AND current.to_status = ?
			AND NOT EXISTS (
				SELECT 1
				FROM port_status_events newer
				WHERE newer.user_id = current.user_id
					AND newer.device_id = current.device_id
					AND newer.port_id = current.port_id
					AND newer.changed_at < ?
					AND (
						newer.changed_at > current.changed_at
						OR (newer.changed_at = current.changed_at AND newer.id > current.id)
					)
			)
	`, at.Unix(), string(model.PortOffline), at.Unix()).Scan(&count)
	return count, err
}
