package engine_test

import (
	"testing"

	"osmu/pkg/engine"
)

func TestQueryEngineAndEventBus(t *testing.T) {
	e1 := engine.NewEntity("1", "Alpha", "Enemy", engine.MeshCube, engine.V3(10, 0, 0), engine.RGB(1, 0, 0))
	e1.Health = 40
	e2 := engine.NewEntity("2", "Beta", "Enemy", engine.MeshCube, engine.V3(3, 0, 0), engine.RGB(1, 0, 0))
	e2.Health = 30
	e3 := engine.NewEntity("3", "GoldCoin", "Coin", engine.MeshSphere, engine.V3(1, 0, 0), engine.RGB(1, 1, 0))

	entities := []*engine.Entity{e1, e2, e3}

	res, err := engine.EvalQueryString(entities, `Tag == "Enemy" && Health <= 45 orderby Distance asc`, engine.V3(0, 0, 0))
	if err != nil {
		t.Fatalf("unexpected query error: %v", err)
	}
	if len(res) != 2 || res[0].ID != "2" || res[1].ID != "1" {
		t.Fatalf("expected [Beta, Alpha] ordered by distance, got %v", res)
	}

	bus := engine.NewEventBus()
	received := 0
	bus.On("WaveCleared", "", func(ev engine.SignalEvent) {
		received++
	})
	bus.Emit("WaveCleared", "1", 100)
	if received != 1 {
		t.Fatalf("expected 1 signal callback, got %d", received)
	}
}
