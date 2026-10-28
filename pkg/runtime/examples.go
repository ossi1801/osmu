package runtime

import (
	"os"
	"path/filepath"

	"osmu/pkg/engine"
	"osmu/pkg/nodegraph"
)

// --- PROJECT 1: CYBER SURVIVORS 2D (.osmu scripts) ---

const CyberHero2DScript = `namespace CyberSurvivors;

component CyberHero2D : Entity {
    [Export] public float MoveSpeed = 6.5f;
    [Export] public float FireRange = 9.5f;
    [Export] public float FireInterval = 0.28f;
    private float fireTimer = 0.0f;
    private int xp = 0;
    private int level = 1;

    signal PlasmaFired(Entity target, int level);
    signal HeroLevelUp(int newLevel);

    public void OnReady() {
        Log("CyberHero2D ready! Use WASD / Arrows to move.");
    }

    public void OnUpdate(float dt) {
        this.fireTimer = this.fireTimer - dt;

        // Player WASD / Arrow movement (with smart kiting if no keys pressed)
        float ix = InputAxisX();
        float iy = InputAxisY();
        if (ix != 0.0f || iy != 0.0f) {
            this.Position = this.Position + Vec3(ix * this.MoveSpeed * dt, iy * this.MoveSpeed * dt, 0.0f);
        } else {
            // Auto-pilot toward nearest XP gem while evading close enemies
            List<Entity> gems = query Entities
                where Tag == "Coin" && Active == true
                orderby DistanceTo(this.Position) asc;
            if (gems.Count > 0) {
                Entity g = gems[0];
                float gd = Distance(this.Position, g.Position);
                if (gd > 0.25f) {
                    Vec3 dir = (g.Position - this.Position) / gd;
                    this.Position = this.Position + dir * (this.MoveSpeed * 0.65f * dt);
                }
            }
        }

        // Auto-target nearest enemy within FireRange using 1-line .osmu query
        List<Entity> enemies = query Entities
            where Tag == "Enemy" && Health > 0.0f && DistanceTo(this.Position) <= this.FireRange
            orderby DistanceTo(this.Position) asc;

        if (enemies.Count > 0 && this.fireTimer <= 0.0f) {
            Entity nearest = enemies[0];
            this.LookAt(nearest.Position);
            float d = Distance(this.Position, nearest.Position);
            if (d > 0.1f) {
                Vec3 boltVel = ((nearest.Position - this.Position) / d) * 14.0f;
                Spawn("PlasmaBolt", "Projectile", "Circle2D", this.Position, boltVel);
                this.fireTimer = this.FireInterval;
                emit PlasmaFired(nearest, this.level);
            }
        }
    }

    public void OnCollision(Entity other) {
        if (other.HasTag("Coin")) {
            Destroy(other);
            this.xp = this.xp + 1;
            if (this.xp >= 2) {
                this.xp = 0;
                this.level = this.level + 1;
                if (this.FireInterval > 0.12f) {
                    this.FireInterval = this.FireInterval - 0.04f;
                }
                Log("LEVEL UP! Hero reached Level " + this.level);
                emit HeroLevelUp(this.level);
            }
        }
    }
}
`

const OrbitalDrone2DScript = `namespace CyberSurvivors;

component OrbitalDrone2D : Entity {
    [Export] public float OrbitRadius = 2.4f;
    [Export] public float OrbitSpeed = 2.8f;
    [Export] public float PhaseOffset = 0.0f;
    private float angle = 0.0f;

    public void OnReady() {
        this.angle = this.PhaseOffset;
    }

    public void OnUpdate(float dt) {
        this.angle = this.angle + this.OrbitSpeed * dt;
        List<Entity> heroes = query Entities where Tag == "Player";
        if (heroes.Count > 0) {
            Entity hero = heroes[0];
            float ox = Cos(this.angle) * this.OrbitRadius;
            float oy = Sin(this.angle) * this.OrbitRadius;
            this.Position = hero.Position + Vec3(ox, oy, 0.0f);
        }

        // Zap any enemy touching the orbital ring
        List<Entity> closeFoes = query Entities
            where Tag == "Enemy" && DistanceTo(this.Position) <= 1.3f;
        foreach (Entity foe in closeFoes) {
            foe.Health = foe.Health - (45.0f * dt);
            if (foe.Health <= 0.0f) {
                Destroy(foe);
            }
        }
    }
}
`

// --- PROJECT 2: 3D FORTRESS SIEGE & TOWER DEFENSE (.osmu scripts) ---

