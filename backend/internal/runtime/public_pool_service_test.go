package runtime

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
)

func newPoolTestUpstream(t *testing.T, validCookies map[string]bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie := r.Header.Get("Cookie")
		ok := false
		for valid := range validCookies {
			if strings.Contains(cookie, valid) {
				ok = true
				break
			}
		}
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"msg":"cookie expired"}`))
			return
		}
		hits.Add(1)
		_ = r.ParseForm()
		id := r.FormValue("id")
		if id == "" {
			id = "2601201412385560101"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + id + `","number":"61001","name":"测试车棚","address":"东区","status":"在线","opennum":2,"used":[1]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func poolTestRequests(baseURL string) []parser.CaptureRequest {
	return []parser.CaptureRequest{
		{
			Name:   "2601201412385560101",
			Method: http.MethodPost,
			URL:    baseURL + "/ajax/WxPay/Api/getdevice",
			Headers: map[string]string{
				"Content-Type": "application/x-www-form-urlencoded",
				"Referer":      baseURL + "/i/device/open?id=2601201412385560101",
			},
			Body: "id=2601201412385560101",
		},
	}
}

func TestPublicPoolQuotaCacheFallbackAndContributionReward(t *testing.T) {
	srv, hits := newPoolTestUpstream(t, map[string]bool{
		"pool-good": true,
	})
	repo := testRepository(t)
	manager, err := NewManager(repo, "", poolTestRequests(srv.URL), "AdminPass123!", 30*time.Second)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	settings := manager.Settings()
	settings.InviteRequired = false
	settings.FreeDailyRefreshQuota = 1
	settings.PileContributionBonusQuota = 2
	settings.SharedPileCacheTTLSeconds = 60
	settings.FreeUserWatchAllowed = false
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	// Add 1 expired + 1 healthy cookie into public pool to verify failover.
	if _, err := manager.CreatePublicPoolCookie(model.PublicPoolCookieCreateRequest{
		Label:  "坏号",
		Cookie: "wxopenid=pool-bad; info=expired",
	}); err != nil {
		t.Fatalf("create bad pool cookie: %v", err)
	}
	if _, err := manager.CreatePublicPoolCookie(model.PublicPoolCookieCreateRequest{
		Label:  "好号",
		Cookie: `curl 'https://ele.mocele.com/test' -H 'Cookie: wxopenid=pool-good; info=ok'`,
	}); err != nil {
		t.Fatalf("create good pool cookie: %v", err)
	}

	userA, err := manager.RegisterUser("alice", "AlicePass123!", "")
	if err != nil {
		t.Fatalf("register alice: %v", err)
	}
	userB, err := manager.RegisterUser("bob", "BobPass12345!", "")
	if err != nil {
		t.Fatalf("register bob: %v", err)
	}

	// 1. Alice contributes a brand-new pile -> gets +2 permanent bonus credits!
	addA, err := manager.AddPileWithCatalog(userA.ID, model.PileUpsertRequest{ID: "2601201412385560101"})
	if err != nil {
		t.Fatalf("alice AddPileWithCatalog: %v", err)
	}
	if addA.RewardGranted != 2 || addA.FromSharedCatalog {
		t.Fatalf("alice add result = %+v, want reward=2 fromShared=false", addA)
	}

	// 2. Bob adds the SAME pile -> hits shared catalog, 0 reward, 0 extra upstream hit!
	hitsBeforeBob := hits.Load()
	addB, err := manager.AddPileWithCatalog(userB.ID, model.PileUpsertRequest{ID: "2601201412385560101"})
	if err != nil {
		t.Fatalf("bob AddPileWithCatalog: %v", err)
	}
	if addB.RewardGranted != 0 || !addB.FromSharedCatalog {
		t.Fatalf("bob add result = %+v, want reward=0 fromShared=true", addB)
	}
	if hits.Load() != hitsBeforeBob {
		t.Fatalf("expected 0 upstream hits for shared catalog add, got delta %d", hits.Load()-hitsBeforeBob)
	}

	// 3. Bob refreshes within 60s shared cache -> 0 quota consumed!
	snapCache, err := manager.RefreshPile(userB.ID, "2601201412385560101", false)
	if err != nil {
		t.Fatalf("bob cached RefreshPile: %v", err)
	}
	if !snapCache.Refresh.Cached || snapCache.Quota == nil || snapCache.Quota.FreeDailyUsed != 0 {
		t.Fatalf("expected cached refresh with 0 quota used, got refresh=%+v quota=%+v", snapCache.Refresh, snapCache.Quota)
	}

	// 4. Free user Bob cannot create watch rule while FreeUserWatchAllowed == false.
	if _, err := manager.CreateWatchRule(userB.ID, model.WatchRuleCreateRequest{
		DeviceID: "2601201412385560101",
	}); !errors.Is(err, ErrPersonalCookieRequired) {
		t.Fatalf("free user CreateWatchRule err = %v, want ErrPersonalCookieRequired", err)
	}

	// 5. Expire shared cache by lowering TTL check -> Bob consumes 1 free daily credit, then 2nd refresh exhausts quota!
	if _, err := repo.UpsertSharedPile(addB.Pile, userA.ID, time.Now().UTC().Add(-2*time.Minute)); err != nil {
		t.Fatalf("age shared pile: %v", err)
	}
	snap1, err := manager.RefreshPile(userB.ID, "2601201412385560101", false)
	if err != nil {
		t.Fatalf("bob 1st live refresh: %v", err)
	}
	if snap1.Quota == nil || snap1.Quota.FreeDailyUsed != 1 || snap1.Quota.TotalFreeRemaining != 0 {
		t.Fatalf("unexpected quota after 1st refresh: %+v", snap1.Quota)
	}

	if _, err := repo.UpsertSharedPile(addB.Pile, userA.ID, time.Now().UTC().Add(-2*time.Minute)); err != nil {
		t.Fatalf("age shared pile again: %v", err)
	}
	if _, err := manager.RefreshPile(userB.ID, "2601201412385560101", false); !errors.Is(err, ErrRefreshQuotaExhausted) {
		t.Fatalf("bob 2nd live refresh err = %v, want ErrRefreshQuotaExhausted", err)
	}

	// 6. Alice has an expired personal cookie -> smoothly falls back to public pool + notifies!
	rtA, _ := manager.runtimeFor(userA.ID)
	_ = rtA.client.UpdateCookie("wxopenid=alice-expired; info=bad")
	snapFallback, err := manager.RefreshPile(userA.ID, "2601201412385560101", true)
	if err != nil {
		t.Fatalf("alice fallback refresh: %v", err)
	}
	if !snapFallback.Refresh.PersonalCookieFallback || !snapFallback.Refresh.UsedPool {
		t.Fatalf("expected personal cookie fallback via pool, got %+v", snapFallback.Refresh)
	}
	if snapFallback.Quota == nil || snapFallback.Quota.PersonalCookieStatus != "expired" {
		t.Fatalf("expected personalCookieStatus=expired, got %+v", snapFallback.Quota)
	}
}
