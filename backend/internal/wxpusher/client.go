package wxpusher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultBaseURL       = "https://wxpusher.zjiecode.com"
	DefaultTimeout       = 10 * time.Second
	MaxResponseBytes     = 256 * 1024
	MaxQRCodeExtraLength = 64
	MaxQRCodeValidity    = 30 * 24 * time.Hour
	MaxSummaryLength     = 100
	MaxContentLength     = 40000
	MaxURLLength         = 400
)

type ContentType int

const (
	ContentTypeText     ContentType = 1
	ContentTypeHTML     ContentType = 2
	ContentTypeMarkdown ContentType = 3
)

type Config struct {
	AppToken       string
	BaseURL        string
	HTTPClient     *http.Client
	RequestTimeout time.Duration
}

type Client struct {
	appToken string
	baseURL  string
	http     *http.Client
	timeout  time.Duration
}

type QRCode struct {
	Code      string
	URL       string
	ShortURL  string
	Extra     string
	ExpiresAt time.Time
}

type ScanResult struct {
	UID     string
	Scanned bool
}

type Message struct {
	UID         string
	Summary     string
	Content     string
	ContentType ContentType
	URL         string
}

type SendResult struct {
	SendRecordID     string
	MessageContentID string
	ProviderCode     int
	Status           string
}

