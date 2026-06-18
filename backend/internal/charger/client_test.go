package charger

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charge-dashboard/internal/parser"
)

func TestFetchPilesUsesBoundedConcurrencyAndReturnsPartialResults(t *testing.T) {
	var active int32
	var maxActive int32
	requestCounts := make(map[string]*int32)
	for _, id := range []string{"device-1", "device-2", "device-3", "device-4"} {
		requestCounts[id] = new(int32)
	}

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}
		id := values.Get("id")
		atomic.AddInt32(requestCounts[id], 1)
		current := atomic.AddInt32(&active, 1)
		defer atomic.AddInt32(&active, -1)
		for {
			observed := atomic.LoadInt32(&maxActive)
			if current <= observed || atomic.CompareAndSwapInt32(&maxActive, observed, current) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		if id == "device-2" {
			return testResponse(http.StatusBadGateway, "temporary failure"), nil
		}
		return testResponse(http.StatusOK, fmt.Sprintf(`{"id":%q,"status":"在线","opennum":1}`, id)), nil
	})

	client := newTestClient(2, requestCounts)
	client.httpClient.Transport = transport
	result := client.FetchPiles(false)
	if result.Attempted != 4 || len(result.Piles) != 3 || len(result.Failures) != 1 {
		t.Fatalf("unexpected partial result: %+v", result)
	}
	if got := atomic.LoadInt32(&maxActive); got > 2 {
		t.Fatalf("max concurrency exceeded: got %d, want <= 2", got)
	}
	if result.Failures[0].DeviceID != "device-2" || result.Failures[0].Skipped {
		t.Fatalf("unexpected failure: %+v", result.Failures[0])
	}

	second := client.FetchPiles(false)
	if second.Attempted != 3 || second.Skipped != 1 || len(second.Piles) != 3 {
		t.Fatalf("expected failed device to be skipped during backoff: %+v", second)
	}
	if got := atomic.LoadInt32(requestCounts["device-2"]); got != 1 {
		t.Fatalf("backed-off device was requested again: %d requests", got)
	}
}

func TestFailureBackoffIncreasesAndCaps(t *testing.T) {
	client := newClient(nil, false)
	now := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }

	first := client.recordFailure("device-1")
	if got := first.Sub(now); got != 30*time.Second {
		t.Fatalf("first backoff = %s, want 30s", got)
	}
	second := client.recordFailure("device-1")
	if got := second.Sub(now); got != time.Minute {
		t.Fatalf("second backoff = %s, want 1m", got)
	}
	for i := 0; i < 10; i++ {
		client.recordFailure("device-1")
	}
	state := client.backoffs["device-1"]
	if got := state.retryAt.Sub(now); got != maxBackoff {
		t.Fatalf("capped backoff = %s, want %s", got, maxBackoff)
	}
}

func TestConcurrentDeviceAddsRespectLimit(t *testing.T) {
	client := newClient(parser.DefaultCaptureRequests(), false)
	const limit = 10

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_ = client.AddDeviceWithLimit("device-"+strconv.Itoa(index), limit)
		}(i)
	}
	wg.Wait()

	if got := len(client.DeviceIDs()); got != limit {
		t.Fatalf("device count = %d, want %d", got, limit)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newTestClient(concurrency int, counts map[string]*int32) *Client {
	requests := make([]parser.CaptureRequest, 0, len(counts))
	for i := 1; i <= len(counts); i++ {
		id := "device-" + strconv.Itoa(i)
		requests = append(requests, parser.CaptureRequest{
			Name:   id,
			URL:    "https://charger.test/status",
			Method: http.MethodPost,
			Body:   url.Values{"id": {id}}.Encode(),
			Headers: map[string]string{
				"Content-Type": "application/x-www-form-urlencoded",
			},
		})
	}
	client := NewClient(requests)
	client.maxConcurrency = concurrency
	return client
}