const HeroPaladin3DScript = `namespace FortressSiege;

component HeroPaladin3D : Entity {
    [Export] public float MoveSpeed = 6.0f;
    [Export] public float CleaveRange = 3.2f;
    [Export] public float CleaveDamage = 40.0f;
    private float attackTimer = 0.0f;

    signal CleaveStrike(Entity target, float dmg);

    public void OnReady() {
        Log("HeroPaladin3D defending the Citadel! Use WASD to command.");
    }

    public void OnUpdate(float dt) {
        this.attackTimer = this.attackTimer - dt;
        float ix = InputAxisX();
        float iz = InputAxisY();
        if (ix != 0.0f || iz != 0.0f) {
            this.Position = this.Position + Vec3(ix * this.MoveSpeed * dt, 0.0f, -iz * this.MoveSpeed * dt);
        } else {
            // Patrol toward nearest gold mana crystal or closest siege attacker
            List<Entity> crystals = query Entities
                where Tag == "Coin" && Active == true
                orderby DistanceTo(this.Position) asc;
            if (crystals.Count > 0) {
                Entity c = crystals[0];
                this.LookAt(c.Position);
                float d = Distance(this.Position, c.Position);
                if (d > 0.25f) {
                    this.Position = this.Position + ((c.Position - this.Position) / d) * (this.MoveSpeed * 0.75f * dt);
                }
            }
        }

        List<Entity> raiders = query Entities
            where Tag == "Enemy" && Health > 0.0f && DistanceTo(this.Position) <= this.CleaveRange
            orderby DistanceTo(this.Position) asc;

        if (raiders.Count > 0 && this.attackTimer <= 0.0f) {
            this.attackTimer = 0.45f;
            Entity target = raiders[0];
            this.LookAt(target.Position);
            emit CleaveStrike(target, this.CleaveDamage);
        }
    }

    on CleaveStrike(Entity target, float dmg) {
        target.Health = target.Health - dmg;
        if (target.Health <= 0.0f) {
            Log("Paladin vanquished " + target.Name);
            Destroy(target);
        }
    }
}
`

const DefenseTower3DScript = `namespace FortressSiege;

component DefenseTower3D : Entity {
    [Export] public float Range = 13.5f;
    [Export] public float ReloadTime = 0.65f;
    [Export] public float ShellSpeed = 15.0f;
    private float timer = 0.0f;

    signal TowerFired(Entity enemy, float dist);

    public void OnReady() {
        Log("DefenseTower3D armed: " + this.Name);
    }

    public void OnUpdate(float dt) {
        this.timer = this.timer - dt;

        List<Entity> enemies = query Entities
            where Tag == "Enemy" && Health > 0.0f && DistanceTo(this.Position) <= this.Range
            orderby DistanceTo(this.Position) asc;

        if (enemies.Count > 0) {
            Entity primary = enemies[0];
            this.LookAt(primary.Position);
            float dist = Distance(this.Position, primary.Position);
            if (this.timer <= 0.0f && dist > 0.2f) {
                this.timer = this.ReloadTime;
                Vec3 dir = (primary.Position - this.Position) / dist;
                Spawn("CannonShell", "Projectile", "Sphere", this.Position + Vec3(0.0f, 0.6f, 0.0f), dir * this.ShellSpeed);
                emit TowerFired(primary, dist);
            }
        } else {
            this.Rotation = this.Rotation + Vec3(0.0f, 25.0f * dt, 0.0f);
        }
    }
}
`

// --- PROJECT 3: 3D DUNGEON QUEST & SIGNAL VAULT (.osmu scripts) ---

const DungeonExplorer3DScript = `namespace DungeonQuest;

component DungeonExplorer3D : Entity {
    [Export] public float Speed = 5.5f;
    private int runesActivated = 0;

    signal RuneTriggered(int count);

    public void OnReady() {
        Log("DungeonExplorer entered the Vault! Activate all 3 Runes.");
    }

    public void OnUpdate(float dt) {
        float ix = InputAxisX();
        float iz = InputAxisY();
        if (ix != 0.0f || iz != 0.0f) {
            this.Position = this.Position + Vec3(ix * this.Speed * dt, 0.0f, -iz * this.Speed * dt);
        } else {
            // Navigate toward next uncollected Rune crystal
            List<Entity> runes = query Entities
                where Tag == "Coin" && Active == true
                orderby DistanceTo(this.Position) asc;
            if (runes.Count > 0) {
                Entity nextRune = runes[0];
                this.LookAt(nextRune.Position);
                float d = Distance(this.Position, nextRune.Position);
                if (d > 0.2f) {
                    this.Position = this.Position + ((nextRune.Position - this.Position) / d) * (this.Speed * dt);
                }
            }
        }
    }

    public void OnCollision(Entity other) {
        if (other.HasTag("Coin")) {
            Destroy(other);
            this.runesActivated = this.runesActivated + 1;
            Log("Activated Ancient Rune #" + this.runesActivated);
            emit RuneTriggered(this.runesActivated);
        }
    }
}
`

