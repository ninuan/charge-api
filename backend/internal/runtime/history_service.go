package runtime

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/persistence"
)

const (
	historyAnalysisEventLimit = 10_000
	historyTimelineLimit      = 200
	historyNotice             = "历史从启用记录后开始，首次记录之前的时段不会计为空闲。"
	quietSampleSeconds        = 6 * 60 * 60
)

var (
	ErrHistoryNotFound     = errors.New("history target not found")
	ErrHistoryQueryInvalid = errors.New("invalid history query")
	ErrHistoryTooLarge     = errors.New("history query exceeds event limit")
)

type historyDurations struct {
	idle    int64
	inUse   int64
	offline int64
}

func (d *historyDurations) add(status model.PortStatus, seconds int64) {
	if seconds <= 0 {
		return
	}
	switch status {
	case model.PortIdle:
		d.idle += seconds
	case model.PortInUse:
		d.inUse += seconds
	case model.PortOffline:
		d.offline += seconds
	}
}

func (d historyDurations) observed() int64 {
	return d.idle + d.inUse + d.offline
}

type historySegment struct {
	status model.PortStatus
	start  time.Time
	end    time.Time
}

type historySession struct {
	start time.Time
	end   time.Time
}

type analyzedPortHistory struct {
	segments []historySegment
	sessions []historySession
	timeline []model.PortHistoryTimelineItem
}

type dailyHistoryAggregate struct {
	durations historyDurations
	sessions  []int64
}

type heatmapHistoryAggregate struct {
	durations historyDurations
	dates     map[string]struct{}
}

func (m *Manager) DeviceHistory(userID, deviceID, rangeName, timezone string) (model.DeviceHistoryResponse, error) {
	return m.deviceHistoryAt(userID, deviceID, rangeName, timezone, time.Now())
}

func (m *Manager) PortHistory(userID, deviceID string, portID int, rangeName, timezone string) (model.PortHistoryResponse, error) {
	return m.portHistoryAt(userID, deviceID, portID, rangeName, timezone, time.Now())
}

func (m *Manager) deviceHistoryAt(userID, deviceID, rangeName, timezone string, now time.Time) (model.DeviceHistoryResponse, error) {
	pile, err := m.historyPile(userID, deviceID)
	if err != nil {
		return model.DeviceHistoryResponse{}, err
	}
	window, location, err := normalizeHistoryWindow(rangeName, timezone, now)
	if err != nil {
		return model.DeviceHistoryResponse{}, err
	}
	starts, err := m.repository.PortStatusEventStarts(userID, deviceID)
	if err != nil {
		return model.DeviceHistoryResponse{}, fmt.Errorf("load port history starts: %w", err)
	}
	events, truncated, err := m.repository.PortStatusEventsForAnalysis(persistence.PortStatusEventQuery{
		UserID: userID, DeviceID: deviceID, Since: window.Start, Until: window.End,
	}, historyAnalysisEventLimit)
	if err != nil {
		return model.DeviceHistoryResponse{}, fmt.Errorf("load device history events: %w", err)
	}
	if truncated {
		return model.DeviceHistoryResponse{}, ErrHistoryTooLarge
	}

	currentStatuses := make(map[int]model.PortStatus, len(pile.Ports))
	portIDs := make(map[int]struct{}, len(pile.Ports)+len(starts))
	for _, port := range pile.Ports {
		currentStatuses[port.ID] = port.Status
		portIDs[port.ID] = struct{}{}
	}
	for portID := range starts {
		portIDs[portID] = struct{}{}
	}
	eventsByPort := groupHistoryEvents(events)
	for portID, items := range eventsByPort {
		portIDs[portID] = struct{}{}
		if _, ok := currentStatuses[portID]; !ok && len(items) > 0 {
			currentStatuses[portID] = items[len(items)-1].ToStatus
		}
	}

	orderedPortIDs := make([]int, 0, len(portIDs))
	for portID := range portIDs {
		orderedPortIDs = append(orderedPortIDs, portID)
	}
	sort.Ints(orderedPortIDs)

	analyses := make(map[int]analyzedPortHistory, len(orderedPortIDs))
	portSummaries := make([]model.PortHistorySummary, 0, len(orderedPortIDs))
	var historyStartedAt *time.Time
	for _, portID := range orderedPortIDs {
		analysis := analyzePortHistory(eventsByPort[portID], window)
		analyses[portID] = analysis
		metrics := metricsForAnalysis(analysis, window.End.Sub(window.Start).Seconds())
		start := optionalTime(starts, portID)
		if start != nil && (historyStartedAt == nil || start.Before(*historyStartedAt)) {
			value := *start
			historyStartedAt = &value
		}
		portSummaries = append(portSummaries, model.PortHistorySummary{
			PortID: portID, CurrentStatus: currentStatuses[portID],
			HistoryStartedAt: start, Metrics: metrics,
		})
	}

	metrics, daily, heatmap := aggregateDeviceHistory(analyses, orderedPortIDs, window, location)
	busiest, quiet := heatmapInsights(heatmap)
	return model.DeviceHistoryResponse{
		Device: model.HistoryDevice{
			ID: pile.ID, Number: pile.Number, Name: pile.Name, Address: pile.Address,
		},
		Window: window, Metrics: metrics, Daily: daily, Heatmap: heatmap,
		Ports: portSummaries, BusiestHours: busiest, QuietSuggestion: quiet,
		HistoryStartedAt: historyStartedAt, HistoryNotice: historyNotice,
	}, nil
}

