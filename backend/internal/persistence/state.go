package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"charge-dashboard/internal/model"
)

type State struct {
	Version    int                  `json:"version"`
	Users      []model.User         `json:"users"`
	UserStates map[string]UserState `json:"userStates"`
	SavedAt    time.Time            `json:"savedAt"`

	Piles     []model.Pile               `json:"piles,omitempty"`
	Refresh   model.RefreshInfo          `json:"refresh,omitempty"`
	DeviceIDs []string                   `json:"deviceIds,omitempty"`
	Cookie    string                     `json:"cookie,omitempty"`
	Settings  model.RegistrationSettings `json:"settings"`
	Invites   []model.InviteCode         `json:"invites"`
}

type UserState struct {
	Piles               []model.Pile               `json:"piles"`
	Refresh             model.RefreshInfo          `json:"refresh"`
	DeviceIDs           []string                   `json:"deviceIds"`
	Cookie              string                     `json:"cookie,omitempty"`
	Stats               model.TrafficStats         `json:"stats"`
	YYBBinding          *model.YYBBinding          `json:"yybBinding,omitempty"`
	RecoveryDiagnostics []model.RecoveryDiagnostic `json:"recoveryDiagnostics,omitempty"`
}

func LoadJSON(path string) (State, bool, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("read legacy state: %w", err)
	}

	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		return State{}, false, fmt.Errorf("parse legacy state: %w", err)
	}
	return state, true, nil
}

func ArchiveMigratedJSON(path string, state State) error {
	state.Cookie = ""
	for userID, userState := range state.UserStates {
		userState.Cookie = ""
		userState.YYBBinding = nil
		state.UserStates[userID] = userState
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode sanitized migration archive: %w", err)
	}
	archivePath := path + ".migrated"
	if err := os.WriteFile(archivePath, body, 0600); err != nil {
		return fmt.Errorf("write sanitized migration archive: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove plaintext legacy state: %w", err)
	}
	return nil
}
