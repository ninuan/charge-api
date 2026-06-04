package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charge-dashboard/internal/model"
)

type rawPayload struct {
	ID      string          `json:"id"`
	Number  string          `json:"number"`
	Name    string          `json:"name"`
	Status  string          `json:"status"`
	Address string          `json:"address"`
	OpenNum int             `json:"opennum"`
	Used    []int           `json:"used"`
	Useds   []rawUsedStatus `json:"useds"`
}

type rawUsedStatus struct {
	Index         int    `json:"i"`
	UsedSeconds   int    `json:"u"`
	RemainingText string `json:"s"`
}

type CaptureRequest struct {
	Name    string
	URL     string
	Method  string
	Body    string
	Headers map[string]string
}

func DefaultCaptureRequests() []CaptureRequest {
	return []CaptureRequest{
		{
			Name:   "default",
			URL:    "https://ele.mocele.com/action/i/api/devicewithnumbers",
			Method: "POST",
			Body:   "id=YOUR_DEVICE_LONG_ID",
			Headers: map[string]string{
				"Accept":           "application/json",
				"Content-Type":     "application/x-www-form-urlencoded",
				"Origin":           "https://ele.mocele.com",
				"Referer":          "https://ele.mocele.com/i/device/open?id=YOUR_DEVICE_LONG_ID",
				"User-Agent":       "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko) Mobile Safari/537.36",
				"X-Requested-With": "XMLHttpRequest",
			},
		},
	}
}

func ParseCaptureRequests(dir string) ([]CaptureRequest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read capture dir: %w", err)
	}

	var requests []CaptureRequest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		request, parseErr := parseCaptureRequest(filepath.Join(dir, entry.Name()), entry.Name())
		if parseErr != nil {
			continue
		}
		requests = append(requests, request)
	}

	if len(requests) == 0 {
		return nil, fmt.Errorf("no valid request found in %s", dir)
	}

	return requests, nil
}

func ParsePayload(source string, body []byte) (model.Pile, error) {
	var raw rawPayload
	if err := json.Unmarshal(body, &raw); err != nil {
		return model.Pile{}, err
	}

	if raw.ID == "" {
		return model.Pile{}, fmt.Errorf("远端接口没有返回设备ID，请输入设备长ID（例如 2601201412385560001），不要输入短桩号；响应：%s", compactPayload(body))
	}

	now := time.Now()
	openNum := raw.OpenNum
	if openNum <= 0 {
		openNum = 10
	}

	usedMap := make(map[int]bool, len(raw.Used))
	for _, u := range raw.Used {
		usedMap[u] = true
	}
	usedStatusMap := make(map[int]rawUsedStatus, len(raw.Useds))
	for _, item := range raw.Useds {
		if item.Index <= 0 {
			continue
		}
		usedMap[item.Index] = true
		usedStatusMap[item.Index] = item
	}

	ports := make([]model.Port, 0, openNum)
	for i := 1; i <= openNum; i++ {
		status := model.PortIdle
		power := 0.0
		energy := 0.0
		var start *time.Time
		minutes := 0
		usedSeconds := 0
		usedText := ""
		remainingText := ""

		if usedMap[i] {
			status = model.PortInUse
			power = 0.45
			energy = 0.12
			if usedStatus, ok := usedStatusMap[i]; ok {
				usedSeconds = usedStatus.UsedSeconds
				usedText = formatDuration(usedSeconds)
				remainingText = strings.TrimSpace(usedStatus.RemainingText)
			}
			if usedSeconds > 0 {
				minutes = usedSeconds / 60
			} else {
				minutes = 15
				usedSeconds = minutes * 60
				usedText = formatDuration(usedSeconds)
			}
			startedAt := now.Add(-time.Duration(usedSeconds) * time.Second)
			start = &startedAt
		}

		if !strings.Contains(raw.Status, "在线") {
			status = model.PortOffline
			power = 0
			energy = 0
			start = nil
			minutes = 0
			usedSeconds = 0
			usedText = ""
			remainingText = ""
		}

		ports = append(ports, model.Port{
			ID:            i,
			Status:        status,
			PowerKW:       power,
			EnergyKWh:     energy,
			UpdatedAt:     now,
			StartedAt:     start,
			SessionMin:    minutes,
			UsedSeconds:   usedSeconds,
			UsedText:      usedText,
			RemainingText: remainingText,
		})
	}

	usedPortIDs := make([]int, 0, len(usedMap))
	for portID, used := range usedMap {
		if used {
			usedPortIDs = append(usedPortIDs, portID)
		}
	}
	sort.Ints(usedPortIDs)

	return model.Pile{
		ID:          raw.ID,
		Number:      raw.Number,
		Name:        raw.Name,
		Status:      raw.Status,
		Address:     raw.Address,
		OpenNum:     openNum,
		Online:      strings.Contains(raw.Status, "在线"),
		CreatedAt:   now,
		UpdatedAt:   now,
		Source:      source,
		Ports:       ports,
		UsedPortIDs: usedPortIDs,
	}, nil
}

func compactPayload(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 180 {
		return text[:180] + "..."
	}
	return text
}

func formatDuration(seconds int) string {
	if seconds <= 0 {
		return ""
	}

	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	remainingSeconds := seconds % 60

	switch {
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%d小时%d分钟", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%d小时", hours)
	case minutes > 0:
		return fmt.Sprintf("%d分钟", minutes)
	default:
		return fmt.Sprintf("%d秒", remainingSeconds)
	}
}

func ParseCaptureDir(dir string) ([]model.Pile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read capture dir: %w", err)
	}

	var piles []model.Pile
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		payloadPath := filepath.Join(dir, entry.Name(), "response_body")
		body, readErr := os.ReadFile(payloadPath)
		if readErr != nil {
			continue
		}

		pile, parseErr := ParsePayload(payloadPath, body)
		if parseErr == nil {
			piles = append(piles, pile)
		}
	}

	if len(piles) == 0 {
		return nil, fmt.Errorf("no valid payload found in %s", dir)
	}

	return piles, nil
}

func parseCaptureRequest(dir string, name string) (CaptureRequest, error) {
	urlBytes, err := os.ReadFile(filepath.Join(dir, "basic"))
	if err != nil {
		return CaptureRequest{}, err
	}

	bodyBytes, err := os.ReadFile(filepath.Join(dir, "request_body"))
	if err != nil {
		return CaptureRequest{}, err
	}

	headerBytes, err := os.ReadFile(filepath.Join(dir, "request_headers"))
	if err != nil {
		return CaptureRequest{}, err
	}

	method, headers := parseRequestHeaders(string(headerBytes))
	if method == "" {
		method = "POST"
	}

	return CaptureRequest{
		Name:    name,
		URL:     strings.TrimSpace(string(urlBytes)),
		Method:  method,
		Body:    string(bodyBytes),
		Headers: headers,
	}, nil
}

func parseRequestHeaders(raw string) (string, map[string]string) {
	headers := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	method := ""

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if i == 0 {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				method = parts[0]
			}
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if shouldForwardHeader(key) {
			headers[key] = strings.TrimSpace(value)
		}
	}

	return method, headers
}

func shouldForwardHeader(key string) bool {
	switch strings.ToLower(key) {
	case "accept", "accept-language", "content-type", "cookie", "origin", "referer", "user-agent", "x-requested-with":
		return true
	default:
		return false
	}
}