func (m *Manager) portHistoryAt(userID, deviceID string, portID int, rangeName, timezone string, now time.Time) (model.PortHistoryResponse, error) {
	if portID <= 0 {
		return model.PortHistoryResponse{}, fmt.Errorf("%w: invalid port id", ErrHistoryQueryInvalid)
	}
	pile, err := m.historyPile(userID, deviceID)
	if err != nil {
		return model.PortHistoryResponse{}, err
	}
	window, location, err := normalizeHistoryWindow(rangeName, timezone, now)
	if err != nil {
		return model.PortHistoryResponse{}, err
	}
	starts, err := m.repository.PortStatusEventStarts(userID, deviceID)
	if err != nil {
		return model.PortHistoryResponse{}, fmt.Errorf("load port history start: %w", err)
	}
	currentStatus, currentPort := currentPilePort(pile, portID)
	if !currentPort {
		if _, hasHistory := starts[portID]; !hasHistory {
			return model.PortHistoryResponse{}, ErrHistoryNotFound
		}
	}
	events, truncated, err := m.repository.PortStatusEventsForAnalysis(persistence.PortStatusEventQuery{
		UserID: userID, DeviceID: deviceID, PortID: &portID,
		Since: window.Start, Until: window.End,
	}, historyAnalysisEventLimit)
	if err != nil {
		return model.PortHistoryResponse{}, fmt.Errorf("load port history events: %w", err)
	}
	if truncated {
		return model.PortHistoryResponse{}, ErrHistoryTooLarge
	}
	if !currentPort && len(events) > 0 {
		currentStatus = events[len(events)-1].ToStatus
	}
	analysis := analyzePortHistory(events, window)
	timeline := analysis.timeline
	timelineTruncated := len(timeline) > historyTimelineLimit
	if timelineTruncated {
		timeline = timeline[len(timeline)-historyTimelineLimit:]
	}
	reverseTimeline(timeline)
	return model.PortHistoryResponse{
		Device: model.HistoryDevice{
			ID: pile.ID, Number: pile.Number, Name: pile.Name, Address: pile.Address,
		},
		PortID: portID, CurrentStatus: currentStatus, Window: window,
		Metrics:  metricsForAnalysis(analysis, window.End.Sub(window.Start).Seconds()),
		Daily:    aggregatePortDaily(analysis, window, location),
		Timeline: timeline, TimelineTruncated: timelineTruncated,
		HistoryStartedAt: optionalTime(starts, portID), HistoryNotice: historyNotice,
	}, nil
}

