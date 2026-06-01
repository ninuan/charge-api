package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"charge-dashboard/internal/api"
	"charge-dashboard/internal/charger"
	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
	"charge-dashboard/internal/store"
)

func main() {
	var (
		listenAddr = flag.String("listen", ":8080", "http listen address")
		captureDir = flag.String("capture", "../20260601_202646", "capture directory")
		statePath  = flag.String("state", "../charge_state.json", "local persisted state file")
	)
	flag.Parse()

	absCaptureDir, err := filepath.Abs(*captureDir)
	if err != nil {
		log.Fatalf("resolve capture dir: %v", err)
	}

	if _, err := os.Stat(absCaptureDir); err != nil {
		log.Fatalf("capture dir not available: %v", err)
	}
	absStatePath, err := filepath.Abs(*statePath)
	if err != nil {
		log.Fatalf("resolve state path: %v", err)
	}

	requests, err := parser.ParseCaptureRequests(absCaptureDir)
	if err != nil {
		log.Fatalf("parse capture requests: %v", err)
	}
	chargerClient := charger.NewClient(requests)

	persistedState, hasPersistedState, err := persistence.Load(absStatePath)
	if err != nil {
		log.Printf("load persisted state failed: %v", err)
	}
	if hasPersistedState && persistedState.Cookie != "" {
		if err := chargerClient.UpdateCookie(persistedState.Cookie); err != nil {
			log.Printf("restore persisted cookie failed: %v", err)
		}
	}
	if hasPersistedState {
		deviceIDs := persistedState.DeviceIDs
		if len(deviceIDs) == 0 {
			for _, pile := range persistedState.Piles {
				deviceIDs = append(deviceIDs, pile.ID)
			}
		}
		chargerClient.RestoreDevices(deviceIDs)
	}

	store := store.NewDashboardStore(nil)
	const minRefreshInterval = 30 * time.Second
	var (
		refreshMu       sync.Mutex
		lastRemoteFetch time.Time
	)
	if hasPersistedState {
		refresh := persistedState.Refresh
		if refresh.MinIntervalSeconds == 0 {
			refresh.MinIntervalSeconds = int(minRefreshInterval.Seconds())
		}
		refresh.Message = "已读取本地缓存，未请求远端接口"
		refresh.Cached = true
		store.Restore(persistedState.Piles, refresh)
		if refresh.LastRemoteAt != nil {
			lastRemoteFetch = *refresh.LastRemoteAt
		}
		log.Printf("restored local state from %s", absStatePath)
	}

	saveCurrentState := func() error {
		snapshot := store.Snapshot()
		return persistence.Save(absStatePath, persistence.State{
			Piles:     snapshot.Piles,
			Refresh:   snapshot.Refresh,
			DeviceIDs: chargerClient.DeviceIDs(),
			Cookie:    chargerClient.Cookie(),
		})
	}

	refreshFn := func(force bool) error {
		refreshMu.Lock()
		defer refreshMu.Unlock()

		now := time.Now()
		if !force && !lastRemoteFetch.IsZero() && now.Sub(lastRemoteFetch) < minRefreshInterval {
			next := lastRemoteFetch.Add(minRefreshInterval)
			store.SetRefreshInfo(model.RefreshInfo{
				LastRemoteAt:       &lastRemoteFetch,
				NextRemoteAt:       &next,
				MinIntervalSeconds: int(minRefreshInterval.Seconds()),
				Cached:             true,
				Message:            "刷新间隔内，已返回缓存数据",
			})
			return saveCurrentState()
		}

		piles, fetchErr := chargerClient.FetchPiles()
		if fetchErr != nil {
			if charger.IsAuthExpired(fetchErr) {
				store.SetRefreshInfo(model.RefreshInfo{
					MinIntervalSeconds: int(minRefreshInterval.Seconds()),
					Cached:             true,
					Message:            "Cookie 可能已过期，请更新 Cookie 后重试",
				})
				if saveErr := saveCurrentState(); saveErr != nil {
					log.Printf("save state failed: %v", saveErr)
				}
			}
			return fetchErr
		}
		store.ReplaceCapturePiles(piles)
		lastRemoteFetch = time.Now()
		next := lastRemoteFetch.Add(minRefreshInterval)
		store.SetRefreshInfo(model.RefreshInfo{
			LastRemoteAt:       &lastRemoteFetch,
			NextRemoteAt:       &next,
			MinIntervalSeconds: int(minRefreshInterval.Seconds()),
			Cached:             false,
			Message:            "已请求远端接口",
		})
		return saveCurrentState()
	}
	addRemoteFn := func(req model.PileUpsertRequest) (model.Pile, error) {
		if err := chargerClient.AddDevice(req.ID); err != nil {
			return model.Pile{}, err
		}
		if err := refreshFn(true); err != nil {
			chargerClient.RemoveDevice(req.ID)
			return model.Pile{}, err
		}
		for _, pile := range store.Snapshot().Piles {
			if pile.ID == req.ID {
				return pile, nil
			}
		}
		return model.Pile{}, fmt.Errorf("device %s was not returned by remote API", req.ID)
	}
	deleteRemoteFn := func(id string) {
		chargerClient.RemoveDevice(id)
		if err := saveCurrentState(); err != nil {
			log.Printf("save state failed: %v", err)
		}
	}
	updateCookieFn := func(cookie string) error {
		previous := chargerClient.Cookie()
		if err := chargerClient.UpdateCookie(cookie); err != nil {
			return err
		}
		if err := refreshFn(true); err != nil {
			_ = chargerClient.UpdateCookie(previous)
			return err
		}
		return nil
	}
	if !hasPersistedState {
		if err := refreshFn(true); err != nil {
			log.Printf("initial remote refresh failed: %v", err)
		}
	} else if err := saveCurrentState(); err != nil {
		log.Printf("save restored state failed: %v", err)
	}

	server := api.NewServer(store, refreshFn, addRemoteFn, deleteRemoteFn, updateCookieFn)
	mux := http.NewServeMux()
	server.Register(mux)

	mux.Handle("/", http.FileServer(http.Dir("../frontend/dist")))

	log.Printf("server listening on %s", *listenAddr)
	log.Printf("capture request templates loaded from %s", absCaptureDir)
	if err := http.ListenAndServe(*listenAddr, withCORS(mux)); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