type MessageStatus struct {
	SendRecordID string
	ProviderCode int
	Status       string
	Succeeded    bool
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"msg"`
	Success *bool           `json:"success"`
	Data    json.RawMessage `json:"data"`
}

type createQRCodeRequest struct {
	AppToken  string `json:"appToken"`
	Extra     string `json:"extra"`
	ValidTime int    `json:"validTime"`
}

type createQRCodeData struct {
	Code     string `json:"code"`
	URL      string `json:"url"`
	ShortURL string `json:"shortUrl"`
	Extra    string `json:"extra"`
	Expires  int64  `json:"expires"`
}

type sendMessageRequest struct {
	AppToken      string      `json:"appToken"`
	Content       string      `json:"content"`
	Summary       string      `json:"summary,omitempty"`
	ContentType   ContentType `json:"contentType"`
	UIDs          []string    `json:"uids"`
	URL           string      `json:"url,omitempty"`
	VerifyPayType int         `json:"verifyPayType"`
}

type sendMessageData struct {
	UID              string          `json:"uid"`
	MessageContentID json.RawMessage `json:"messageContentId"`
	SendRecordID     json.RawMessage `json:"sendRecordId"`
	Code             int             `json:"code"`
	Status           string          `json:"status"`
}

type messageStatusData struct {
	SendRecordID json.RawMessage `json:"sendRecordId"`
	Code         int             `json:"code"`
	Status       string          `json:"status"`
	Success      *bool           `json:"success"`
}

func NewClient(cfg Config) (*Client, error) {
	appToken := strings.TrimSpace(cfg.AppToken)
	if appToken == "" {
		return nil, newClientError(ErrorUnconfigured, "configure", false, 0, 0, nil)
	}
	baseURL, err := validateBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, newClientError(ErrorInvalidRequest, "configure", false, 0, 0, err)
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	httpClientCopy := *httpClient
	// Provider requests carry credentials in their body. Never forward them to
	// another origin through an unexpected redirect.
	httpClientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Client{appToken: appToken, baseURL: baseURL, http: &httpClientCopy, timeout: timeout}, nil
}

func (c *Client) CreateQRCode(ctx context.Context, extra string, validFor time.Duration) (QRCode, error) {
	extra = strings.TrimSpace(extra)
	if extra == "" || utf8.RuneCountInString(extra) > MaxQRCodeExtraLength {
		return QRCode{}, newClientError(ErrorInvalidRequest, "create_qr_code", false, 0, 0, nil)
	}
	if validFor == 0 {
		validFor = 30 * time.Minute
	}
	if validFor < time.Second || validFor > MaxQRCodeValidity {
		return QRCode{}, newClientError(ErrorInvalidRequest, "create_qr_code", false, 0, 0, nil)
	}
	var data createQRCodeData
	err := c.doJSON(ctx, http.MethodPost, "/api/fun/create/qrcode", createQRCodeRequest{
		AppToken:  c.appToken,
		Extra:     extra,
		ValidTime: int(validFor / time.Second),
	}, &data, "create_qr_code")
	if err != nil {
		return QRCode{}, err
	}
	if strings.TrimSpace(data.Code) == "" || strings.TrimSpace(data.URL) == "" || data.Expires <= 0 {
		return QRCode{}, newClientError(ErrorInvalidResponse, "create_qr_code", false, 0, 0, nil)
	}
	expiresAt, err := providerExpiry(data.Expires)
	if err != nil {
		return QRCode{}, newClientError(ErrorInvalidResponse, "create_qr_code", false, 0, 0, err)
	}
	return QRCode{
		Code:      data.Code,
		URL:       data.URL,
		ShortURL:  data.ShortURL,
		Extra:     data.Extra,
		ExpiresAt: expiresAt,
	}, nil
}

func (c *Client) QueryScanUID(ctx context.Context, code string) (ScanResult, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return ScanResult{}, newClientError(ErrorInvalidRequest, "query_scan_uid", false, 0, 0, nil)
	}
	query := url.Values{"code": []string{code}}
	var data json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, "/api/fun/scan-qrcode-uid?"+query.Encode(), nil, &data, "query_scan_uid"); err != nil {
		return ScanResult{}, err
	}
	uid, err := scanUID(data)
	if err != nil {
		return ScanResult{}, newClientError(ErrorInvalidResponse, "query_scan_uid", false, 0, 0, err)
	}
	return ScanResult{UID: uid, Scanned: uid != ""}, nil
}

func (c *Client) Send(ctx context.Context, message Message) (SendResult, error) {
	message.UID = strings.TrimSpace(message.UID)
	message.Summary = strings.TrimSpace(message.Summary)
	message.Content = strings.TrimSpace(message.Content)
	message.URL = strings.TrimSpace(message.URL)
	if err := validateMessage(message); err != nil {
		return SendResult{}, err
	}
	var data []sendMessageData
	err := c.doJSON(ctx, http.MethodPost, "/api/send/message", sendMessageRequest{
		AppToken:      c.appToken,
		Content:       message.Content,
		Summary:       message.Summary,
		ContentType:   message.ContentType,
		UIDs:          []string{message.UID},
		URL:           message.URL,
		VerifyPayType: 0,
	}, &data, "send_message")
	if err != nil {
		return SendResult{}, err
	}
	if len(data) != 1 {
		return SendResult{}, newClientError(ErrorInvalidResponse, "send_message", false, 0, 0, nil)
	}
	item := data[0]
	if item.Code != 1000 {
		return SendResult{}, classifyBusinessError("send_message", item.Code, item.Status)
	}
	recordID, err := providerID(item.SendRecordID)
	if err != nil || recordID == "" {
		return SendResult{}, newClientError(ErrorInvalidResponse, "send_message", false, 0, 0, err)
	}
	contentID, err := providerID(item.MessageContentID)
	if err != nil || contentID == "" {
		return SendResult{}, newClientError(ErrorInvalidResponse, "send_message", false, 0, 0, err)
	}
	return SendResult{
		SendRecordID:     recordID,
		MessageContentID: contentID,
		ProviderCode:     item.Code,
		Status:           item.Status,
	}, nil
}

func (c *Client) QueryMessageStatus(ctx context.Context, sendRecordID string) (MessageStatus, error) {
	sendRecordID = strings.TrimSpace(sendRecordID)
	if sendRecordID == "" {
		return MessageStatus{}, newClientError(ErrorInvalidRequest, "query_message_status", false, 0, 0, nil)
	}
	query := url.Values{"sendRecordId": []string{sendRecordID}}
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, "/api/send/query/status?"+query.Encode(), nil, &raw, "query_message_status"); err != nil {
		return MessageStatus{}, err
	}
	status, err := parseMessageStatus(raw, sendRecordID)
	if err != nil {
		return MessageStatus{}, newClientError(ErrorInvalidResponse, "query_message_status", false, 0, 0, err)
	}
	return status, nil
}

func validateMessage(message Message) error {
	if message.UID == "" || message.Content == "" {
		return newClientError(ErrorInvalidRequest, "send_message", false, 0, 0, nil)
	}
	if message.ContentType != ContentTypeText && message.ContentType != ContentTypeHTML && message.ContentType != ContentTypeMarkdown {
		return newClientError(ErrorInvalidRequest, "send_message", false, 0, 0, nil)
	}
	if utf8.RuneCountInString(message.Summary) > MaxSummaryLength ||
		utf8.RuneCountInString(message.Content) > MaxContentLength ||
		utf8.RuneCountInString(message.URL) > MaxURLLength {
		return newClientError(ErrorInvalidRequest, "send_message", false, 0, 0, nil)
	}
	if message.URL != "" {
		parsed, err := url.ParseRequestURI(message.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return newClientError(ErrorInvalidRequest, "send_message", false, 0, 0, nil)
		}
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, requestBody, responseData any, operation string) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return newClientError(ErrorInvalidRequest, operation, false, 0, 0, err)
		}
		body = bytes.NewReader(encoded)
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, method, c.baseURL+path, body)
	if err != nil {
		return newClientError(ErrorInvalidRequest, operation, false, 0, 0, err)
	}
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return newClientError(ErrorProviderTimeout, operation, true, 0, 0, err)
		}
		return newClientError(ErrorProviderUnavailable, operation, true, 0, 0, err)
	}
	defer resp.Body.Close()
	responseBody, err := readLimited(resp.Body)
	if err != nil {
		return newClientError(ErrorInvalidResponse, operation, false, resp.StatusCode, 0, err)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return newClientError(ErrorRateLimited, operation, true, resp.StatusCode, 0, nil)
	}
	if resp.StatusCode >= 500 {
		return newClientError(ErrorProviderUnavailable, operation, true, resp.StatusCode, 0, nil)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newClientError(ErrorProviderRejected, operation, false, resp.StatusCode, 0, nil)
	}
	var env envelope
	if err := json.Unmarshal(responseBody, &env); err != nil {
		return newClientError(ErrorInvalidResponse, operation, false, resp.StatusCode, 0, err)
	}
	if env.Code != 1000 {
		return classifyBusinessError(operation, env.Code, env.Message)
	}
	if env.Success != nil && !*env.Success {
		return newClientError(ErrorProviderRejected, operation, false, resp.StatusCode, env.Code, nil)
	}
	if responseData == nil {
		return nil
	}
	if raw, ok := responseData.(*json.RawMessage); ok {
		*raw = append((*raw)[:0], env.Data...)
		return nil
	}
	if len(env.Data) == 0 || bytes.Equal(env.Data, []byte("null")) {
		return newClientError(ErrorInvalidResponse, operation, false, resp.StatusCode, env.Code, nil)
	}
	if err := json.Unmarshal(env.Data, responseData); err != nil {
		return newClientError(ErrorInvalidResponse, operation, false, resp.StatusCode, env.Code, err)
	}
	return nil
}

func readLimited(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, MaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxResponseBytes {
		return nil, fmt.Errorf("response exceeds limit")
	}
	return body, nil
}

func validateBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = DefaultBaseURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid wxpusher base URL")
	}
	if parsed.Scheme != "https" {
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if parsed.Scheme != "http" || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return "", fmt.Errorf("wxpusher base URL must use HTTPS or loopback HTTP")
		}
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func scanUID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	var uid string
	if err := json.Unmarshal(raw, &uid); err == nil {
		return strings.TrimSpace(uid), nil
	}
	var data struct {
		UID string `json:"uid"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", err
	}
	return strings.TrimSpace(data.UID), nil
}