func (m *Manager) historyPile(userID, deviceID string) (model.Pile, error) {
	deviceID = strings.TrimSpace(deviceID)
	runtime, err := m.runtimeFor(userID)
	if err != nil || deviceID == "" {
		return model.Pile{}, ErrHistoryNotFound
	}
	owned := false
	for _, id := range runtime.client.DeviceIDs() {
		if id == deviceID {
			owned = true
			break
		}
	}
	if !owned {
		return model.Pile{}, ErrHistoryNotFound
	}
	for _, pile := range runtime.store.Snapshot().Piles {
		if pile.ID == deviceID {
			return pile, nil
		}
	}
	return model.Pile{ID: deviceID, Ports: []model.Port{}}, nil
}

func normalizeHistoryWindow(rangeName, timezone string, now time.Time) (model.HistoryWindow, *time.Location, error) {
	rangeName = strings.TrimSpace(strings.ToLower(rangeName))
	if rangeName == "" {
		rangeName = "7d"
	}
	switch rangeName {
	case "24h", "7d", "30d":
	default:
		return model.HistoryWindow{}, nil, fmt.Errorf("%w: unsupported range", ErrHistoryQueryInvalid)
	}
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	if !validTimezoneName(timezone) {
		return model.HistoryWindow{}, nil, fmt.Errorf("%w: invalid timezone", ErrHistoryQueryInvalid)
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return model.HistoryWindow{}, nil, fmt.Errorf("%w: unknown timezone", ErrHistoryQueryInvalid)
	}
	end := now.UTC().Truncate(time.Second)
	var start time.Time
	if rangeName == "24h" {
		start = end.Add(-24 * time.Hour)
	} else {
		localEnd := end.In(location)
		startOfToday := time.Date(localEnd.Year(), localEnd.Month(), localEnd.Day(), 0, 0, 0, 0, location)
		days := -6
		if rangeName == "30d" {
			days = -29
		}
		start = startOfToday.AddDate(0, 0, days).UTC()
	}
	return model.HistoryWindow{
		Range: rangeName, Timezone: timezone, Start: start, End: end,
	}, location, nil
}

func validTimezoneName(value string) bool {
	if value == "" || len(value) > 64 || strings.HasPrefix(value, "/") || strings.Contains(value, "..") {
		return false
	}
	if value != "UTC" && !strings.Contains(value, "/") {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("_+-/", character) {
			continue
		}
		return false
	}
	return true
}

func groupHistoryEvents(events []model.PortStatusEvent) map[int][]model.PortStatusEvent {
	grouped := make(map[int][]model.PortStatusEvent)
	for _, event := range events {
		grouped[event.PortID] = append(grouped[event.PortID], event)
	}
	for portID := range grouped {
		sort.SliceStable(grouped[portID], func(i, j int) bool {
			if grouped[portID][i].ChangedAt.Equal(grouped[portID][j].ChangedAt) {
				return grouped[portID][i].ID < grouped[portID][j].ID
			}
			return grouped[portID][i].ChangedAt.Before(grouped[portID][j].ChangedAt)
		})
	}
	return grouped
}

