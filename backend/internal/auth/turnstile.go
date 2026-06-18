package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

type TurnstileVerifier struct {
	siteKey  string
	secret   string
	hostname string
	client   *http.Client
}

type turnstileResponse struct {
	Success    bool     `json:"success"`
	Hostname   string   `json:"hostname"`
	Action     string   `json:"action"`
	ErrorCodes []string `json:"error-codes"`
}

func NewTurnstileVerifier(siteKey string, secret string, hostname string) *TurnstileVerifier {
	return &TurnstileVerifier{
		siteKey:  strings.TrimSpace(siteKey),
		secret:   strings.TrimSpace(secret),
		hostname: strings.TrimSpace(hostname),
		client:   &http.Client{Timeout: 8 * time.Second},
	}
}

func (v *TurnstileVerifier) Enabled() bool {
	return v != nil && v.siteKey != "" && v.secret != ""
}

func (v *TurnstileVerifier) SiteKey() string {
	if v == nil {
		return ""
	}
	return v.siteKey
}

func (v *TurnstileVerifier) Verify(ctx context.Context, token string, remoteIP string, expectedAction string) error {
	if !v.Enabled() {
		return nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("请完成人机验证")
	}
	if len(token) > 2048 {
		return fmt.Errorf("人机验证 token 无效")
	}

	form := url.Values{
		"secret":   {v.secret},
		"response": {token},
	}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create turnstile request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("人机验证服务暂时不可用")
	}
	defer resp.Body.Close()

	var result turnstileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析人机验证结果失败")
	}
	if !result.Success {
		return fmt.Errorf("人机验证失败，请重试")
	}
	if !v.usesTestSecret() && v.hostname != "" && !strings.EqualFold(result.Hostname, v.hostname) {
		return fmt.Errorf("人机验证来源不匹配")
	}
	if !v.usesTestSecret() && expectedAction != "" && result.Action != "" && result.Action != expectedAction {
		return fmt.Errorf("人机验证操作不匹配")
	}
	return nil
}

func (v *TurnstileVerifier) usesTestSecret() bool {
	switch v.secret {
	case "1x0000000000000000000000000000000AA",
		"2x0000000000000000000000000000000AA",
		"3x0000000000000000000000000000000AA":
		return true
	default:
		return false
	}
}
