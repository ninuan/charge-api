package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	hcaptchaVerifyURL     = "https://api.hcaptcha.com/siteverify"
	hcaptchaTokenMaxBytes = 8192
	hcaptchaBodyMaxBytes  = 64 * 1024
)

type HCaptchaVerifier struct {
	siteKey   string
	secret    string
	verifyURL string
	client    *http.Client
}

type hcaptchaResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

func NewHCaptchaVerifier(siteKey string, secret string) *HCaptchaVerifier {
	return newHCaptchaVerifier(siteKey, secret, hcaptchaVerifyURL, &http.Client{Timeout: 8 * time.Second})
}

func newHCaptchaVerifier(siteKey string, secret string, verifyURL string, client *http.Client) *HCaptchaVerifier {
	return &HCaptchaVerifier{
		siteKey:   strings.TrimSpace(siteKey),
		secret:    strings.TrimSpace(secret),
		verifyURL: strings.TrimSpace(verifyURL),
		client:    client,
	}
}

func (v *HCaptchaVerifier) Enabled() bool {
	return v != nil && v.siteKey != "" && v.secret != ""
}

func (v *HCaptchaVerifier) SiteKey() string {
	if v == nil {
		return ""
	}
	return v.siteKey
}

func (v *HCaptchaVerifier) Verify(ctx context.Context, token string, remoteIP string) error {
	if !v.Enabled() {
		return nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("请完成人机验证")
	}
	if len(token) > hcaptchaTokenMaxBytes {
		return fmt.Errorf("人机验证 token 无效")
	}
	if v.client == nil || v.verifyURL == "" {
		return fmt.Errorf("人机验证服务未正确配置")
	}

	form := url.Values{
		"secret":   {v.secret},
		"response": {token},
		"sitekey":  {v.siteKey},
	}
	if remoteIP = strings.TrimSpace(remoteIP); remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create hcaptcha request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("人机验证服务暂时不可用")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("人机验证服务暂时不可用")
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, hcaptchaBodyMaxBytes+1))
	if err != nil {
		return fmt.Errorf("读取人机验证结果失败")
	}
	if len(body) > hcaptchaBodyMaxBytes {
		return fmt.Errorf("人机验证结果过大")
	}
	var result hcaptchaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析人机验证结果失败")
	}
	if !result.Success {
		return fmt.Errorf("人机验证失败，请重试")
	}
	return nil
}