func analyzePortHistory(events []model.PortStatusEvent, window model.HistoryWindow) analyzedPortHistory {
	if len(events) == 0 {
		return analyzedPortHistory{segments: []historySegment{}, sessions: []historySession{}, timeline: []model.PortHistoryTimelineItem{}}
	}
	events = append([]model.PortStatusEvent(nil), events...)
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].ChangedAt.Equal(events[j].ChangedAt) {
			return events[i].ID < events[j].ID
		}
		return events[i].ChangedAt.Before(events[j].ChangedAt)
	})

	result := analyzedPortHistory{
		segments: []historySegment{}, sessions: []historySession{}, timeline: []model.PortHistoryTimelineItem{},
	}
	var predecessor *model.PortStatusEvent
	var rangeEvents []model.PortStatusEvent
	for index := range events {
		event := events[index]
		if event.ChangedAt.Before(window.Start) {
			copy := event
			predecessor = &copy
			continue
		}
		if event.ChangedAt.Before(window.End) {
			rangeEvents = append(rangeEvents, event)
			result.timeline = append(result.timeline, model.PortHistoryTimelineItem{
				PortID: event.PortID, FromStatus: event.FromStatus, ToStatus: event.ToStatus,
				ChangedAt: event.ChangedAt, UsedSeconds: event.UsedSeconds, RemainingText: event.RemainingText,
			})
		}
	}

	known := false
	status := model.PortStatus("")
	cursor := window.Start
	var sessionStart *time.Time
	if predecessor != nil {
		known = true
		status = predecessor.ToStatus
		if status == model.PortInUse && predecessor.FromStatus != nil && *predecessor.FromStatus != model.PortInUse {
			value := predecessor.ChangedAt
			sessionStart = &value
		}
	} else if len(rangeEvents) > 0 && rangeEvents[0].ChangedAt.After(window.Start) && rangeEvents[0].FromStatus != nil {
		known = true
		status = *rangeEvents[0].FromStatus
	}

	for _, event := range rangeEvents {
		eventAt := event.ChangedAt
		if eventAt.Before(window.Start) {
			eventAt = window.Start
		}
		if known && eventAt.After(cursor) {
			result.segments = append(result.segments, historySegment{status: status, start: cursor, end: eventAt})
		}
		if known && status == model.PortInUse && event.ToStatus != model.PortInUse {
			if sessionStart != nil && event.ChangedAt.After(*sessionStart) {
				result.sessions = append(result.sessions, historySession{start: *sessionStart, end: event.ChangedAt})
			}
			sessionStart = nil
		}
		if event.ToStatus == model.PortInUse && (!known || status != model.PortInUse) {
			if event.FromStatus != nil && *event.FromStatus != model.PortInUse {
				value := event.ChangedAt
				sessionStart = &value
			} else {
				sessionStart = nil
			}
		}
		known = true
		status = event.ToStatus
		cursor = eventAt
	}
	if known && window.End.After(cursor) {
		result.segments = append(result.segments, historySegment{status: status, start: cursor, end: window.End})
	}
	return result
}

func metricsForAnalysis(analysis analyzedPortHistory, expectedSeconds float64) model.PortHistoryMetrics {
	var durations historyDurations
	for _, segment := range analysis.segments {
		durations.add(segment.status, int64(segment.end.Sub(segment.start).Seconds()))
	}
	sessions := make([]int64, 0, len(analysis.sessions))
	for _, session := range analysis.sessions {
		if seconds := int64(session.end.Sub(session.start).Seconds()); seconds > 0 {
			sessions = append(sessions, seconds)
		}
	}
	return buildHistoryMetrics(durations, int64(expectedSeconds), sessions)
}

func buildHistoryMetrics(durations historyDurations, expectedSeconds int64, sessions []int64) model.PortHistoryMetrics {
	observed := durations.observed()
	gap := expectedSeconds - observed
	if gap < 0 {
		gap = 0
	}
	available := durations.idle + durations.inUse
	var occupancy *float64
	if available > 0 {
		value := roundOneDecimal(float64(durations.inUse) / float64(available) * 100)
		occupancy = &value
	}
	var average *int64
	if len(sessions) > 0 {
		var total int64
		for _, seconds := range sessions {
			total += seconds
		}
		value := total / int64(len(sessions))
		average = &value
	}
	sampleState := "sufficient"
	switch {
	case observed == 0:
		sampleState = "no_data"
	case available == 0:
		sampleState = "insufficient"
	case gap > 0:
		sampleState = "partial"
	}
	return model.PortHistoryMetrics{
		ObservedSeconds: observed, GapSeconds: gap,
		IdleSeconds: durations.idle, InUseSeconds: durations.inUse, OfflineSeconds: durations.offline,
		OccupancyPercent: occupancy, CompletedSessions: len(sessions),
		AverageSessionSeconds: average, SampleState: sampleState,
	}
}

