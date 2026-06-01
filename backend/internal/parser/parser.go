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
	ID      string `json:"id"`
	Number  string `json:"number"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Address string `json:"address"`
	OpenNum int    `json:"opennum"`
	Used    []int  `json:"used"`
}

type CaptureRequest struct {
	Name    string
	URL     string
	Method  string
	Body    string
	Headers map[string]string
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

	ports := make([]model.Port, 0, openNum)
	for i := 1; i <= openNum; i++ {
		status := model.PortIdle
		power := 0.0
		energy := 0.0
		var start *time.Time
		minutes := 0

		if usedMap[i] {
			status = model.PortInUse
			power = 0.45
			energy = 0.12
			startedAt := now.Add(-15 * time.Minute)
			start = &startedAt
			minutes = 15
		}

		if !strings.Contains(raw.Status, "在线") {
			status = model.PortOffline
			power = 0
			energy = 0
			start = nil
			minutes = 0
		}

		ports = append(ports, model.Port{
			ID:         i,
			Status:     status,
			PowerKW:    power,
			EnergyKWh:  energy,
			UpdatedAt:  now,
			StartedAt:  start,
			SessionMin: minutes,
		})
	}

	sort.Ints(raw.Used)
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
		UsedPortIDs: raw.Used,
	}, nil
}

func compactPayload(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 180 {
		return text[:180] + "..."
	}
	return text
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
