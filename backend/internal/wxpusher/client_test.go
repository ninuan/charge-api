package wxpusher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientProviderOperations(t *testing.T) {
	expires := time.Now().Add(30 * time.Minute).Truncate(time.Millisecond)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fun/create/qrcode":
			if r.Method != http.MethodPost {
				t.Fatalf("create QR method = %s", r.Method)
			}
			var request createQRCodeRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode create QR request: %v", err)
			}
			if request.AppToken != "AT_test_secret" || request.Extra != "bind-session" || request.ValidTime != 1800 {
				t.Fatalf("create QR request = %+v", request)
			}
			fmt.Fprintf(w, `{"code":1000,"msg":"ok","success":true,"data":{"code":"QR_code","url":"https://wxpusher.example/qr","shortUrl":"https://wxpusher.example/s","extra":"bind-session","expires":%d}}`, expires.UnixMilli())
		case "/api/fun/scan-qrcode-uid":
			if r.URL.Query().Get("code") != "QR_code" {
				t.Fatalf("scan code = %q", r.URL.Query().Get("code"))
			}
			fmt.Fprint(w, `{"code":1000,"msg":"ok","success":true,"data":{"uid":"UID_scanned"}}`)
		case "/api/send/message":
			var request sendMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode send request: %v", err)
			}
			if request.AppToken != "AT_test_secret" || len(request.UIDs) != 1 || request.UIDs[0] != "UID_scanned" || request.ContentType != ContentTypeText {
				t.Fatalf("send request = %+v", request)
			}
			fmt.Fprint(w, `{"code":1000,"msg":"ok","success":true,"data":[{"uid":"UID_scanned","messageContentId":2123,"sendRecordId":12313,"code":1000,"status":"创建发送任务成功"}]}`)
		case "/api/send/query/status":
			if r.URL.Query().Get("sendRecordId") != "12313" {
				t.Fatalf("send record ID = %q", r.URL.Query().Get("sendRecordId"))
			}
			fmt.Fprint(w, `{"code":1000,"msg":"ok","success":true,"data":{"sendRecordId":12313,"code":1000,"status":"发送成功","success":true}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	qr, err := client.CreateQRCode(context.Background(), " bind-session ", 30*time.Minute)
	if err != nil {
		t.Fatalf("CreateQRCode: %v", err)
	}
	if qr.Code != "QR_code" || qr.URL != "https://wxpusher.example/qr" || !qr.ExpiresAt.Equal(expires) {
		t.Fatalf("QR code = %+v", qr)
	}
	scan, err := client.QueryScanUID(context.Background(), qr.Code)
	if err != nil {
		t.Fatalf("QueryScanUID: %v", err)
	}
	if !scan.Scanned || scan.UID != "UID_scanned" {
		t.Fatalf("scan result = %+v", scan)
	}
	sent, err := client.Send(context.Background(), Message{
		UID: scan.UID, Summary: "空闲提醒", Content: "当前有空闲充电口", ContentType: ContentTypeText,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sent.SendRecordID != "12313" || sent.MessageContentID != "2123" {
		t.Fatalf("send result = %+v", sent)
	}
	status, err := client.QueryMessageStatus(context.Background(), sent.SendRecordID)
	if err != nil {
		t.Fatalf("QueryMessageStatus: %v", err)
	}
	if status.SendRecordID != "12313" || !status.Succeeded || status.Status != "发送成功" {
		t.Fatalf("message status = %+v", status)
	}
}

func TestClientQueryScanUIDPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"code":1000,"msg":"ok","success":true,"data":null}`)
	}))
	defer server.Close()
	result, err := newTestClient(t, server.URL, 0).QueryScanUID(context.Background(), "QR_pending")
	if err != nil {
		t.Fatalf("QueryScanUID: %v", err)
	}
	if result.Scanned || result.UID != "" {
		t.Fatalf("pending scan result = %+v", result)
	}
}

func TestClientClassifiesTransportFailures(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		want      ErrorCode
		retryable bool
	}{
		{name: "rate limited", status: http.StatusTooManyRequests, body: `rate limited`, want: ErrorRateLimited, retryable: true},
		{name: "provider failure", status: http.StatusBadGateway, body: `upstream failed`, want: ErrorProviderUnavailable, retryable: true},
		{name: "invalid JSON", status: http.StatusOK, body: `{not-json`, want: ErrorInvalidResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			_, err := newTestClient(t, server.URL, 0).CreateQRCode(context.Background(), "session", time.Minute)
			assertProviderError(t, err, tt.want, tt.retryable)
		})
	}
}

func TestClientClassifiesTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		fmt.Fprint(w, `{"code":1000,"data":{}}`)
	}))
	defer server.Close()
	_, err := newTestClient(t, server.URL, 20*time.Millisecond).CreateQRCode(context.Background(), "session", time.Minute)
	assertProviderError(t, err, ErrorProviderTimeout, true)
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	if got := parseRetryAfter("120", now); got != 2*time.Minute {
		t.Fatalf("delta retry-after = %s", got)
	}
	if got := parseRetryAfter(now.Add(3*time.Minute).Format(http.TimeFormat), now); got != 3*time.Minute {
		t.Fatalf("date retry-after = %s", got)
	}
	if got := parseRetryAfter("invalid", now); got != 0 {
		t.Fatalf("invalid retry-after = %s", got)
	}
}

func TestClientClassifiesBusinessFailuresWithoutLeakingSecrets(t *testing.T) {
	const token = "AT_do_not_log_this_token"
	const uid = "UID_do_not_log_this_user"
	tests := []struct {
		name string
		body string
		want ErrorCode
	}{
		{
			name: "invalid token",
			body: `{"code":1001,"msg":"appToken AT_do_not_log_this_token 无效","success":false,"data":null}`,
			want: ErrorInvalidToken,
		},
		{
			name: "invalid UID",
			body: `{"code":1000,"msg":"ok","success":true,"data":[{"uid":"UID_do_not_log_this_user","messageContentId":1,"sendRecordId":2,"code":1002,"status":"UID_do_not_log_this_user UID无效"}]}`,
			want: ErrorInvalidUID,
		},
		{
			name: "recipient rejected",
			body: `{"code":1000,"msg":"ok","success":true,"data":[{"uid":"UID_do_not_log_this_user","messageContentId":1,"sendRecordId":2,"code":1003,"status":"UID_do_not_log_this_user 已拒收消息"}]}`,
			want: ErrorRecipientRejected,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			client, err := NewClient(Config{AppToken: token, BaseURL: server.URL})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			_, err = client.Send(context.Background(), Message{UID: uid, Content: "test", ContentType: ContentTypeText})
			assertProviderError(t, err, tt.want, false)
			errorText := fmt.Sprintf("%v", err)
			if strings.Contains(errorText, token) || strings.Contains(errorText, uid) || strings.Contains(errorText, tt.body) {
				t.Fatalf("error leaked provider data: %s", errorText)
			}
		})
	}
}

func TestClientRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, strings.Repeat("x", MaxResponseBytes+1))
	}))
	defer server.Close()
	_, err := newTestClient(t, server.URL, 0).CreateQRCode(context.Background(), "session", time.Minute)
	assertProviderError(t, err, ErrorInvalidResponse, false)
}

func TestClientValidatesConfigurationAndMessages(t *testing.T) {
	if _, err := NewClient(Config{}); CodeOf(err) != ErrorUnconfigured {
		t.Fatalf("empty token error = %v", err)
	}
	if _, err := NewClient(Config{AppToken: "AT_test", BaseURL: "http://example.com"}); CodeOf(err) != ErrorInvalidRequest {
		t.Fatalf("unsafe base URL error = %v", err)
	}
	client := newTestClient(t, "http://127.0.0.1:1", 0)
	_, err := client.Send(context.Background(), Message{
		UID: "UID_test", Content: "test", ContentType: ContentTypeText,
		Summary: strings.Repeat("字", MaxSummaryLength+1),
	})
	assertProviderError(t, err, ErrorInvalidRequest, false)
}

func newTestClient(t *testing.T, baseURL string, timeout time.Duration) *Client {
	t.Helper()
	client, err := NewClient(Config{AppToken: "AT_test_secret", BaseURL: baseURL, RequestTimeout: timeout})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func assertProviderError(t *testing.T, err error, want ErrorCode, retryable bool) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error", want)
	}
	var providerErr *Error
	if !errors.As(err, &providerErr) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if providerErr.Code != want || providerErr.Retryable != retryable || IsRetryable(err) != retryable {
		t.Fatalf("provider error = %+v, want code=%s retryable=%v", providerErr, want, retryable)
	}
}