func aggregateDeviceHistory(
	analyses map[int]analyzedPortHistory,
	portIDs []int,
	window model.HistoryWindow,
	location *time.Location,
) (model.PortHistoryMetrics, []model.HistoryDailyPoint, []model.HistoryHeatmapCell) {
	var total historyDurations
	allSessions := make([]int64, 0)
	daily := make(map[string]*dailyHistoryAggregate)
	heatmap := make(map[[2]int]*heatmapHistoryAggregate)
	for _, portID := range portIDs {
		analysis := analyses[portID]
		for _, segment := range analysis.segments {
			seconds := int64(segment.end.Sub(segment.start).Seconds())
			total.add(segment.status, seconds)
			addSegmentToDaily(daily, segment, location)
			addSegmentToHeatmap(heatmap, segment, location)
		}
		for _, session := range analysis.sessions {
			seconds := int64(session.end.Sub(session.start).Seconds())
			if seconds <= 0 {
				continue
			}
			allSessions = append(allSessions, seconds)
			date := session.end.In(location).Format("2006-01-02")
			bucket := ensureDailyAggregate(daily, date)
			bucket.sessions = append(bucket.sessions, seconds)
		}
	}
	expected := int64(window.End.Sub(window.Start).Seconds()) * int64(len(portIDs))
	metrics := buildHistoryMetrics(total, expected, allSessions)
	return metrics, dailyPoints(daily, window, location, len(portIDs)), heatmapCells(heatmap)
}

func aggregatePortDaily(analysis analyzedPortHistory, window model.HistoryWindow, location *time.Location) []model.HistoryDailyPoint {
	daily := make(map[string]*dailyHistoryAggregate)
	for _, segment := range analysis.segments {
		addSegmentToDaily(daily, segment, location)
	}
	for _, session := range analysis.sessions {
		seconds := int64(session.end.Sub(session.start).Seconds())
		if seconds <= 0 {
			continue
		}
		date := session.end.In(location).Format("2006-01-02")
		bucket := ensureDailyAggregate(daily, date)
		bucket.sessions = append(bucket.sessions, seconds)
	}
	return dailyPoints(daily, window, location, 1)
}

func addSegmentToDaily(buckets map[string]*dailyHistoryAggregate, segment historySegment, location *time.Location) {
	for cursor := segment.start; cursor.Before(segment.end); {
		local := cursor.In(location)
		next := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, location).UTC()
		if !next.After(cursor) || next.After(segment.end) {
			next = segment.end
		}
		bucket := ensureDailyAggregate(buckets, local.Format("2006-01-02"))
		bucket.durations.add(segment.status, int64(next.Sub(cursor).Seconds()))
		cursor = next
	}
}

func addSegmentToHeatmap(buckets map[[2]int]*heatmapHistoryAggregate, segment historySegment, location *time.Location) {
	for cursor := segment.start; cursor.Before(segment.end); {
		local := cursor.In(location)
		next := time.Date(local.Year(), local.Month(), local.Day(), local.Hour()+1, 0, 0, 0, location).UTC()
		if !next.After(cursor) {
			next = cursor.Add(time.Hour)
		}
		if next.After(segment.end) {
			next = segment.end
		}
		key := [2]int{isoWeekday(local.Weekday()), local.Hour()}
		bucket := buckets[key]
		if bucket == nil {
			bucket = &heatmapHistoryAggregate{dates: make(map[string]struct{})}
			buckets[key] = bucket
		}
		bucket.durations.add(segment.status, int64(next.Sub(cursor).Seconds()))
		if segment.status == model.PortIdle || segment.status == model.PortInUse {
			bucket.dates[local.Format("2006-01-02")] = struct{}{}
		}
		cursor = next
	}
}

func ensureDailyAggregate(buckets map[string]*dailyHistoryAggregate, date string) *dailyHistoryAggregate {
	bucket := buckets[date]
	if bucket == nil {
		bucket = &dailyHistoryAggregate{}
		buckets[date] = bucket
	}
	return bucket
}

