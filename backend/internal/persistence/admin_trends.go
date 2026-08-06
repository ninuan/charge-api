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
	counts, err := s.OfflinePortCountsAt([]time.Time{at})
	if err != nil {
		return 0, err
	}
	return counts[0], nil
}

// OfflinePortCountsAt reconstructs multiple gauge samples in one chronological
// pass. Admin trend ranges ask for up to 30 adjacent samples; running the full
// latest-state query once per bucket made that endpoint scale with
// events*bucket-count instead of events+bucket-count.
func (s *Store) OfflinePortCountsAt(instants []time.Time) ([]int, error) {
	if len(instants) == 0 {
		return []int{}, nil
	}
	for index, instant := range instants {
		if instant.IsZero() {
			return nil, fmt.Errorf("offline port count requires a valid instant")
		}
		if index > 0 && instant.Before(instants[index-1]) {
			return nil, fmt.Errorf("offline port count instants must be ordered")
		}
	}

	rows, err := s.db.Query(`
		SELECT user_id, device_id, port_id, to_status, changed_at
		FROM port_status_events
		WHERE changed_at < ?
		ORDER BY changed_at, id
	`, instants[len(instants)-1].Unix())
	if err != nil {
		return nil, fmt.Errorf("query offline port history: %w", err)
	}
	defer rows.Close()

	type portKey struct {
		userID   string
		deviceID string
		portID   int
	}
	type statusEvent struct {
		key       portKey
		status    string
		changedAt int64
	}

	latest := make(map[portKey]string)
	counts := make([]int, len(instants))
	offline := 0
	var next statusEvent
	hasNext := rows.Next()
	readNext := func() error {
		if !hasNext {
			return nil
		}
		if err := rows.Scan(
			&next.key.userID,
			&next.key.deviceID,
			&next.key.portID,
			&next.status,
			&next.changedAt,
		); err != nil {
			return fmt.Errorf("scan offline port history: %w", err)
		}
		return nil
	}
	if err := readNext(); err != nil {
		return nil, err
	}

	for index, instant := range instants {
		cutoff := instant.Unix()
		for hasNext && next.changedAt < cutoff {
			previous := latest[next.key]
			if previous == string(model.PortOffline) && next.status != string(model.PortOffline) {
				offline--
			} else if previous != string(model.PortOffline) && next.status == string(model.PortOffline) {
				offline++
			}
			latest[next.key] = next.status
			hasNext = rows.Next()
			if err := readNext(); err != nil {
				return nil, err
			}
		}
		counts[index] = offline
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate offline port history: %w", err)
	}
	return counts, nil
}