const VaultGate3DScript = `namespace DungeonQuest;

component VaultGate3D : Entity {
    [Export] public float OpenHeight = 4.6f;
    [Export] public float LiftSpeed = 2.8f;
    private bool unlocked = false;

    signal VaultOpened(string gateName);

    public void OnReady() {
        Log("VaultGate3D sealed: awaiting 3 charged Obelisks.");
    }

    public void OnUpdate(float dt) {
        List<Entity> charged = query Entities
            where Tag == "Relay" && Health >= 100.0f
            orderby Name asc;

        if (charged.Count >= 2 && !this.unlocked) {
            this.unlocked = true;
            Log("All Obelisks energized! Raising " + this.Name);
            emit VaultOpened(this.Name);
        }

        if (this.unlocked && this.Position.Y < this.OpenHeight) {
            this.Position = this.Position + Vec3(0.0f, this.LiftSpeed * dt, 0.0f);
        }
    }
}
`

// BuiltinProjects builds and returns the 3 standalone OSMU Game Projects.
func BuiltinProjects() []*Project {
	return []*Project{
		buildCyberSurvivorsProject(),
		buildFortressSiegeProject(),
		buildDungeonQuestProject(),
	}
}

// BuiltinExampleScenes returns the default scenes from all built-in projects for backward compatibility.
func BuiltinExampleScenes() []*Scene {
	projs := BuiltinProjects()
	out := make([]*Scene, 0, len(projs))
	for _, p := range projs {
		if p.ActiveScene != nil {
			out = append(out, p.ActiveScene)
		}
	}
	return out
}

