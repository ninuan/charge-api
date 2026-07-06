package yyb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"charge-dashboard/internal/securelink"
	"charge-dashboard/internal/security"
)

type Config struct {
	BaseURL    string
	APISecret  []byte
	HTTPClient *http.Client
}

type Client struct {
	baseURL string
	signer  *securelink.Signer
	http    *http.Client
}

type QRSession struct {
	SessionID   string `json:"session_id"`
	Status      string `json:"status"`
	ImageURL    string `json:"image_url"`
	ImageBase64 string `json:"image_base64,omitempty"`
}

type QRPollResult struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

type YYBAccount struct {
	ID       string `json:"id,omitempty"`
	Ref      string `json:"ref"`
	OpenID   string `json:"openid"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Status   string `json:"status"`
}

func (a *YYBAccount) UnmarshalJSON(data []byte) error {
	type yybAccountAlias YYBAccount
	var raw struct {
		yybAccountAlias
		ID any `json:"id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*a = YYBAccount(raw.yybAccountAlias)
	switch id := raw.ID.(type) {
	case nil:
	case string:
		a.ID = id
	case float64:
		a.ID = fmt.Sprintf("%.0f", id)
	default:
		a.ID = fmt.Sprint(id)
	}
	return nil
}

type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type wxappCodeData struct {
	Code   string `json:"code"`
	Result struct {
		Code string `json:"code"`
	} `json:"result"`
}

func NewClient(cfg Config) (*Client, error) {
	base, err := securelink.LoopbackBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	signer, err := securelink.NewSigner(cfg.APISecret)
	if err != nil {
		return nil, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(base.String(), "/"), signer: signer, http: httpClient}, nil
}

func (c *Client) CreateQR(ctx context.Context) (QRSession, error) {
	var out QRSession
	if err := c.doJSON(ctx, http.MethodPost, "/qr?as_base64=true", nil, &out); err != nil {
		return QRSession{}, err
	}
	if out.SessionID == "" {
		return QRSession{}, fmt.Errorf("yyb create qr response missing session_id")
	}
	return out, nil
}

func (c *Client) PollQR(ctx context.Context, sessionID string) (QRPollResult, error) {
	var out QRPollResult
	if err := c.doJSON(ctx, http.MethodGet, "/qr/"+sessionID+"/poll", nil, &out); err != nil {
		return QRPollResult{}, err
	}
	return out, nil
}

func (c *Client) ConfirmQR(ctx context.Context, sessionID string) (YYBAccount, error) {
	var out YYBAccount
	if err := c.doJSON(ctx, http.MethodPost, "/qr/"+sessionID+"/confirm", nil, &out); err != nil {
		return YYBAccount{}, err
	}
	if out.Ref == "" {
		out.Ref = strings.TrimSpace(out.ID)
	}
	if out.Ref == "" {
		out.Ref = strings.TrimSpace(out.OpenID)
	}
	if out.Ref == "" {
		return YYBAccount{}, fmt.Errorf("yyb confirm response missing ref")
	}
	return out, nil
}

func (c *Client) GetCode(ctx context.Context, ref string, appID string) (string, error) {
	var out wxappCodeData
	if err := c.doJSON(ctx, http.MethodPost, "/wxapp/getCode", map[string]any{"ref": ref, "app_id": appID}, &out); err != nil {
		return "", err
	}
	code := out.Result.Code
	if code == "" {
		code = out.Code
	}
	if code == "" {
		return "", fmt.Errorf("yyb getCode response missing code")
	}
	return code, nil
}

func (c *Client) RefreshAccount(ctx context.Context, ref string) error {
	var out json.RawMessage
	return c.doJSON(ctx, http.MethodPost, "/accounts/refresh", map[string]any{"ref": ref}, &out)
}

func (c *Client) doJSON(ctx context.Context, method string, path string, payload any, out any) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	} else {
		body = []byte("{}")
	}
	reqBody := bytes.NewReader(body)
	if method == http.MethodGet {
		reqBody = bytes.NewReader(nil)
		body = nil
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return err
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.signer.Sign(req, body); err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("yyb request failed: status=%d body=%s", resp.StatusCode, security.RedactText(string(respBody), 512))
	}
	if len(respBody) == 0 || out == nil {
		return nil
	}
	payloadBody, err := unwrapEnvelope(respBody)
	if err != nil {
		return err
	}
	if len(payloadBody) == 0 || string(payloadBody) == "null" {
		return nil
	}
	if raw, ok := out.(*json.RawMessage); ok {
		*raw = append((*raw)[:0], payloadBody...)
		return nil
	}
	if err := json.Unmarshal(payloadBody, out); err != nil {
		return fmt.Errorf("decode yyb response: %w", err)
	}
	return nil
}

func unwrapEnvelope(body []byte) ([]byte, error) {
	var env envelope
	if err := json.Unmarshal(body, &env); err == nil && env.Data != nil {
		if env.Code != 0 {
			return nil, fmt.Errorf("yyb request rejected: code=%d msg=<redacted>", env.Code)
		}
		return env.Data, nil
	}
	return body, nil
}
