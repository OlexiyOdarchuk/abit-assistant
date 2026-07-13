// Package fsm is the persistent conversation-state store for the bot.
//
// Each user has at most one active conversation. The state is a pair of
// (Name, Data) — Name routes the next user message to the right handler;
// Data carries per-conversation scratchpad (subject being edited, URL
// being parsed, ...). The whole thing lives in SQLite, so an in-flight
// conversation survives a bot restart.
package fsm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage/db"
)

// State describes the conversation the user is currently in.
type State struct {
	// Name is the state identifier (e.g. "profile.enter_score"). Empty
	// means "no active conversation".
	Name string
	// Data is opaque scratchpad — handlers cast values they put in.
	Data map[string]any
}

// IsActive reports whether the user is currently mid-conversation.
func (s *State) IsActive() bool {
	return s != nil && s.Name != ""
}

// Get returns Data[key] as string, or "" if missing/not a string.
func (s *State) Get(key string) string {
	if s == nil || s.Data == nil {
		return ""
	}
	v, _ := s.Data[key].(string)
	return v
}

// Manager is the FSM facade. Construct one and share across handlers.
type Manager struct {
	store *storage.Store
}

// New wires a Manager onto the storage layer.
func New(store *storage.Store) *Manager { return &Manager{store: store} }

// Get returns the active state, or an empty (Name == "") state when no
// conversation exists.
func (m *Manager) Get(ctx context.Context, tgID int64) (State, error) {
	row, err := m.store.ReadQueries.GetFSM(ctx, tgID)
	if errors.Is(err, sql.ErrNoRows) {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("fsm: get: %w", err)
	}
	state := State{Name: row.State}
	if row.Data != "" && row.Data != "{}" {
		state.Data = make(map[string]any)
		if err := json.Unmarshal([]byte(row.Data), &state.Data); err != nil {
			return State{}, fmt.Errorf("fsm: decode data: %w", err)
		}
	}
	return state, nil
}

// Set persists the given state. Passing an empty name is allowed — use
// Clear for the more obvious intent.
func (m *Manager) Set(ctx context.Context, tgID int64, name string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("fsm: encode data: %w", err)
	}
	return m.store.Queries.SetFSM(ctx, db.SetFSMParams{
		TgID:  tgID,
		State: name,
		Data:  string(raw),
	})
}

// Clear removes any active conversation. Idempotent.
func (m *Manager) Clear(ctx context.Context, tgID int64) error {
	return m.store.Queries.ClearFSM(ctx, tgID)
}