func buildCyberSurvivorsProject() *Project {
	sc := NewScene("Cyber Survivors 2D - Neon Arena", Mode2D)
	sc.Description = "Playable 2D Twin-Stick / Bullet-Heaven Action Game (WASD + Auto-Targeting Plasma Bolts + Orbital Drones + XP Leveling)"

	// Arena Pillars
	pillars := []struct {
		id  string
		pos engine.Vec3
	}{
		{"pillar_nw", engine.V3(-4.5, 3.2, 0)},
		{"pillar_ne", engine.V3(4.5, 3.2, 0)},
		{"pillar_sw", engine.V3(-4.5, -3.2, 0)},
		{"pillar_se", engine.V3(4.5, -3.2, 0)},
	}
	for _, p := range pillars {
		pil := engine.NewEntity(p.id, "NeonPillar", "Environment", engine.MeshSprite2D, p.pos, engine.RGB(0.22, 0.28, 0.42))
		pil.Transform.Scale = engine.V3(1.1, 1.1, 1.0)
		pil.Physics.IsStatic = true
		sc.AddEntity(pil)
	}

	// Player Cyber Hero
	hero := engine.NewEntity("hero_2d", "CyberValkyrie", "Player", engine.MeshSprite2D, engine.V3(0, 0, 0), engine.RGB(0.15, 0.95, 1.0))
	hero.Transform.Scale = engine.V3(1.15, 1.15, 1.0)
	hero.Properties["Score"] = 0.0
	hero.Scripts = []engine.ScriptAttachment{
		{ScriptPath: "scripts/CyberHero2D.osmu", ComponentName: "CyberHero2D", Properties: map[string]interface{}{"MoveSpeed": 6.5, "FireRange": 9.5, "FireInterval": 0.28}},
	}
	sc.AddEntity(hero)

	// Two Orbiting Defense Drones
	for i, phase := range []float64{0.0, 3.14159} {
		id := "drone_2d_" + string(rune('1'+i))
		dr := engine.NewEntity(id, "OrbitalDrone_"+string(rune('A'+i)), "Drone", engine.MeshRing2D, engine.V3(2.2, 0, 0), engine.RGB(0.35, 1.0, 0.65))
		dr.Transform.Scale = engine.V3(0.7, 0.7, 0.7)
		dr.Scripts = []engine.ScriptAttachment{
			{ScriptPath: "scripts/OrbitalDrone2D.osmu", ComponentName: "OrbitalDrone2D", Properties: map[string]interface{}{"OrbitRadius": 2.4, "OrbitSpeed": 3.0, "PhaseOffset": phase}},
		}
		sc.AddEntity(dr)
	}

	// Enemy Swarmers & Elite Chargers
	enemies := []struct {
		id    string
		name  string
		pos   engine.Vec3
		hp    float64
		scale float64
		col   engine.Color
	}{
		{"sw_01", "Swarmer_01", engine.V3(-7.0, 4.2, 0), 70, 0.85, engine.RGB(1.0, 0.28, 0.52)},
		{"sw_02", "Swarmer_02", engine.V3(-6.2, -4.0, 0), 70, 0.85, engine.RGB(1.0, 0.28, 0.52)},
		{"sw_03", "Swarmer_03", engine.V3(7.2, 3.8, 0), 70, 0.85, engine.RGB(1.0, 0.28, 0.52)},
		{"sw_04", "Swarmer_04", engine.V3(6.5, -4.2, 0), 70, 0.85, engine.RGB(1.0, 0.28, 0.52)},
		{"sw_05", "Swarmer_05", engine.V3(0.0, 5.8, 0), 70, 0.85, engine.RGB(1.0, 0.28, 0.52)},
		{"sw_06", "Swarmer_06", engine.V3(0.0, -5.8, 0), 70, 0.85, engine.RGB(1.0, 0.28, 0.52)},
		{"el_01", "CyberBrute_Alpha", engine.V3(-8.2, 0.0, 0), 160, 1.35, engine.RGB(0.95, 0.45, 0.18)},
		{"el_02", "CyberBrute_Omega", engine.V3(8.2, 0.0, 0), 160, 1.35, engine.RGB(0.95, 0.45, 0.18)},
	}
	for _, ec := range enemies {
		en := engine.NewEntity(ec.id, ec.name, "Enemy", engine.MeshCircle2D, ec.pos, ec.col)
		en.Health = ec.hp
		en.MaxHealth = ec.hp
		en.Transform.Scale = engine.V3(ec.scale, ec.scale, 1.0)
		sc.AddEntity(en)
	}

	// Collectible XP Gems
	gems := []engine.Vec3{
		engine.V3(-2.5, 1.8, 0),
		engine.V3(2.6, 1.8, 0),
		engine.V3(-2.6, -1.9, 0),
		engine.V3(2.5, -1.9, 0),
		engine.V3(0.0, 3.2, 0),
	}
	for i, gp := range gems {
		id := "gem_2d_" + string(rune('1'+i))
		gm := engine.NewEntity(id, "XPCrystal_0"+string(rune('1'+i)), "Coin", engine.MeshRing2D, gp, engine.RGB(1.0, 0.90, 0.22))
		gm.Transform.Scale = engine.V3(0.65, 0.65, 1.0)
		sc.AddEntity(gm)
	}

	// Visual NodeGraph for Swarm Pursuit & Score Rewards
	g := sc.NodeGraph
	nUpd := nodegraph.CreateNodeFromTemplate("n_upd", "OnUpdate", 16, 14)
	nSig := nodegraph.CreateNodeFromTemplate("n_sig", "OnSignal", 16, 134)
	nSig.Params["Signal"] = "PlasmaFired"
	nCol := nodegraph.CreateNodeFromTemplate("n_col", "OnCollision", 16, 254)
	nCol.Params["TagA"] = "Player"
	nCol.Params["TagB"] = "Coin"

	nQry := nodegraph.CreateNodeFromTemplate("n_qry", "QueryEntities", 260, 14)
	nQry.Params["Query"] = "Tag == \"Enemy\" && Health > 0 orderby Distance asc"

	nFlt := nodegraph.CreateNodeFromTemplate("n_flt", "FilterLowHealth", 260, 156)
	nFlt.Params["Property"] = "Health"
	nFlt.Params["Op"] = "<="
	nFlt.Params["Value"] = "80"

	nMov := nodegraph.CreateNodeFromTemplate("n_mov", "MoveTowards", 508, 14)
	nMov.Params["ActorTag"] = "Enemy"
	nMov.Params["GoalTag"] = "Player"
	nMov.Params["Speed"] = "2.2"

	nScr := nodegraph.CreateNodeFromTemplate("n_scr", "ModifyProperty", 508, 156)
	nScr.Params["TargetTag"] = "Player"
	nScr.Params["Property"] = "Score"
	nScr.Params["Mode"] = "Add"
	nScr.Params["Value"] = "25"

	g.Nodes = []*nodegraph.Node{nUpd, nSig, nCol, nQry, nFlt, nMov, nScr}
	g.AddLink("n_upd", "Tick", "n_qry", "In", nodegraph.SocketFlow)
	g.AddLink("n_sig", "Trigger", "n_qry", "In", nodegraph.SocketFlow)
	g.AddLink("n_sig", "Trigger", "n_flt", "In", nodegraph.SocketFlow)
	g.AddLink("n_qry", "Found", "n_mov", "In", nodegraph.SocketFlow)
	g.AddLink("n_qry", "Entities", "n_mov", "Actors", nodegraph.SocketEntityList)
	g.AddLink("n_qry", "Nearest", "n_mov", "Goal", nodegraph.SocketEntity)
	g.AddLink("n_flt", "Match", "n_scr", "In", nodegraph.SocketFlow)
	g.AddLink("n_col", "Hit", "n_scr", "In", nodegraph.SocketFlow)

	sc.ScriptFiles = []*ScriptFile{
		{Path: "scripts/CyberHero2D.osmu", Source: CyberHero2DScript},
		{Path: "scripts/OrbitalDrone2D.osmu", Source: OrbitalDrone2DScript},
	}
	sc.CompileScripts()

	rootDir := filepath.Join("projects", "01_cyber_survivors_2d")
	sc.FilePath = filepath.Join(rootDir, "scenes", "main.osmu.json")
	return &Project{
		RootDir:      rootDir,
		ManifestPath: filepath.Join(rootDir, "project.osmu.json"),
		Manifest: ProjectManifest{
			Name:         "Cyber Survivors 2D",
			Version:      "1.0.0",
			Description:  sc.Description,
			Genre:        "2D Action / Bullet-Heaven",
			DefaultMode:  "2D",
			DefaultScene: "scenes/main.osmu.json",
			Scenes:       []string{"scenes/main.osmu.json"},
			Scripts:      []string{"scripts/CyberHero2D.osmu", "scripts/OrbitalDrone2D.osmu"},
		},
		LoadedScenes: []*Scene{sc},
		ActiveScene:  sc,
	}
}

