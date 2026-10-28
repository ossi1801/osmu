package osmulang_test

import (
	"strings"
	"testing"

	"osmu/pkg/engine"
	"osmu/pkg/osmulang"
)

type mockWorld struct {
	entities []*engine.Entity
	bus      *engine.EventBus
	logs     []string
}

func (m *mockWorld) Entities() []*engine.Entity { return m.entities }
func (m *mockWorld) EventBus() *engine.EventBus  { return m.bus }
func (m *mockWorld) LogScript(msg string)        { m.logs = append(m.logs, msg) }

func TestValidScriptAndVMExecution(t *testing.T) {
	src := `namespace TestGame;

component Hunter : Entity {
    [Export] public float Range = 15.0f;
    private int kills = 0;

    signal HitTarget(Entity enemy, float damage);

    public void OnUpdate(float dt) {
        List<Entity> found = query Entities
            where Tag == "Enemy" && Health > 0.0f && DistanceTo(this.Position) <= this.Range
            orderby DistanceTo(this.Position) asc;

        if (found.Count > 0) {
            Entity first = found[0];
            emit HitTarget(first, 60.0f);
        }
    }

    on HitTarget(Entity enemy, float damage) {
        enemy.Health = enemy.Health - damage;
        if (enemy.Health <= 0.0f) {
            this.kills = this.kills + 1;
            Destroy(enemy);
        }
    }
}
`
	res := osmulang.CompileAndCheck("Hunter.osmu", src)
	if res.HasErrors() {
		t.Fatalf("expected 0 errors on valid .osmu script, got: %v", res.Diagnostics)
	}

	hunterEnt := engine.NewEntity("h1", "HunterOne", "Player", engine.MeshCube, engine.V3(0, 0, 0), engine.RGB(0, 1, 0))
	enemyNear := engine.NewEntity("e1", "GoblinNear", "Enemy", engine.MeshCube, engine.V3(3, 0, 0), engine.RGB(1, 0, 0))
	enemyNear.Health = 50
	enemyFar := engine.NewEntity("e2", "GoblinFar", "Enemy", engine.MeshCube, engine.V3(9, 0, 0), engine.RGB(1, 0, 0))
	enemyFar.Health = 100

	world := &mockWorld{
		entities: []*engine.Entity{hunterEnt, enemyFar, enemyNear},
		bus:      engine.NewEventBus(),
	}

	comp := res.AST.Components[0]
	inst := osmulang.NewComponentInstance(comp, hunterEnt, world, nil)
	inst.CallMethod("OnUpdate", 0.016)

	if enemyNear.Active {
		t.Fatalf("expected nearest enemy to be destroyed after taking 60 damage (had 50 HP)")
	}
	if kills, ok := inst.Fields["kills"].(float64); !ok || kills != 1 {
		t.Fatalf("expected hunter kills == 1, got %v", inst.Fields["kills"])
	}
}

func TestTypeCheckerDetectsErrors(t *testing.T) {
	badSrc := `namespace Broken;

component BadScript : Entity {
    public int Count = "not_an_int";
    signal Ping(int code);

    public void OnUpdate(float dt) {
        if (123) {
            emit Ping("wrong_arg_type");
            emit UndeclaredSignal(1);
        }
        List<Entity> badQuery = query Entities where 42;
    }
}
`
	res := osmulang.CompileAndCheck("BadScript.osmu", badSrc)
	if !res.HasErrors() {
		t.Fatalf("expected static type errors on invalid script, got none")
	}
	joined := ""
	for _, d := range res.Diagnostics {
		joined += d.Code + ":" + d.Message + "\n"
	}
	expectedCodes := []string{"OSMU206", "OSMU214", "OSMU220", "OSMU218", "OSMU242"}
	for _, code := range expectedCodes {
		if !strings.Contains(joined, code) {
			t.Errorf("expected diagnostic code %s in:\n%s", code, joined)
		}
	}
}