func dailyPoints(
	buckets map[string]*dailyHistoryAggregate,
	window model.HistoryWindow,
	location *time.Location,
	portCount int,
) []model.HistoryDailyPoint {
	points := make([]model.HistoryDailyPoint, 0)
	localStart := window.Start.In(location)
	day := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, location)
	localEndDate := window.End.In(location).Format("2006-01-02")
	for day.UTC().Before(window.End) || (window.Range != "24h" && day.Format("2006-01-02") == localEndDate) {
		nextDay := day.AddDate(0, 0, 1)
		start := day.UTC()
		if start.Before(window.Start) {
			start = window.Start
		}
		end := nextDay.UTC()
		if end.After(window.End) {
			end = window.End
		}
		date := day.Format("2006-01-02")
		bucket := buckets[date]
		if bucket == nil {
			bucket = &dailyHistoryAggregate{}
		}
		expected := int64(end.Sub(start).Seconds()) * int64(portCount)
		points = append(points, model.HistoryDailyPoint{
			Date: date, Metrics: buildHistoryMetrics(bucket.durations, expected, bucket.sessions),
		})
		day = nextDay
	}
	return points
}

func heatmapCells(buckets map[[2]int]*heatmapHistoryAggregate) []model.HistoryHeatmapCell {
	cells := make([]model.HistoryHeatmapCell, 0, 7*24)
	for weekday := 1; weekday <= 7; weekday++ {
		for hour := 0; hour < 24; hour++ {
			bucket := buckets[[2]int{weekday, hour}]
			if bucket == nil {
				bucket = &heatmapHistoryAggregate{dates: map[string]struct{}{}}
			}
			available := bucket.durations.idle + bucket.durations.inUse
			var occupancy *float64
			if available > 0 {
				value := roundOneDecimal(float64(bucket.durations.inUse) / float64(available) * 100)
				occupancy = &value
			}
			cells = append(cells, model.HistoryHeatmapCell{
				Weekday: weekday, Hour: hour,
				IdleSeconds: bucket.durations.idle, InUseSeconds: bucket.durations.inUse,
				OfflineSeconds: bucket.durations.offline, OccupancyPercent: occupancy,
				SampleDates:      len(bucket.dates),
				SampleSufficient: len(bucket.dates) >= 3 && available >= quietSampleSeconds,
			})
		}
	}
	return cells
}

func heatmapInsights(cells []model.HistoryHeatmapCell) ([]model.HistoryHourInsight, *model.HistoryHourInsight) {
	eligible := make([]model.HistoryHourInsight, 0)
	for _, cell := range cells {
		if !cell.SampleSufficient || cell.OccupancyPercent == nil {
			continue
		}
		eligible = append(eligible, model.HistoryHourInsight{
			Weekday: cell.Weekday, Hour: cell.Hour,
			OccupancyPercent: *cell.OccupancyPercent, SampleDates: cell.SampleDates,
		})
	}
	if len(eligible) == 0 {
		return []model.HistoryHourInsight{}, nil
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].OccupancyPercent == eligible[j].OccupancyPercent {
			if eligible[i].Weekday == eligible[j].Weekday {
				return eligible[i].Hour < eligible[j].Hour
			}
			return eligible[i].Weekday < eligible[j].Weekday
		}
		return eligible[i].OccupancyPercent > eligible[j].OccupancyPercent
	})
	busiestCount := 3
	if len(eligible) < busiestCount {
		busiestCount = len(eligible)
	}
	busiest := append([]model.HistoryHourInsight(nil), eligible[:busiestCount]...)
	quiet := eligible[len(eligible)-1]
	return busiest, &quiet
}

func currentPilePort(pile model.Pile, portID int) (model.PortStatus, bool) {
	for _, port := range pile.Ports {
		if port.ID == portID {
			return port.Status, true
		}
	}
	return "", false
}

func optionalTime(values map[int]time.Time, key int) *time.Time {
	value, ok := values[key]
	if !ok {
		return nil
	}
	copy := value
	return &copy
}

func reverseTimeline(items []model.PortHistoryTimelineItem) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func isoWeekday(day time.Weekday) int {
	if day == time.Sunday {
		return 7
	}
	return int(day)
}

func roundOneDecimal(value float64) float64 {
	return math.Round(value*10) / 10
}