func buildFortressSiegeProject() *Project {
	sc := NewScene("3D Fortress Siege & Tower Defense", Mode3D)
	sc.Description = "3D Castle Defense with WASD Hero Paladin, 3 Auto-Targeting Artillery Towers, 3D Projectiles, and Siege Golems."

	floor := engine.NewEntity("fort_ground", "BattlefieldTerrain", "Environment", engine.MeshPlane, engine.V3(0, 0, 0), engine.RGB(0.14, 0.18, 0.22))
	floor.Transform.Scale = engine.V3(20, 1, 20)
	floor.Physics.IsStatic = true
	sc.AddEntity(floor)

	// Central Citadel Core
	citadel := engine.NewEntity("fort_citadel", "CitadelCore", "Citadel", engine.MeshPyramid, engine.V3(0, 1.4, 0), engine.RGB(0.28, 0.68, 0.98))
	citadel.Transform.Scale = engine.V3(2.0, 2.6, 2.0)
	citadel.Physics.IsStatic = true
	sc.AddEntity(citadel)

	// Fortress Perimeter Bastions
	bastions := []struct {
		id   string
		name string
		pos  engine.Vec3
	}{
		{"bastion_nw", "Bastion_NW", engine.V3(-3.8, 0.8, -3.8)},
		{"bastion_ne", "Bastion_NE", engine.V3(3.8, 0.8, -3.8)},
		{"bastion_sw", "Bastion_SW", engine.V3(-3.8, 0.8, 3.8)},
		{"bastion_se", "Bastion_SE", engine.V3(3.8, 0.8, 3.8)},
	}
	for _, b := range bastions {
		wall := engine.NewEntity(b.id, b.name, "Wall", engine.MeshCube, b.pos, engine.RGB(0.38, 0.44, 0.54))
		wall.Transform.Scale = engine.V3(1.2, 1.6, 1.2)
		wall.Physics.IsStatic = true
		sc.AddEntity(wall)
	}

	// Playable Hero Paladin
	paladin := engine.NewEntity("hero_paladin", "HeroPaladin", "Player", engine.MeshPyramid, engine.V3(0, 0.85, 2.6), engine.RGB(0.20, 0.95, 0.88))
	paladin.Transform.Scale = engine.V3(1.1, 1.5, 1.1)
	paladin.Properties["Score"] = 0.0
	paladin.Scripts = []engine.ScriptAttachment{
		{ScriptPath: "scripts/HeroPaladin3D.osmu", ComponentName: "HeroPaladin3D", Properties: map[string]interface{}{"MoveSpeed": 6.0, "CleaveRange": 3.4, "CleaveDamage": 45.0}},
	}
	sc.AddEntity(paladin)

	// 3 Automated Artillery / Laser Defense Towers
	towers := []struct {
		id   string
		name string
		pos  engine.Vec3
	}{
		{"tower_w", "ArtilleryTower_West", engine.V3(-3.5, 1.2, 0.0)},
		{"tower_e", "ArtilleryTower_East", engine.V3(3.5, 1.2, 0.0)},
		{"tower_n", "SentinelTower_North", engine.V3(0.0, 1.2, -3.5)},
	}
	for _, tw := range towers {
		tEnt := engine.NewEntity(tw.id, tw.name, "Turret", engine.MeshCylinder, tw.pos, engine.RGB(0.32, 0.92, 0.52))
		tEnt.Transform.Scale = engine.V3(1.0, 1.8, 1.0)
		tEnt.Physics.IsStatic = true
		tEnt.Scripts = []engine.ScriptAttachment{
			{ScriptPath: "scripts/DefenseTower3D.osmu", ComponentName: "DefenseTower3D", Properties: map[string]interface{}{"Range": 14.0, "ReloadTime": 0.55, "ShellSpeed": 15.0}},
		}
		sc.AddEntity(tEnt)
	}

	// Siege Enemies marching from all 4 directions
	siegeUnits := []struct {
		id    string
		name  string
		pos   engine.Vec3
		hp    float64
		scale engine.Vec3
		col   engine.Color
	}{
		{"siege_1", "SiegeGolem_North", engine.V3(-1.8, 1.0, -7.8), 160, engine.V3(1.4, 1.8, 1.4), engine.RGB(0.95, 0.28, 0.32)},
		{"siege_2", "SiegeGolem_East", engine.V3(7.8, 1.0, -1.5), 160, engine.V3(1.4, 1.8, 1.4), engine.RGB(0.95, 0.28, 0.32)},
		{"siege_3", "Raider_NW", engine.V3(-7.0, 0.75, -5.5), 85, engine.V3(1.0, 1.2, 1.0), engine.RGB(0.98, 0.48, 0.22)},
		{"siege_4", "Raider_NE", engine.V3(6.8, 0.75, -6.0), 85, engine.V3(1.0, 1.2, 1.0), engine.RGB(0.98, 0.48, 0.22)},
		{"siege_5", "Raider_SW", engine.V3(-6.8, 0.75, 5.8), 85, engine.V3(1.0, 1.2, 1.0), engine.RGB(0.98, 0.48, 0.22)},
		{"siege_6", "Raider_SE", engine.V3(6.8, 0.75, 5.8), 85, engine.V3(1.0, 1.2, 1.0), engine.RGB(0.98, 0.48, 0.22)},
		{"siege_boss", "InfernalColossus", engine.V3(0.0, 1.3, 8.2), 240, engine.V3(1.8, 2.3, 1.8), engine.RGB(0.88, 0.18, 0.42)},
	}
	for _, su := range siegeUnits {
		en := engine.NewEntity(su.id, su.name, "Enemy", engine.MeshCube, su.pos, su.col)
		en.Health = su.hp
		en.MaxHealth = su.hp
		en.Transform.Scale = su.scale
		sc.AddEntity(en)
	}

	// Mana Shrines (Collectibles)
	shrines := []engine.Vec3{
		engine.V3(-2.4, 0.7, 2.2),
		engine.V3(2.4, 0.7, 2.2),
		engine.V3(0.0, 0.7, -2.2),
	}
	for i, sp := range shrines {
		sh := engine.NewEntity("mana_"+string(rune('1'+i)), "ManaCrystal_0"+string(rune('1'+i)), "Coin", engine.MeshSphere, sp, engine.RGB(1.0, 0.86, 0.20))
		sh.Transform.Scale = engine.V3(0.75, 0.75, 0.75)
		sh.Physics.AngularVel = engine.V3(0, 90, 0)
		sc.AddEntity(sh)
	}

	// Visual NodeGraph for Siege Wave March & Bounty Score
	g := sc.NodeGraph
	nUpd := nodegraph.CreateNodeFromTemplate("f_upd", "OnUpdate", 16, 14)
	nSig := nodegraph.CreateNodeFromTemplate("f_sig", "OnSignal", 16, 134)
	nSig.Params["Signal"] = "TowerFired"
	nCol := nodegraph.CreateNodeFromTemplate("f_col", "OnCollision", 16, 254)
	nCol.Params["TagA"] = "Player"
	nCol.Params["TagB"] = "Coin"

	nQry := nodegraph.CreateNodeFromTemplate("f_qry", "QueryEntities", 260, 14)
	nQry.Params["Query"] = "Tag == \"Enemy\" && Health > 0 orderby Distance asc"

	nFlt := nodegraph.CreateNodeFromTemplate("f_flt", "FilterLowHealth", 260, 156)
	nFlt.Params["Property"] = "Health"
	nFlt.Params["Op"] = "<="
	nFlt.Params["Value"] = "90"

	nMov := nodegraph.CreateNodeFromTemplate("f_mov", "MoveTowards", 508, 14)
	nMov.Params["ActorTag"] = "Enemy"
	nMov.Params["GoalTag"] = "Citadel"
	nMov.Params["Speed"] = "1.65"

	nScr := nodegraph.CreateNodeFromTemplate("f_scr", "ModifyProperty", 508, 156)
	nScr.Params["TargetTag"] = "Player"
	nScr.Params["Property"] = "Score"
	nScr.Params["Mode"] = "Add"
	nScr.Params["Value"] = "50"

	g.Nodes = []*nodegraph.Node{nUpd, nSig, nCol, nQry, nFlt, nMov, nScr}
	g.AddLink("f_upd", "Tick", "f_qry", "In", nodegraph.SocketFlow)
	g.AddLink("f_sig", "Trigger", "f_qry", "In", nodegraph.SocketFlow)
	g.AddLink("f_sig", "Trigger", "f_flt", "In", nodegraph.SocketFlow)
	g.AddLink("f_qry", "Found", "f_mov", "In", nodegraph.SocketFlow)
	g.AddLink("f_qry", "Entities", "f_mov", "Actors", nodegraph.SocketEntityList)
	g.AddLink("f_qry", "Nearest", "f_mov", "Goal", nodegraph.SocketEntity)
	g.AddLink("f_flt", "Match", "f_scr", "In", nodegraph.SocketFlow)
	g.AddLink("f_col", "Hit", "f_scr", "In", nodegraph.SocketFlow)

	sc.ScriptFiles = []*ScriptFile{
		{Path: "scripts/HeroPaladin3D.osmu", Source: HeroPaladin3DScript},
		{Path: "scripts/DefenseTower3D.osmu", Source: DefenseTower3DScript},
	}
	sc.CompileScripts()

	rootDir := filepath.Join("projects", "02_fortress_siege_3d")
	sc.FilePath = filepath.Join(rootDir, "scenes", "main.osmu.json")
	return &Project{
		RootDir:      rootDir,
		ManifestPath: filepath.Join(rootDir, "project.osmu.json"),
		Manifest: ProjectManifest{
			Name:         "3D Fortress Siege & Tower Defense",
			Version:      "1.0.0",
			Description:  sc.Description,
			Genre:        "3D Action Strategy / Tower Defense",
			DefaultMode:  "3D",
			DefaultScene: "scenes/main.osmu.json",
			Scenes:       []string{"scenes/main.osmu.json"},
			Scripts:      []string{"scripts/HeroPaladin3D.osmu", "scripts/DefenseTower3D.osmu"},
		},
		LoadedScenes: []*Scene{sc},
		ActiveScene:  sc,
	}
}

