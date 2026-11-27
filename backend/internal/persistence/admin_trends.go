package persistence

import (
	"fmt"
	"time"

	"charge-dashboard/internal/model"
)

// MetricRange represents a half-open [Start, End) time interval.
type MetricRange struct {
	Start time.Time
	End   time.Time
}

// MetricAggregate returns counter totals for a half-open time range. Keeping
// the range explicit lets the runtime build calendar buckets in any IANA
// timezone, including days affected by daylight-saving changes.
func (s *Store) MetricAggregate(start, end time.Time) (model.MetricPoint, error) {
	if !end.After(start) {
		return model.MetricPoint{}, fmt.Errorf("metric aggregate end must be after start")
	}
	rows, err := s.reader().Query(`
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

// MetricBuckets aggregates multiple chronological non-overlapping ranges plus
// the overall [windowStart, windowEnd) summary in a single SQLite range scan,
// replacing the 31-query N+1 loop on 30-day admin trends.
func (s *Store) MetricBuckets(ranges []MetricRange, windowStart, windowEnd time.Time) ([]model.MetricPoint, model.MetricPoint, error) {
	if !windowEnd.After(windowStart) {
		return nil, model.MetricPoint{}, fmt.Errorf("metric buckets end must be after start")
	}
	points := make([]model.MetricPoint, len(ranges))
	bucketUsers := make([]map[string]struct{}, len(ranges))
	for i, r := range ranges {
		if !r.End.After(r.Start) {
			return nil, model.MetricPoint{}, fmt.Errorf("metric bucket end must be after start")
		}
		if i > 0 && r.Start.Before(ranges[i-1].Start) {
			return nil, model.MetricPoint{}, fmt.Errorf("metric buckets must be ordered")
		}
		points[i] = model.MetricPoint{Time: r.Start.UTC()}
		bucketUsers[i] = make(map[string]struct{})
	}
	summary := model.MetricPoint{Time: windowStart.UTC()}
	summaryUsers := make(map[string]struct{})

	minStart := windowStart.Unix()
	maxEnd := windowEnd.Unix()
	for _, r := range ranges {
		if r.Start.Unix() < minStart {
			minStart = r.Start.Unix()
		}
		if r.End.Unix() > maxEnd {
			maxEnd = r.End.Unix()
		}
	}

	rows, err := s.reader().Query(`
		SELECT user_id, kind, created_at, count
		FROM metrics
		WHERE created_at >= ? AND created_at < ?
		ORDER BY created_at ASC
	`, minStart, maxEnd)
	if err != nil {
		return nil, model.MetricPoint{}, err
	}
	defer rows.Close()

	addKind := func(target *model.MetricPoint, activeSet map[string]struct{}, userID, kind string, count int) {
		switch kind {
		case "request":
			target.Requests += count
			activeSet[userID] = struct{}{}
		case "remote":
			target.Remote += count
		case "cache":
			target.CacheHits += count
		case "remote_ok":
			target.RemoteOK += count
		case "remote_failed":
			target.RemoteFailed += count
		case "cookie_error":
			target.CookieErrors += count
		}
	}

	winStartUnix := windowStart.Unix()
	winEndUnix := windowEnd.Unix()
	bucketIdx := 0
	for rows.Next() {
		var userID, kind string
		var createdAt int64
		var count int
		if err := rows.Scan(&userID, &kind, &createdAt, &count); err != nil {
			return nil, model.MetricPoint{}, err
		}
		if createdAt >= winStartUnix && createdAt < winEndUnix {
			addKind(&summary, summaryUsers, userID, kind, count)
		}
		for bucketIdx < len(ranges) && createdAt >= ranges[bucketIdx].End.Unix() {
			bucketIdx++
		}
		for idx := bucketIdx; idx < len(ranges); idx++ {
			if createdAt < ranges[idx].Start.Unix() {
				break
			}
			if createdAt < ranges[idx].End.Unix() {
				addKind(&points[idx], bucketUsers[idx], userID, kind, count)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, model.MetricPoint{}, err
	}
	for i := range points {
		points[i].ActiveUsers = len(bucketUsers[i])
	}
	summary.ActiveUsers = len(summaryUsers)
	return points, summary, nil
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

	rows, err := s.reader().Query(`
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
