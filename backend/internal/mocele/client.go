package mocele

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"charge-dashboard/internal/security"
)

const defaultBaseURL = "https://ele.mocele.com"

type Config struct {
	BaseURL    string
	HTTPClient *http.Client
	Org        string
	OpenIndex  string
}

type Client struct {
	baseURL   string
	http      *http.Client
	org       string
	openIndex string
}

type CookieResult struct {
	Cookie   string
	WXOpenID string
	Info     string
}

func NewClient(cfg Config) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	org := cfg.Org
	if org == "" {
		org = "1"
	}
	openIndex := cfg.OpenIndex
	if openIndex == "" {
		openIndex = "7"
	}
	return &Client{baseURL: baseURL, http: httpClient, org: org, openIndex: openIndex}
}

func (c *Client) ExchangeCode(ctx context.Context, deviceID string, code string) (CookieResult, error) {
	if strings.TrimSpace(deviceID) == "" {
		return CookieResult{}, fmt.Errorf("deviceID is required")
	}
	if strings.TrimSpace(code) == "" {
		return CookieResult{}, fmt.Errorf("code is required")
	}
	autologinURL, err := c.autologinURL(deviceID, code)
	if err != nil {
		return CookieResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, autologinURL, nil)
	if err != nil {
		return CookieResult{}, err
	}
	httpClient, jar, err := c.exchangeHTTPClient()
	if err != nil {
		return CookieResult{}, err
	}
	jar.SetCookies(req.URL, []*http.Cookie{
		{Name: "deviceid", Value: deviceID},
		{Name: "org", Value: c.org},
		{Name: "openindex", Value: c.openIndex},
	})
	resp, err := httpClient.Do(req)
	if err != nil {
		return CookieResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return CookieResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CookieResult{}, fmt.Errorf("mocele autologin failed: status=%d body=%s", resp.StatusCode, security.RedactText(string(body), 256))
	}
	cookies := map[string]string{}
	for _, cookie := range jar.Cookies(resp.Request.URL) {
		cookies[strings.ToLower(cookie.Name)] = cookie.Value
	}
	wxopenid := cookies["wxopenid"]
	if wxopenid == "" {
		return CookieResult{}, fmt.Errorf("mocele autologin response missing wxopenid cookie")
	}
	info := cookies["info"]
	if info == "" {
		return CookieResult{}, fmt.Errorf("mocele autologin response missing info cookie")
	}
	finalCookie := fmt.Sprintf("deviceid=%s; org=%s; openindex=%s; wxopenid=%s; info=%s", deviceID, c.org, c.openIndex, wxopenid, info)
	return CookieResult{Cookie: finalCookie, WXOpenID: wxopenid, Info: info}, nil
}

func (c *Client) exchangeHTTPClient() (*http.Client, http.CookieJar, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create autologin cookie jar: %w", err)
	}
	client := *c.http
	client.Jar = jar
	return &client, jar, nil
}

func (c *Client) autologinURL(deviceID string, code string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	base.Path = "/ajax/WxPay/Api/autologin"
	q := base.Query()
	q.Set("r", "/i/device/open?id="+deviceID)
	q.Set("code", code)
	q.Set("state", "1")
	base.RawQuery = q.Encode()
	return base.String(), nil
}
