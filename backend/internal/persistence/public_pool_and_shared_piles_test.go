package persistence

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

func openPoolTestStore(t *testing.T) *Store {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "pool_test.db"), key)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestPublicPoolGuardsMetadataAndSharedPileMetaLock(t *testing.T) {
	store := openPoolTestStore(t)

	// 1. Seed admin cookie + user mutual-aid cookie.
	adminCookie, err := store.CreatePublicPoolCookie(PublicPoolCookieRecord{
		PublicPoolCookie: model.PublicPoolCookie{
			ID:      "pool_admin_1",
			Label:   "管理员主号",
			Enabled: true,
			Status:  "healthy",
		},
		Cookie: "wxopenid=admin; info=ok",
	})
	if err != nil {
		t.Fatalf("CreatePublicPoolCookie: %v", err)
	}
	userCookie, err := store.UpsertUserPublicPoolCookie("u-alice", "用户互助 · alice", "wxopenid=alice; info=ok")
	if err != nil {
		t.Fatalf("UpsertUserPublicPoolCookie: %v", err)
	}
	if userCookie.OwnerUserID != "u-alice" {
		t.Fatalf("userCookie.OwnerUserID = %q, want u-alice", userCookie.OwnerUserID)
	}

	// 2. Empty userID guard must NOT wipe admin cookies (owner_user_id == "").
	if err := store.DeleteUserPublicPoolCookie(""); err != nil {
		t.Fatalf("DeleteUserPublicPoolCookie empty: %v", err)
	}
	if err := store.MarkUserPublicPoolCookieExpired("   ", "ignored"); err != nil {
		t.Fatalf("MarkUserPublicPoolCookieExpired empty: %v", err)
	}
	count, err := store.CountHealthyPublicPoolCookies()
	if err != nil || count != 2 {
		t.Fatalf("CountHealthyPublicPoolCookies = %d, %v, want 2", count, err)
	}

	// 3. UpdatePublicPoolCookie preserves OwnerUserID.
	newLabel := "用户互助 · alice-updated"
	updated, err := store.UpdatePublicPoolCookie(userCookie.ID, &newLabel, nil, nil)
	if err != nil {
		t.Fatalf("UpdatePublicPoolCookie: %v", err)
	}
	if updated.OwnerUserID != "u-alice" {
		t.Fatalf("updated.OwnerUserID = %q, want u-alice", updated.OwnerUserID)
	}

	// 4. MarkUserPublicPoolCookieExpired only expires user's cookie, leaving admin healthy.
	if err := store.MarkUserPublicPoolCookieExpired("u-alice", "个人 Cookie 远端鉴权失效"); err != nil {
		t.Fatalf("MarkUserPublicPoolCookieExpired: %v", err)
	}
	meta, err := store.GetUserPublicPoolCookie("u-alice")
	if err != nil || meta == nil || meta.Status != "expired" {
		t.Fatalf("GetUserPublicPoolCookie = %+v, %v, want expired", meta, err)
	}
	count, err = store.CountHealthyPublicPoolCookies()
	if err != nil || count != 1 {
		t.Fatalf("CountHealthyPublicPoolCookies after expire = %d, %v, want 1", count, err)
	}

	// 5. Deleting user's pool cookie keeps admin's cookie intact.
	if err := store.DeleteUserPublicPoolCookie("u-alice"); err != nil {
		t.Fatalf("DeleteUserPublicPoolCookie: %v", err)
	}
	list, err := store.ListPublicPoolCookies()
	if err != nil || len(list) != 1 || list[0].ID != adminCookie.ID {
		t.Fatalf("ListPublicPoolCookies = %+v, %v, want only %s", list, err, adminCookie.ID)
	}

	// 6. Admin-edited shared pile name/address must survive subsequent remote refreshes.
	now := time.Now().UTC()
	inserted, err := store.UpsertSharedPile(model.Pile{
		ID:      "2601201412385560999",
		Number:  "610999",
		Name:    "原始桩名",
		Address: "原始地址",
		OpenNum: 10,
	}, "", now)
	if err != nil || !inserted {
		t.Fatalf("initial UpsertSharedPile: inserted=%v err=%v", inserted, err)
	}
	if _, err := store.UpdateSharedPileMeta("2601201412385560999", "管理员规范名称", "北门车棚东侧"); err != nil {
		t.Fatalf("UpdateSharedPileMeta: %v", err)
	}
	if _, err := store.UpsertSharedPile(model.Pile{
		ID:      "2601201412385560999",
		Number:  "610999",
		Name:    "远端旧名",
		Address: "远端旧地址",
		OpenNum: 10,
		Online:  true,
	}, "", now.Add(time.Minute)); err != nil {
		t.Fatalf("subsequent UpsertSharedPile: %v", err)
	}
	shared, err := store.GetSharedPile("2601201412385560999")
	if err != nil || shared == nil {
		t.Fatalf("GetSharedPile: %+v, %v", shared, err)
	}
	if shared.Name != "管理员规范名称" || shared.Address != "北门车棚东侧" {
		t.Fatalf("admin-locked metadata overwritten: name=%q address=%q", shared.Name, shared.Address)
	}
	if shared.LastSnapshot == nil || shared.LastSnapshot.Name != "管理员规范名称" || shared.LastSnapshot.Address != "北门车棚东侧" {
		t.Fatalf("admin-locked snapshot metadata overwritten: %+v", shared.LastSnapshot)
	}
}

func TestPublicPoolCookieDeduplicatesByWXOpenID(t *testing.T) {
	store := openPoolTestStore(t)

	// User A joins pool with wxopenid=same-wx.
	if _, err := store.UpsertUserPublicPoolCookie("u-a", "用户互助 · a", "deviceid=1; wxopenid=same-wx; info=v1"); err != nil {
		t.Fatalf("u-a upsert: %v", err)
	}

	// User B tries to join with the SAME wxopenid (different order, refreshed info, extra noise key) -> rejected!
	if _, err := store.UpsertUserPublicPoolCookie("u-b", "用户互助 · b", "foo=bar; info=v2; wxopenid=same-wx; deviceid=999"); !errors.Is(err, ErrPublicPoolCookieDuplicate) {
		t.Fatalf("u-b duplicate upsert err = %v, want ErrPublicPoolCookieDuplicate", err)
	}

	// Admin tries to add the SAME wxopenid -> rejected!
	if _, err := store.CreatePublicPoolCookie(PublicPoolCookieRecord{
		PublicPoolCookie: model.PublicPoolCookie{ID: "pool_dup", Label: "重复号", Enabled: true, Status: "healthy"},
		Cookie:           "wxopenid=same-wx; info=v3",
	}); !errors.Is(err, ErrPublicPoolCookieDuplicate) {
		t.Fatalf("admin duplicate create err = %v, want ErrPublicPoolCookieDuplicate", err)
	}

	// User A updates their OWN cookie with a refreshed info (same wxopenid) -> allowed!
	if _, err := store.UpsertUserPublicPoolCookie("u-a", "用户互助 · a", "wxopenid=same-wx; info=v4-refreshed"); err != nil {
		t.Fatalf("u-a self update should succeed: %v", err)
	}

	// Once User A leaves the pool, User B can join with that wxopenid.
	if err := store.DeleteUserPublicPoolCookie("u-a"); err != nil {
		t.Fatalf("u-a delete: %v", err)
	}
	if _, err := store.UpsertUserPublicPoolCookie("u-b", "用户互助 · b", "wxopenid=same-wx; info=v5"); err != nil {
		t.Fatalf("u-b upsert after u-a left should succeed: %v", err)
	}
}
