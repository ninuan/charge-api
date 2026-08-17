package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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

type hcaptchaVerificationError struct {
	unavailable bool
	reason      string
}

func (e *hcaptchaVerificationError) Error() string {
	return "hcaptcha verification failed: " + e.reason
}

// IsHCaptchaUnavailable reports whether verification failed because the
// provider or its server-side configuration is unavailable. Callers should
// not count these failures against the user.
func IsHCaptchaUnavailable(err error) bool {
	var verificationError *hcaptchaVerificationError
	return errors.As(err, &verificationError) && verificationError.unavailable
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
		return newHCaptchaError(false, "missing-response")
	}
	if len(token) > hcaptchaTokenMaxBytes {
		return newHCaptchaError(false, "response-too-large")
	}
	if v.client == nil || v.verifyURL == "" {
		return newHCaptchaError(true, "verifier-not-configured")
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
		if errors.Is(err, context.DeadlineExceeded) {
			return newHCaptchaError(true, "provider-timeout")
		}
		return newHCaptchaError(true, "provider-request-failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return newHCaptchaError(true, fmt.Sprintf("provider-http-%d", resp.StatusCode))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, hcaptchaBodyMaxBytes+1))
	if err != nil {
		return newHCaptchaError(true, "provider-response-read-failed")
	}
	if len(body) > hcaptchaBodyMaxBytes {
		return newHCaptchaError(true, "provider-response-too-large")
	}
	var result hcaptchaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return newHCaptchaError(true, "provider-response-invalid-json")
	}
	if !result.Success {
		reasons := safeHCaptchaErrorCodes(result.ErrorCodes)
		unavailable := false
		for _, reason := range reasons {
			if isHCaptchaConfigurationError(reason) {
				unavailable = true
				break
			}
		}
		return newHCaptchaError(unavailable, strings.Join(reasons, ","))
	}
	return nil
}

func newHCaptchaError(unavailable bool, reason string) error {
	return &hcaptchaVerificationError{unavailable: unavailable, reason: reason}
}

func safeHCaptchaErrorCodes(codes []string) []string {
	unique := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" || len(code) > 64 {
			continue
		}
		safe := true
		for _, character := range code {
			if (character < 'a' || character > 'z') &&
				(character < '0' || character > '9') &&
				character != '-' && character != '_' {
				safe = false
				break
			}
		}
		if safe {
			unique[code] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return []string{"provider-rejected-response"}
	}
	result := make([]string, 0, len(unique))
	for code := range unique {
		result = append(result, code)
	}
	sort.Strings(result)
	return result
}

func isHCaptchaConfigurationError(code string) bool {
	switch code {
	case "missing-input-secret",
		"invalid-input-secret",
		"sitekey-secret-mismatch",
		"not-using-dummy-passcode",
		"missing-remoteip",
		"invalid-remoteip",
		"bad-request":
		return true
	default:
		return false
	}
}
