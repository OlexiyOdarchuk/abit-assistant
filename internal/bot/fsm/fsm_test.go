package fsm_test

import (
	"context"
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/bot/fsm"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage/pgtest"
)

func newManager(t *testing.T) (*fsm.Manager, int64) {
	t.Helper()
	store := pgtest.New(t)
	// FSM rows FK on users.tg_id with ON DELETE CASCADE — seed a user.
	const uid = int64(42)
	if err := store.UpsertUser(context.Background(), uid); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	return fsm.New(store), uid
}

func TestManager_NoStateInitially(t *testing.T) {
	m, uid := newManager(t)
	got, err := m.Get(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsActive() {
		t.Errorf("expected empty state, got %+v", got)
	}
}

func TestManager_SetGetRoundtrip(t *testing.T) {
	m, uid := newManager(t)
	ctx := context.Background()

	want := map[string]any{
		"current_subject": "Математика",
		"score":           175.5,
		"step":            float64(2), // JSON numbers round-trip as float64
	}
	if err := m.Set(ctx, uid, "profile.enter_score", want); err != nil {
		t.Fatal(err)
	}

	got, err := m.Get(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "profile.enter_score" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Get("current_subject") != "Математика" {
		t.Errorf("subject = %q", got.Get("current_subject"))
	}
	if got.Data["score"] != 175.5 {
		t.Errorf("score = %v", got.Data["score"])
	}
}

func TestManager_SetOverwrites(t *testing.T) {
	m, uid := newManager(t)
	ctx := context.Background()

	_ = m.Set(ctx, uid, "a", map[string]any{"x": "1"})
	_ = m.Set(ctx, uid, "b", map[string]any{"y": "2"})

	got, _ := m.Get(ctx, uid)
	if got.Name != "b" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Get("y") != "2" || got.Get("x") != "" {
		t.Errorf("Data = %+v", got.Data)
	}
}

func TestManager_Clear(t *testing.T) {
	m, uid := newManager(t)
	ctx := context.Background()

	_ = m.Set(ctx, uid, "x", nil)
	if err := m.Clear(ctx, uid); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get(ctx, uid)
	if got.IsActive() {
		t.Errorf("after Clear, IsActive should be false, got %+v", got)
	}
}

func TestManager_Clear_Idempotent(t *testing.T) {
	m, uid := newManager(t)
	ctx := context.Background()
	if err := m.Clear(ctx, uid); err != nil {
		t.Errorf("clear on empty: %v", err)
	}
}

func TestState_AccessorsOnZero(t *testing.T) {
	var s fsm.State
	if s.IsActive() {
		t.Error("zero state should not be active")
	}
	if s.Get("anything") != "" {
		t.Error("zero state Get should be empty")
	}
}
