package charger

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
)

var ErrAuthExpired = errors.New("cookie expired or unauthorized")

type Client struct {
	httpClient *http.Client
	mu         sync.RWMutex
	template   parser.CaptureRequest
	requests   map[string]parser.CaptureRequest
	order      []string
}

func NewClient(requests []parser.CaptureRequest) *Client {
	return newClient(requests, true)
}

func NewClientTemplateOnly(requests []parser.CaptureRequest) *Client {
	return newClient(requests, false)
}

func newClient(requests []parser.CaptureRequest, preload bool) *Client {
	requestMap := make(map[string]parser.CaptureRequest, len(requests))
	order := make([]string, 0, len(requests))
	if preload {
		for _, request := range requests {
			id := requestDeviceID(request)
			if id == "" {
				id = request.Name
			}
			requestMap[id] = request
			order = append(order, id)
		}
	}

	var template parser.CaptureRequest
	if len(requests) > 0 {
		template = requests[0]
		if !preload {
			template.Headers = cloneHeaders(template.Headers)
			template.Headers["Cookie"] = ""
		}
	}

	return &Client{
		httpClient: &http.Client{Timeout: 12 * time.Second},
		template:   template,
		requests:   requestMap,
		order:      order,
	}
}

func (c *Client) FetchPiles() ([]model.Pile, error) {
	requests := c.snapshotRequests()

	piles := make([]model.Pile, 0, len(requests))
	for _, captureRequest := range requests {
		pile, err := c.fetchPile(captureRequest)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", captureRequest.Name, err)
		}
		piles = append(piles, pile)
	}
	return piles, nil
}

func (c *Client) AddDevice(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("device id is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	request := c.template
	request.Name = id
	request.Body = withFormValue(request.Body, "id", id)
	request.Headers = cloneHeaders(request.Headers)
	request.Headers["Referer"] = withURLQueryValue(request.Headers["Referer"], "id", id)
	request.Headers["Cookie"] = withCookieValue(request.Headers["Cookie"], "deviceid", id)

	if _, exists := c.requests[id]; !exists {
		c.order = append(c.order, id)
	}
	c.requests[id] = request
	return nil
}

func (c *Client) RestoreDevices(ids []string) {
	for _, id := range ids {
		_ = c.AddDevice(id)
	}
}

func (c *Client) DeviceIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ids := make([]string, 0, len(c.order))
	for _, id := range c.order {
		if _, ok := c.requests[id]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func (c *Client) RemoveDevice(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.requests, id)
	nextOrder := c.order[:0]
	for _, existingID := range c.order {
		if existingID != id {
			nextOrder = append(nextOrder, existingID)
		}
	}
	c.order = nextOrder
}

func (c *Client) UpdateCookie(cookie string) error {
	cookie = strings.TrimSpace(cookie)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.template.Headers = cloneHeaders(c.template.Headers)
	c.template.Headers["Cookie"] = withCookieValue(cookie, "deviceid", requestDeviceID(c.template))

	for id, request := range c.requests {
		request.Headers = cloneHeaders(request.Headers)
		request.Headers["Cookie"] = withCookieValue(cookie, "deviceid", id)
		c.requests[id] = request
	}

	return nil
}

func (c *Client) Cookie() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.template.Headers["Cookie"]
}

func IsAuthExpired(err error) bool {
	return errors.Is(err, ErrAuthExpired)
}

func (c *Client) fetchPile(captureRequest parser.CaptureRequest) (model.Pile, error) {
	req, err := http.NewRequest(
		captureRequest.Method,
		captureRequest.URL,
		strings.NewReader(captureRequest.Body),
	)
	if err != nil {
		return model.Pile{}, err
	}

	for key, value := range captureRequest.Headers {
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return model.Pile{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return model.Pile{}, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return model.Pile{}, fmt.Errorf("%w: remote API returned %s", ErrAuthExpired, resp.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return model.Pile{}, fmt.Errorf("remote API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	pile, err := parser.ParsePayload(captureRequest.URL, body)
	if err != nil && looksLikeAuthExpired(body) {
		return model.Pile{}, fmt.Errorf("%w: %s", ErrAuthExpired, err)
	}
	return pile, err
}

func (c *Client) snapshotRequests() []parser.CaptureRequest {
	c.mu.RLock()
	defer c.mu.RUnlock()

	requests := make([]parser.CaptureRequest, 0, len(c.order))
	for _, id := range c.order {
		if request, ok := c.requests[id]; ok {
			requests = append(requests, request)
		}
	}
	return requests
}

func requestDeviceID(request parser.CaptureRequest) string {
	values, err := url.ParseQuery(request.Body)
	if err != nil {
		return ""
	}
	return values.Get("id")
}

func withFormValue(raw string, key string, value string) string {
	values, err := url.ParseQuery(raw)
	if err != nil {
		values = url.Values{}
	}
	values.Set(key, value)
	return values.Encode()
}

func withURLQueryValue(raw string, key string, value string) string {
	if raw == "" {
		return raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func withCookieValue(raw string, key string, value string) string {
	if raw == "" {
		return raw
	}

	parts := strings.Split(raw, ";")
	found := false
	for i, part := range parts {
		name, _, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && name == key {
			parts[i] = " " + key + "=" + value
			found = true
		}
	}
	if !found {
		parts = append(parts, " "+key+"="+value)
	}
	return strings.TrimSpace(strings.Join(parts, ";"))
}

func cloneHeaders(headers map[string]string) map[string]string {
	clone := make(map[string]string, len(headers))
	for key, value := range headers {
		clone[key] = value
	}
	return clone
}

func looksLikeAuthExpired(body []byte) bool {
	text := strings.ToLower(string(body))
	keywords := []string{
		"cookie",
		"sid",
		"unauthorized",
		"forbidden",
		"login",
		"登录",
		"未登录",
		"授权",
		"过期",
	}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}