func providerID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text), nil
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return "", err
	}
	return number.String(), nil
}

func providerExpiry(value int64) (time.Time, error) {
	if value <= 0 {
		return time.Time{}, fmt.Errorf("missing expiry")
	}
	// Current responses use a Unix millisecond timestamp. Accept Unix seconds
	// as well so a provider-side serialization change cannot create a date in 1970.
	if value < 100_000_000_000 {
		return time.Unix(value, 0), nil
	}
	return time.UnixMilli(value), nil
}

func parseMessageStatus(raw json.RawMessage, requestedID string) (MessageStatus, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return MessageStatus{}, fmt.Errorf("missing status data")
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return MessageStatus{}, fmt.Errorf("missing status")
		}
		return MessageStatus{SendRecordID: requestedID, Status: text}, nil
	}
	var data messageStatusData
	if err := json.Unmarshal(raw, &data); err != nil {
		return MessageStatus{}, err
	}
	recordID, err := providerID(data.SendRecordID)
	if err != nil {
		return MessageStatus{}, err
	}
	if recordID == "" {
		recordID = requestedID
	}
	if strings.TrimSpace(data.Status) == "" && data.Success == nil && data.Code == 0 {
		return MessageStatus{}, fmt.Errorf("missing status")
	}
	succeeded := data.Code == 1000
	if data.Success != nil {
		succeeded = *data.Success
	}
	return MessageStatus{
		SendRecordID: recordID,
		ProviderCode: data.Code,
		Status:       data.Status,
		Succeeded:    succeeded,
	}, nil
}
