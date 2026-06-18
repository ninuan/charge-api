package store

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"charge-dashboard/internal/model"
)

type DashboardStore struct {
	mu          sync.RWMutex
	piles       map[string]model.Pile
	refresh     model.RefreshInfo
	subscribers map[chan model.DashboardSnapshot]struct{}
}

func NewDashboardStore(initial []model.Pile) *DashboardStore {
	piles := make(map[string]model.Pile, len(initial))
	for _, p := range initial {
		piles[p.ID] = p
	}
	return &DashboardStore{
		piles:       piles,
		subscribers: make(map[chan model.DashboardSnapshot]struct{}),
	}
}

func (s *DashboardStore) Restore(piles []model.Pile, refresh model.RefreshInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	restored := make(map[string]model.Pile, len(piles))
	for _, pile := range piles {
		restored[pile.ID] = pile
	}
	s.piles = restored
	s.refresh = refresh
	s.publishLocked()
}

func (s *DashboardStore) Snapshot() model.DashboardSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

func (s *DashboardStore) SetRefreshInfo(info model.RefreshInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.refresh = info
	s.publishLocked()
}

func (s *DashboardStore) UpsertPile(req model.PileUpsertRequest) (model.Pile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.ID == "" {
		return model.Pile{}, fmt.Errorf("id is required")
	}
	if req.OpenNum <= 0 {
		req.OpenNum = 10
	}
	if req.Name == "" {
		req.Name = "未命名充电桩"
	}
	if req.Status == "" {
		req.Status = "在线"
	}

	now := time.Now()
	existing, ok := s.piles[req.ID]
	if ok {
		existing.Name = req.Name
		existing.Number = req.Number
		existing.Status = req.Status
		existing.Address = req.Address
		existing.OpenNum = req.OpenNum
		existing.Online = req.Status == "在线"
		existing.UpdatedAt = now
		existing.Ports = normalizePorts(existing.Ports, req.OpenNum, existing.Online, now)
		existing.UsedPortIDs = collectUsed(existing.Ports)
		s.piles[req.ID] = existing
		s.publishLocked()
		return existing, nil
	}

	ports := make([]model.Port, 0, req.OpenNum)
	for i := 1; i <= req.OpenNum; i++ {
		ports = append(ports, model.Port{
			ID:        i,
			Status:    model.PortIdle,
			UpdatedAt: now,
		})
	}

	pile := model.Pile{
		ID:          req.ID,
		Name:        req.Name,
		Number:      req.Number,
		Status:      req.Status,
		Address:     req.Address,
		OpenNum:     req.OpenNum,
		Online:      req.Status == "在线",
		CreatedAt:   now,
		UpdatedAt:   now,
		Source:      "manual",
		Ports:       ports,
		UsedPortIDs: []int{},
	}
	s.piles[pile.ID] = pile
	s.publishLocked()
	return pile, nil
}

func (s *DashboardStore) DeletePile(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.piles[id]; !ok {
		return false
	}
	delete(s.piles, id)
	s.publishLocked()
	return true
}

func (s *DashboardStore) MergeCapturePiles(captured []model.Pile) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pile := range captured {
		if existing, ok := s.piles[pile.ID]; ok {
			pile.CreatedAt = existing.CreatedAt
		}
		s.piles[pile.ID] = pile
	}
	s.publishLocked()
}

func (s *DashboardStore) Subscribe() chan model.DashboardSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan model.DashboardSnapshot, 8)
	s.subscribers[ch] = struct{}{}
	ch <- s.snapshotLocked()
	return ch
}

func (s *DashboardStore) Unsubscribe(ch chan model.DashboardSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subscribers[ch]; ok {
		delete(s.subscribers, ch)
		close(ch)
	}
}

func (s *DashboardStore) publishLocked() {
	snapshot := s.snapshotLocked()
	for ch := range s.subscribers {
		select {
		case ch <- snapshot:
		default:
		}
	}
}

func (s *DashboardStore) snapshotLocked() model.DashboardSnapshot {
	piles := make([]model.Pile, 0, len(s.piles))
	for _, p := range s.piles {
		copyPile := p
		copyPile.Ports = append([]model.Port(nil), p.Ports...)
		copyPile.UsedPortIDs = append([]int(nil), p.UsedPortIDs...)
		piles = append(piles, copyPile)
	}

	sort.Slice(piles, func(i, j int) bool {
		return piles[i].ID < piles[j].ID
	})

	stats := model.DashboardCounters{
		PileCount: len(piles),
	}
	for _, p := range piles {
		stats.PortCount += len(p.Ports)
		for _, port := range p.Ports {
			switch port.Status {
			case model.PortInUse:
				stats.InUsePortCount++
			case model.PortOffline:
				stats.OfflinePorts++
			default:
				stats.IdlePortCount++
			}
		}
	}

	return model.DashboardSnapshot{
		Piles:      piles,
		UpdatedAt:  time.Now(),
		Statistics: stats,
		Refresh:    s.refresh,
	}
}

func normalizePorts(existing []model.Port, openNum int, online bool, now time.Time) []model.Port {
	m := make(map[int]model.Port, len(existing))
	for _, p := range existing {
		m[p.ID] = p
	}
	ports := make([]model.Port, 0, openNum)
	for i := 1; i <= openNum; i++ {
		p, ok := m[i]
		if !ok {
			p = model.Port{ID: i, Status: model.PortIdle}
		}
		if !online {
			p.Status = model.PortOffline
			p.PowerKW = 0
			p.EnergyKWh = 0
			p.StartedAt = nil
			p.SessionMin = 0
		}
		p.UpdatedAt = now
		ports = append(ports, p)
	}
	return ports
}

func collectUsed(ports []model.Port) []int {
	used := make([]int, 0)
	for _, p := range ports {
		if p.Status == model.PortInUse {
			used = append(used, p.ID)
		}
	}
	sort.Ints(used)
	return used
}