func buildDungeonQuestProject() *Project {
	sc := NewScene("3D Dungeon Quest & Signal Vault", Mode3D)
	sc.Description = "3D Dungeon Adventure where activating 3 Ancient Runes energizes Obelisk Relays via bundled visual nodes to raise the Vault Gate."

	floor := engine.NewEntity("dng_floor", "DungeonFloor", "Environment", engine.MeshPlane, engine.V3(0, 0, 0), engine.RGB(0.13, 0.15, 0.20))
	floor.Transform.Scale = engine.V3(18, 1, 18)
	floor.Physics.IsStatic = true
	sc.AddEntity(floor)

	// Dungeon Corridor Walls
	walls := []struct {
		id    string
		name  string
		pos   engine.Vec3
		scale engine.Vec3
	}{
		{"wall_n", "DungeonWall_North", engine.V3(0, 1.2, -5.5), engine.V3(12, 2.4, 0.6)},
		{"wall_s", "DungeonWall_South", engine.V3(0, 1.2, 5.5), engine.V3(12, 2.4, 0.6)},
		{"pillar_1", "StoneColumn_NW", engine.V3(-2.5, 1.3, -2.5), engine.V3(0.9, 2.6, 0.9)},
		{"pillar_2", "StoneColumn_SW", engine.V3(-2.5, 1.3, 2.5), engine.V3(0.9, 2.6, 0.9)},
	}
	for _, w := range walls {
		wl := engine.NewEntity(w.id, w.name, "Wall", engine.MeshCube, w.pos, engine.RGB(0.32, 0.36, 0.45))
		wl.Transform.Scale = w.scale
		wl.Physics.IsStatic = true
		sc.AddEntity(wl)
	}

	// Playable Dungeon Explorer
	explorer := engine.NewEntity("dng_hero", "DungeonExplorer", "Player", engine.MeshPyramid, engine.V3(-5.2, 0.85, 0.0), engine.RGB(0.22, 0.92, 0.98))
	explorer.Transform.Scale = engine.V3(1.1, 1.5, 1.1)
	explorer.Properties["Score"] = 0.0
	explorer.Scripts = []engine.ScriptAttachment{
		{ScriptPath: "scripts/DungeonExplorer3D.osmu", ComponentName: "DungeonExplorer3D", Properties: map[string]interface{}{"Speed": 5.5}},
	}
	sc.AddEntity(explorer)

	// 3 Ancient Rune Crystals
	runes := []struct {
		id   string
		name string
		pos  engine.Vec3
	}{
		{"rune_1", "Rune_Crimson", engine.V3(-1.2, 0.75, -1.8)},
		{"rune_2", "Rune_Azure", engine.V3(0.2, 0.75, 1.8)},
		{"rune_3", "Rune_Solar", engine.V3(2.0, 0.75, 0.0)},
	}
	for _, rn := range runes {
		rEnt := engine.NewEntity(rn.id, rn.name, "Coin", engine.MeshSphere, rn.pos, engine.RGB(1.0, 0.82, 0.22))
		rEnt.Transform.Scale = engine.V3(0.8, 0.8, 0.8)
		sc.AddEntity(rEnt)
	}

	// 2 Obelisk Relays + Vault Gate
	ob1 := engine.NewEntity("obelisk_n", "ObeliskRelay_North", "Relay", engine.MeshCylinder, engine.V3(3.2, 1.2, -2.8), engine.RGB(0.32, 0.95, 0.58))
	ob1.Transform.Scale = engine.V3(0.9, 2.2, 0.9)
	ob1.Health = 50
	ob1.Physics.IsStatic = true
	sc.AddEntity(ob1)

	ob2 := engine.NewEntity("obelisk_s", "ObeliskRelay_South", "Relay", engine.MeshCylinder, engine.V3(3.2, 1.2, 2.8), engine.RGB(0.32, 0.95, 0.58))
	ob2.Transform.Scale = engine.V3(0.9, 2.2, 0.9)
	ob2.Health = 50
	ob2.Physics.IsStatic = true
	sc.AddEntity(ob2)

	gate := engine.NewEntity("vault_gate", "AncientVaultGate", "Gate", engine.MeshCube, engine.V3(5.2, 1.4, 0.0), engine.RGB(0.95, 0.32, 0.42))
	gate.Transform.Scale = engine.V3(0.8, 2.8, 4.8)
	gate.Physics.IsStatic = true
	gate.Scripts = []engine.ScriptAttachment{
		{ScriptPath: "scripts/VaultGate3D.osmu", ComponentName: "VaultGate3D", Properties: map[string]interface{}{"OpenHeight": 4.8, "LiftSpeed": 2.8}},
	}
	sc.AddEntity(gate)

	g := sc.NodeGraph
	nCol := nodegraph.CreateNodeFromTemplate("np_col", "OnCollision", 16, 20)
	nCol.Params["TagA"] = "Player"
	nCol.Params["TagB"] = "Coin"
	nSig := nodegraph.CreateNodeFromTemplate("np_sig", "OnSignal", 16, 155)
	nSig.Params["Signal"] = "RuneTriggered"

	nQry := nodegraph.CreateNodeFromTemplate("np_qry", "QueryEntities", 260, 20)
	nQry.Params["Query"] = "Tag == \"Relay\""

	nCharge := nodegraph.CreateNodeFromTemplate("np_chg", "ModifyProperty", 508, 20)
	nCharge.Params["TargetTag"] = "Relay"
	nCharge.Params["Property"] = "Health"
	nCharge.Params["Mode"] = "Set"
	nCharge.Params["Value"] = "100"

	nEmit := nodegraph.CreateNodeFromTemplate("np_emt", "EmitSignal", 508, 155)
	nEmit.Params["Signal"] = "VaultCleared"
	nEmit.Params["Arg"] = "Unlocked"

	g.Nodes = []*nodegraph.Node{nCol, nSig, nQry, nCharge, nEmit}
	g.AddLink("np_col", "Hit", "np_qry", "In", nodegraph.SocketFlow)
	g.AddLink("np_sig", "Trigger", "np_qry", "In", nodegraph.SocketFlow)
	g.AddLink("np_qry", "Found", "np_chg", "In", nodegraph.SocketFlow)
	g.AddLink("np_qry", "Entities", "np_chg", "Targets", nodegraph.SocketEntityList)
	g.AddLink("np_qry", "Found", "np_emt", "In", nodegraph.SocketFlow)
	g.AddLink("np_qry", "Nearest", "np_emt", "Target", nodegraph.SocketEntity)

	sc.ScriptFiles = []*ScriptFile{
		{Path: "scripts/DungeonExplorer3D.osmu", Source: DungeonExplorer3DScript},
		{Path: "scripts/VaultGate3D.osmu", Source: VaultGate3DScript},
	}
	sc.CompileScripts()

	rootDir := filepath.Join("projects", "03_dungeon_quest_3d")
	sc.FilePath = filepath.Join(rootDir, "scenes", "main.osmu.json")
	return &Project{
		RootDir:      rootDir,
		ManifestPath: filepath.Join(rootDir, "project.osmu.json"),
		Manifest: ProjectManifest{
			Name:         "3D Dungeon Quest & Signal Vault",
			Version:      "1.0.0",
			Description:  sc.Description,
			Genre:        "3D Adventure / Signal Puzzle",
			DefaultMode:  "3D",
			DefaultScene: "scenes/main.osmu.json",
			Scenes:       []string{"scenes/main.osmu.json"},
			Scripts:      []string{"scripts/DungeonExplorer3D.osmu", "scripts/VaultGate3D.osmu"},
		},
		LoadedScenes: []*Scene{sc},
		ActiveScene:  sc,
	}
}

// MaterializeExamples writes the 3 standalone example projects to `projects/` (and cleans up legacy `examples/`).
func MaterializeExamples(baseDir string) error {
	_ = os.RemoveAll(filepath.Join(baseDir, "examples"))
	for _, proj := range BuiltinProjects() {
		fullProj := *proj
		if !filepath.IsAbs(fullProj.RootDir) {
			fullProj.RootDir = filepath.Join(baseDir, proj.RootDir)
			fullProj.ManifestPath = filepath.Join(fullProj.RootDir, "project.osmu.json")
		}
		if err := fullProj.SaveProject(); err != nil {
			return err
		}
	}
	return nil
}
