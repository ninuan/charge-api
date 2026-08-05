package runtime

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"charge-dashboard/internal/model"
)

var ErrAdminTrendQueryInvalid = errors.New("invalid admin trend query")

type adminTrendBucket struct {
	start time.Time
	end   time.Time
}

func (m *Manager) AdminTrends(rangeName, timezone string) (model.AdminTrends, error) {
	return m.adminTrendsAt(rangeName, timezone, time.Now())
}

func (m *Manager) adminTrendsAt(rangeName, timezone string, now time.Time) (model.AdminTrends, error) {
	window, location, err := normalizeAdminTrendWindow(rangeName, timezone, now)
	if err != nil {
		return model.AdminTrends{}, err
	}
	buckets := adminTrendBuckets(window, location)
	points := make([]model.AdminTrendPoint, 0, len(buckets))
	for _, bucket := range buckets {
		metrics, err := m.repository.MetricAggregate(bucket.start, bucket.end)
		if err != nil {
			return model.AdminTrends{}, fmt.Errorf("load admin trend metric bucket: %w", err)
		}
		offlinePorts, err := m.repository.OfflinePortCountAt(bucket.end)
		if err != nil {
			return model.AdminTrends{}, fmt.Errorf("load admin trend offline ports: %w", err)
		}
		points = append(points, model.AdminTrendPoint{
			Start: bucket.start, End: bucket.end,
			Requests: metrics.Requests, RemoteAttempts: metrics.Remote,
			RemoteSuccesses: metrics.RemoteOK, RemoteFailures: metrics.RemoteFailed,
			RemoteSuccessRate: metricSuccessRate(metrics.RemoteOK, metrics.Remote),
			ActiveUsers:       metrics.ActiveUsers, OfflinePorts: offlinePorts,
		})
	}

	totals, err := m.repository.MetricAggregate(window.Start, window.End)
	if err != nil {
		return model.AdminTrends{}, fmt.Errorf("load admin trend summary: %w", err)
	}
	offlinePorts, err := m.repository.OfflinePortCountAt(window.End)
	if err != nil {
		return model.AdminTrends{}, fmt.Errorf("load admin trend offline port summary: %w", err)
	}
	return model.AdminTrends{
		Window: window,
		Summary: model.AdminTrendSummary{
			Requests: totals.Requests, RemoteAttempts: totals.Remote,
			RemoteSuccesses: totals.RemoteOK, RemoteFailures: totals.RemoteFailed,
			RemoteSuccessRate: metricSuccessRate(totals.RemoteOK, totals.Remote),
			ActiveUsers:       totals.ActiveUsers, OfflinePorts: offlinePorts,
		},
		Points: points, UpdatedAt: window.End,
	}, nil
}

func normalizeAdminTrendWindow(rangeName, timezone string, now time.Time) (model.AdminTrendWindow, *time.Location, error) {
	rangeName = strings.TrimSpace(strings.ToLower(rangeName))
	if rangeName == "" {
		rangeName = "24h"
	}
	if rangeName != "24h" && rangeName != "7d" && rangeName != "30d" {
		return model.AdminTrendWindow{}, nil, fmt.Errorf("%w: unsupported range", ErrAdminTrendQueryInvalid)
	}
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	if !validTimezoneName(timezone) {
		return model.AdminTrendWindow{}, nil, fmt.Errorf("%w: invalid timezone", ErrAdminTrendQueryInvalid)
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return model.AdminTrendWindow{}, nil, fmt.Errorf("%w: unknown timezone", ErrAdminTrendQueryInvalid)
	}

	end := now.UTC().Truncate(time.Second)
	start := end.Add(-24 * time.Hour)
	bucketUnit := "hour"
	if rangeName != "24h" {
		localEnd := end.In(location)
		startOfToday := time.Date(localEnd.Year(), localEnd.Month(), localEnd.Day(), 0, 0, 0, 0, location)
		days := -6
		if rangeName == "30d" {
			days = -29
		}
		start = startOfToday.AddDate(0, 0, days).UTC()
		bucketUnit = "day"
	}
	return model.AdminTrendWindow{
		Range: rangeName, Timezone: timezone, BucketUnit: bucketUnit,
		Start: start, End: end,
	}, location, nil
}

func adminTrendBuckets(window model.AdminTrendWindow, location *time.Location) []adminTrendBucket {
	buckets := make([]adminTrendBucket, 0, 30)
	if window.BucketUnit == "hour" {
		for start := window.Start; start.Before(window.End); start = start.Add(time.Hour) {
			end := start.Add(time.Hour)
			if end.After(window.End) {
				end = window.End
			}
			buckets = append(buckets, adminTrendBucket{start: start, end: end})
		}
		return buckets
	}
	for localStart := window.Start.In(location); localStart.Before(window.End.In(location)); localStart = localStart.AddDate(0, 0, 1) {
		start := localStart.UTC()
		end := localStart.AddDate(0, 0, 1).UTC()
		if end.After(window.End) {
			end = window.End
		}
		buckets = append(buckets, adminTrendBucket{start: start, end: end})
	}
	return buckets
}

func metricSuccessRate(successes, attempts int) *float64 {
	if attempts <= 0 {
		return nil
	}
	value := roundOneDecimal(float64(successes) / float64(attempts) * 100)
	return &value
}
