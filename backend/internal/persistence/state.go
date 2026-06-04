package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"charge-dashboard/internal/model"
)

type State struct {
	Version    int                  `json:"version"`
	Users      []model.User         `json:"users"`
	UserStates map[string]UserState `json:"userStates"`
	SavedAt    time.Time            `json:"savedAt"`

	Piles     []model.Pile      `json:"piles,omitempty"`
	Refresh   model.RefreshInfo `json:"refresh,omitempty"`
	DeviceIDs []string          `json:"deviceIds,omitempty"`
	Cookie    string            `json:"cookie,omitempty"`
}

type UserState struct {
	Piles     []model.Pile       `json:"piles"`
	Refresh   model.RefreshInfo  `json:"refresh"`
	DeviceIDs []string           `json:"deviceIds"`
	Cookie    string             `json:"cookie,omitempty"`
	Stats     model.TrafficStats `json:"stats"`
}

func Load(path string) (State, bool, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("read state: %w", err)
	}

	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		return State{}, false, fmt.Errorf("parse state: %w", err)
	}
	return state, true, nil
}

func Save(path string, state State) error {
	state.SavedAt = time.Now()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}
