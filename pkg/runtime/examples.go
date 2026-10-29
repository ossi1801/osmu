package runtime

import (
	"fmt"
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

// --- PROJECT 4: MINESWEEPER 2D (.osmu scripts) ---

const MinesweeperBoardScript = `namespace Minesweeper;

component MinesweeperBoard : Entity {
    [Export] public float StepSize = 1.6f;
    [Export] public float BoardLimit = 4.0f;
    [Export] public float AutoHintInterval = 0.55f;

    private float moveCooldown = 0.0f;
    private float autoTimer = 0.0f;
    private bool eWasDown = false;
    private bool fWasDown = false;
    private bool hWasDown = false;
    private bool rWasDown = false;
    private bool manualControl = false;
    private bool gameOver = false;
    private bool gameWon = false;

    signal TileRevealed(Entity tile, int adjMines);
    signal FlagToggled(Entity tile, int totalFlags);
    signal MineDetonated(Entity mineTile);
    signal BoardCleared(int safeCount);
    signal EntityClicked(Entity target);
    signal EntityRightClicked(Entity target);

    public void OnReady() {
        Log("Minesweeper ready! LMB/E: Reveal | RMB/F: Flag | WASD: Move | H: AI Hint | R: Reset");
        this.ResetBoard();
    }

    public void ResetBoard() {
        this.gameOver = false;
        this.gameWon = false;
        this.autoTimer = 0.0f;
        this.Color = Color(0.25f, 0.95f, 1.0f, 1.0f);

        List<Entity> allTiles = query Entities where Tag == "Tile";
        foreach (Entity t in allTiles) {
            t.Health = 100.0f;
            t.MaxHealth = 100.0f;
            t.Damage = 0.0f;
            t.Label = ".";
            t.Color = Color(0.32f, 0.39f, 0.52f, 1.0f);
        }

        // Start at top-left safe corner (-4.0, 4.0) and open initial safe cascade
        this.Position = Vec3(-4.0f, 4.0f, 0.0f);
        this.RevealAtPos(this.Position);
    }

    public void OnUpdate(float dt) {
        bool rDown = IsKeyDown("R");
        if (rDown && !this.rWasDown) {
            this.manualControl = true;
            this.ResetBoard();
        }
        this.rWasDown = rDown;

        if (this.gameOver || this.gameWon) {
            return;
        }

        this.moveCooldown = this.moveCooldown - dt;
        float ix = InputAxisX();
        float iy = InputAxisY();
        if (ix != 0.0f || iy != 0.0f) {
            this.manualControl = true;
            if (this.moveCooldown <= 0.0f) {
                float nx = this.Position.X;
                float ny = this.Position.Y;
                if (ix > 0.1f) {
                    nx = nx + this.StepSize;
                }
                if (ix < -0.1f) {
                    nx = nx - this.StepSize;
                }
                if (iy > 0.1f) {
                    ny = ny + this.StepSize;
                }
                if (iy < -0.1f) {
                    ny = ny - this.StepSize;
                }
                nx = Clamp(nx, -this.BoardLimit, this.BoardLimit);
                ny = Clamp(ny, -this.BoardLimit, this.BoardLimit);
                this.Position = Vec3(nx, ny, 0.0f);
                this.moveCooldown = 0.16f;
            }
        } else {
            this.moveCooldown = 0.0f;
        }

        bool eDown = IsKeyDown("E") || IsKeyDown("ENTER");
        if (eDown && !this.eWasDown) {
            this.manualControl = true;
            this.RevealAtPos(this.Position);
        }
        this.eWasDown = eDown;

        bool fDown = IsKeyDown("F");
        if (fDown && !this.fWasDown) {
            this.manualControl = true;
            this.ToggleFlagAtPos(this.Position);
        }
        this.fWasDown = fDown;

        bool hDown = IsKeyDown("H");
        if (hDown && !this.hWasDown) {
            this.PerformAIHintStep();
        }
        this.hWasDown = hDown;

        // If no manual input yet, demonstrate logical Minesweeper solving step-by-step
        if (!this.manualControl) {
            this.autoTimer = this.autoTimer + dt;
            if (this.autoTimer >= this.AutoHintInterval) {
                this.autoTimer = 0.0f;
                this.PerformAIHintStep();
            }
        }
    }

    public void RevealAtPos(Vec3 pos) {
        List<Entity> found = query Entities
            where Tag == "Tile" && DistanceTo(pos) <= 0.6f
            orderby DistanceTo(pos) asc;
        if (found.Count > 0) {
            this.RevealTileAndCascade(found[0]);
        }
    }

    public void ToggleFlagAtPos(Vec3 pos) {
        List<Entity> found = query Entities
            where Tag == "Tile" && DistanceTo(pos) <= 0.6f
            orderby DistanceTo(pos) asc;
        if (found.Count > 0) {
            this.ToggleFlagTile(found[0]);
        }
    }

    public void ToggleFlagTile(Entity tile) {
        if (tile.Health < 10.0f) {
            return;
        }
        if (tile.Health > 60.0f) {
            tile.Health = 50.0f;
            tile.Label = "F";
            tile.Color = Color(0.88f, 0.54f, 0.14f, 1.0f);
        } else {
            tile.Health = 100.0f;
            tile.Label = ".";
            tile.Color = Color(0.32f, 0.39f, 0.52f, 1.0f);
        }
        List<Entity> flags = query Entities
            where Tag == "Tile" && Health > 40.0f && Health < 60.0f;
        Log("Flag toggled on " + tile.Name + " (Total Flags: " + flags.Count + ")");
        emit FlagToggled(tile, flags.Count);
    }

    public void RevealSingleTile(Entity tile) {
        if (tile.Health < 60.0f) {
            return;
        }
        if (tile.Speed > 0.5f) {
            tile.Health = 0.0f;
            tile.Label = "*";
            tile.Color = Color(0.95f, 0.18f, 0.22f, 1.0f);
            this.gameOver = true;
            this.Color = Color(1.0f, 0.22f, 0.25f, 1.0f);

            List<Entity> allMines = query Entities
                where Tag == "Tile" && Speed > 0.5f;
            foreach (Entity m in allMines) {
                m.Health = 0.0f;
                m.Label = "*";
                m.Color = Color(0.95f, 0.18f, 0.22f, 1.0f);
            }
            Log("MINE DETONATED at " + tile.Name + "! Press R to reset.");
            emit MineDetonated(tile);
            return;
        }

        Vec3 tPos = tile.Position;
        List<Entity> adjMines = query Entities
            where Tag == "Tile" && Speed > 0.5f && DistanceTo(tPos) > 0.1f && DistanceTo(tPos) <= 2.5f;
        int adj = adjMines.Count;

        tile.Health = 0.0f;
        tile.Damage = adj;

        if (adj == 0) {
            tile.MaxHealth = 1.0f;
            tile.Label = "";
            tile.Color = Color(0.09f, 0.11f, 0.15f, 1.0f);
        }
        if (adj == 1) {
            tile.Label = "1";
            tile.Color = Color(0.16f, 0.38f, 0.72f, 1.0f);
        }
        if (adj == 2) {
            tile.Label = "2";
            tile.Color = Color(0.16f, 0.54f, 0.32f, 1.0f);
        }
        if (adj == 3) {
            tile.Label = "3";
            tile.Color = Color(0.72f, 0.24f, 0.28f, 1.0f);
        }
        if (adj >= 4) {
            tile.Label = "" + adj;
            tile.Color = Color(0.52f, 0.24f, 0.72f, 1.0f);
        }

        emit TileRevealed(tile, adj);
    }

    public void RevealTileAndCascade(Entity startTile) {
        if (startTile.Health < 60.0f) {
            return;
        }
        this.RevealSingleTile(startTile);
        if (this.gameOver) {
            return;
        }

        // Flood-fill cascade for connected 0-mine tiles using .osmu spatial queries
        bool expanding = true;
        while (expanding) {
            List<Entity> zeroFront = query Entities
                where Tag == "Tile" && Health == 0.0f && MaxHealth == 1.0f;
            if (zeroFront.Count == 0) {
                expanding = false;
            } else {
                foreach (Entity zt in zeroFront) {
                    zt.MaxHealth = 0.0f;
                    Vec3 zPos = zt.Position;
                    List<Entity> neighbors = query Entities
                        where Tag == "Tile" && Health > 60.0f && DistanceTo(zPos) > 0.1f && DistanceTo(zPos) <= 2.5f;
                    foreach (Entity nb in neighbors) {
                        this.RevealSingleTile(nb);
                    }
                }
            }
        }

        this.CheckWinCondition();
    }

    public void CheckWinCondition() {
        List<Entity> hiddenSafe = query Entities
            where Tag == "Tile" && Speed < 0.5f && Health > 10.0f;
        if (hiddenSafe.Count == 0 && !this.gameWon && !this.gameOver) {
            this.gameWon = true;
            this.Color = Color(0.25f, 1.0f, 0.55f, 1.0f);
            List<Entity> unflaggedMines = query Entities
                where Tag == "Tile" && Speed > 0.5f && Health > 60.0f;
            foreach (Entity m in unflaggedMines) {
                m.Health = 50.0f;
                m.Label = "F";
                m.Color = Color(0.22f, 0.78f, 0.42f, 1.0f);
            }
            List<Entity> safeTiles = query Entities where Tag == "Tile" && Speed < 0.5f;
            Log("BOARD CLEARED! All " + safeTiles.Count + " safe tiles swept!");
            emit BoardCleared(safeTiles.Count);
        }
    }

    public void PerformAIHintStep() {
        if (this.gameOver || this.gameWon) {
            return;
        }

        // Rule 1 & Rule 2 constraint-satisfaction over revealed numbered tiles
        List<Entity> numbered = query Entities
            where Tag == "Tile" && Health == 0.0f && Damage > 0.0f
            orderby Name asc;

        foreach (Entity rt in numbered) {
            Vec3 rPos = rt.Position;
            List<Entity> hiddenNbs = query Entities
                where Tag == "Tile" && Health > 60.0f && DistanceTo(rPos) > 0.1f && DistanceTo(rPos) <= 2.5f;
            List<Entity> flagNbs = query Entities
                where Tag == "Tile" && Health > 40.0f && Health < 60.0f && DistanceTo(rPos) > 0.1f && DistanceTo(rPos) <= 2.5f;

            if (hiddenNbs.Count > 0) {
                // Rule 1: If hiddenNbs + flagNbs == number on tile, all hiddenNbs are guaranteed mines
                if (hiddenNbs.Count + flagNbs.Count == rt.Damage) {
                    Entity mineTarget = hiddenNbs[0];
                    this.Position = mineTarget.Position;
                    this.ToggleFlagTile(mineTarget);
                    return;
                }
                // Rule 2: If flagNbs == number on tile, all remaining hiddenNbs are guaranteed safe
                if (flagNbs.Count == rt.Damage) {
                    Entity safeTarget = hiddenNbs[0];
                    this.Position = safeTarget.Position;
                    Log("AI Solver: Deduced " + safeTarget.Name + " is safe from " + rt.Name);
                    this.RevealTileAndCascade(safeTarget);
                    return;
                }
            }
        }

        // Fallback: pick nearest unrevealed safe tile
        List<Entity> fallbackSafe = query Entities
            where Tag == "Tile" && Speed < 0.5f && Health > 60.0f
            orderby DistanceTo(this.Position) asc;
        if (fallbackSafe.Count > 0) {
            Entity nextSafe = fallbackSafe[0];
            this.Position = nextSafe.Position;
            this.RevealTileAndCascade(nextSafe);
        }
    }

    on EntityClicked(Entity target) {
        if (target.HasTag("Tile") && !this.gameOver && !this.gameWon) {
            this.manualControl = true;
            this.Position = target.Position;
            this.RevealTileAndCascade(target);
        }
    }

    on EntityRightClicked(Entity target) {
        if (target.HasTag("Tile") && !this.gameOver && !this.gameWon) {
            this.manualControl = true;
            this.Position = target.Position;
            this.ToggleFlagTile(target);
        }
    }
}
`

const MineScannerHUDScript = `namespace Minesweeper;

component MineScannerHUD : Entity {
    [Export] public int TotalMines = 6;
    private int revealedCount = 0;
    private int flagCount = 0;

    signal TileRevealed(Entity tile, int adjMines);
    signal FlagToggled(Entity tile, int totalFlags);
    signal MineDetonated(Entity mineTile);
    signal BoardCleared(int safeCount);

    public void OnReady() {
        this.revealedCount = 0;
        this.flagCount = 0;
        this.Label = "MINES: " + this.TotalMines;
        this.Color = Color(0.20f, 0.65f, 0.95f, 1.0f);
    }

    public void OnUpdate(float dt) {
        List<Entity> cleared = query Entities
            where Tag == "Tile" && Health == 0.0f && Speed < 0.5f;
        this.revealedCount = cleared.Count;
        this.Score = this.revealedCount;
    }

    on TileRevealed(Entity tile, int adjMines) {
        List<Entity> flags = query Entities
            where Tag == "Tile" && Health > 40.0f && Health < 60.0f;
        int rem = this.TotalMines - flags.Count;
        this.Label = "M:" + rem;
    }

    on FlagToggled(Entity tile, int totalFlags) {
        this.flagCount = totalFlags;
        int rem = this.TotalMines - totalFlags;
        this.Label = "M:" + rem;
        this.Color = Color(0.95f, 0.68f, 0.18f, 1.0f);
    }

    on MineDetonated(Entity mineTile) {
        this.Label = "BOOM";
        this.Color = Color(0.98f, 0.20f, 0.24f, 1.0f);
    }

    on BoardCleared(int safeCount) {
        this.Label = "WIN!";
        this.Color = Color(0.22f, 0.95f, 0.48f, 1.0f);
    }
}
`

const FlappyBirdControllerScript = `namespace FlappyBird;

component FlappyBirdController : Entity {
    [Export] public float Gravity = 22.0f;
    [Export] public float FlapVelocity = 8.2f;
    [Export] public float PipeSpeed = 4.5f;
    [Export] public float PipeGap = 3.6f;
    [Export] public float PipeWidth = 1.6f;
    [Export] public float PipeHeight = 8.0f;
    [Export] public float BirdRadius = 0.55f;
    [Export] public float FloorY = -5.8f;
    [Export] public float CeilingY = 5.8f;

    private float vy = 0.0f;
    private int score = 0;
    private int bestScore = 0;
    private bool gameOver = false;
    private bool manualControl = false;
    private bool flapWasDown = false;
    private bool rWasDown = false;
    private float aiCooldown = 0.0f;
    private int rngSeed = 7;

    signal BirdFlapped(float yPos, float velY);
    signal ScoreIncreased(int newScore, int best);
    signal GameOver(int finalScore, int best);
    signal GameReset();
    signal EntityClicked(Entity target);

    public void OnReady() {
        Log("Flappy Bird ready! SPACE/W/UP/Click: Flap | R: Reset (Auto-Pilot active until input)");
        this.ResetGame();
    }

    public void ResetGame() {
        this.gameOver = false;
        this.score = 0;
        this.vy = 2.5f;
        this.aiCooldown = 0.0f;
        this.Position = Vec3(-3.5f, 0.5f, 0.2f);
        this.Rotation = Vec3(0.0f, 0.0f, 12.0f);
        this.Color = Color(1.0f, 0.85f, 0.15f, 1.0f);
        this.Label = "0";
        this.Score = 0.0f;

        // Reset pipe pairs across the screen at X = 3.5, 9.0, 14.5, 20.0
        List<Entity> topPipes = query Entities
            where Tag == "PipeTop"
            orderby Name asc;
        List<Entity> botPipes = query Entities
            where Tag == "PipeBottom"
            orderby Name asc;

        float startX = 3.5f;
        float spacing = 5.5f;
        int idx = 0;
        while (idx < topPipes.Count && idx < botPipes.Count) {
            Entity pt = topPipes[idx];
            Entity pb = botPipes[idx];
            float px = startX + idx * spacing;
            float gapY = this.NextGapY(idx);
            float halfGap = this.PipeGap * 0.5f;
            float halfH = this.PipeHeight * 0.5f;

            pt.Position = Vec3(px, gapY + halfGap + halfH, 0.0f);
            pb.Position = Vec3(px, gapY - halfGap - halfH, 0.0f);
            pt.Damage = gapY;
            pb.Damage = gapY;
            pt.MaxHealth = 0.0f;
            pb.MaxHealth = 0.0f;
            pt.Color = Color(0.22f, 0.78f, 0.35f, 1.0f);
            pb.Color = Color(0.22f, 0.78f, 0.35f, 1.0f);
            idx = idx + 1;
        }

        emit GameReset();
    }

    public float NextGapY(int step) {
        this.rngSeed = this.rngSeed + 1 + step;
        float wave = Sin(this.rngSeed * 1.85f) * 2.1f + Cos(this.rngSeed * 0.95f) * 0.8f;
        return Clamp(wave, -2.4f, 2.4f);
    }

    public void Flap() {
        if (this.gameOver) {
            return;
        }
        this.vy = this.FlapVelocity;
        this.Rotation = Vec3(0.0f, 0.0f, 25.0f);
        emit BirdFlapped(this.Position.Y, this.vy);
    }

    public void OnUpdate(float dt) {
        bool rDown = IsKeyDown("R");
        if (rDown && !this.rWasDown) {
            this.ResetGame();
        }
        this.rWasDown = rDown;

        bool keyFlap = IsKeyDown("SPACE") || IsKeyDown("W") || IsKeyDown("UP") || IsKeyDown("E") || IsKeyDown("ENTER");
        if (keyFlap && !this.flapWasDown) {
            this.manualControl = true;
            if (this.gameOver) {
                this.ResetGame();
            } else {
                this.Flap();
            }
        }
        this.flapWasDown = keyFlap;

        if (this.gameOver) {
            return;
        }

        // Scroll background clouds slowly for parallax depth
        List<Entity> clouds = query Entities where Tag == "Cloud";
        foreach (Entity cl in clouds) {
            float cx = cl.Position.X - (this.PipeSpeed * 0.32f * dt);
            if (cx < -13.0f) {
                cx = 13.0f;
            }
            cl.Position = Vec3(cx, cl.Position.Y, cl.Position.Z);
        }

        // Scroll ground segments for continuous motion
        List<Entity> grounds = query Entities where Tag == "GroundTile";
        foreach (Entity gr in grounds) {
            float gx = gr.Position.X - (this.PipeSpeed * dt);
            if (gx < -12.0f) {
                gx = gx + 24.0f;
            }
            gr.Position = Vec3(gx, gr.Position.Y, gr.Position.Z);
        }

        // Auto-pilot AI when player has not pressed any manual flap key yet
        if (!this.manualControl) {
            this.aiCooldown = this.aiCooldown - dt;
            float targetY = 0.2f;

            // Find the nearest upcoming pipe that hasn't been passed yet
            List<Entity> upcoming = query Entities
                where Tag == "PipeTop" && Position.X >= (this.Position.X - 1.1f)
                orderby Position.X asc;

            if (upcoming.Count > 0) {
                Entity nextPipe = upcoming[0];
                targetY = nextPipe.Damage - 0.35f;
            }

            if (this.aiCooldown <= 0.0f) {
                if (this.Position.Y < targetY && this.vy < 1.8f) {
                    this.Flap();
                    this.aiCooldown = 0.14f;
                } else if (this.Position.Y < (this.FloorY + 1.5f) && this.vy < 0.0f) {
                    this.Flap();
                    this.aiCooldown = 0.14f;
                }
            }
        }

        // Apply gravity & vertical velocity to Bird
        this.vy = this.vy - (this.Gravity * dt);
        this.vy = Clamp(this.vy, -14.0f, 12.0f);
        float newY = this.Position.Y + (this.vy * dt);
        this.Position = Vec3(this.Position.X, newY, this.Position.Z);

        // Tilt bird nose up when rising, down when falling
        float pitchAngle = Clamp(this.vy * 4.5f, -65.0f, 30.0f);
        this.Rotation = Vec3(0.0f, 0.0f, pitchAngle);

        // Move pipes left, recycle off-screen pipes, and check collisions & scoring
        List<Entity> topPipes = query Entities
            where Tag == "PipeTop"
            orderby Name asc;
        List<Entity> botPipes = query Entities
            where Tag == "PipeBottom"
            orderby Name asc;

        float halfW = (this.PipeWidth * 0.5f) + this.BirdRadius - 0.12f;
        float halfH = (this.PipeHeight * 0.5f) + this.BirdRadius - 0.12f;

        int i = 0;
        while (i < topPipes.Count && i < botPipes.Count) {
            Entity pt = topPipes[i];
            Entity pb = botPipes[i];

            float nx = pt.Position.X - (this.PipeSpeed * dt);

            // Recycle pipe pair to the right when it exits left of the screen
            if (nx < -11.0f) {
                nx = nx + 22.0f;
                float newGapY = this.NextGapY(this.score + i);
                float halfGap = this.PipeGap * 0.5f;
                float pipeHalfH = this.PipeHeight * 0.5f;
                pt.Position = Vec3(nx, newGapY + halfGap + pipeHalfH, 0.0f);
                pb.Position = Vec3(nx, newGapY - halfGap - pipeHalfH, 0.0f);
                pt.Damage = newGapY;
                pb.Damage = newGapY;
                pt.MaxHealth = 0.0f;
                pb.MaxHealth = 0.0f;
                pt.Color = Color(0.22f, 0.78f, 0.35f, 1.0f);
                pb.Color = Color(0.22f, 0.78f, 0.35f, 1.0f);
            } else {
                pt.Position = Vec3(nx, pt.Position.Y, 0.0f);
                pb.Position = Vec3(nx, pb.Position.Y, 0.0f);
            }

            // Score point when bird passes the center of the pipe pair
            if (pt.MaxHealth < 0.5f && nx < this.Position.X) {
                pt.MaxHealth = 1.0f;
                pb.MaxHealth = 1.0f;
                pt.Color = Color(0.32f, 0.92f, 0.48f, 1.0f);
                pb.Color = Color(0.32f, 0.92f, 0.48f, 1.0f);
                this.score = this.score + 1;
                if (this.score > this.bestScore) {
                    this.bestScore = this.score;
                }
                this.Score = this.score;
                this.Label = "" + this.score;
                Log("Pipe cleared! Score: " + this.score + " (Best: " + this.bestScore + ")");
                emit ScoreIncreased(this.score, this.bestScore);
            }

            // AABB Collision check against Top Pipe and Bottom Pipe
            float dx = this.Position.X - nx;
            if (dx < 0.0f) {
                dx = -dx;
            }
            if (dx < halfW) {
                float dyTop = this.Position.Y - pt.Position.Y;
                if (dyTop < 0.0f) {
                    dyTop = -dyTop;
                }
                float dyBot = this.Position.Y - pb.Position.Y;
                if (dyBot < 0.0f) {
                    dyBot = -dyBot;
                }
                if (dyTop < halfH || dyBot < halfH) {
                    pt.Color = Color(0.95f, 0.25f, 0.25f, 1.0f);
                    pb.Color = Color(0.95f, 0.25f, 0.25f, 1.0f);
                    this.TriggerGameOver("Hit " + pt.Name);
                    return;
                }
            }

            i = i + 1;
        }

        // Check floor / ceiling bounds
        if (this.Position.Y <= this.FloorY) {
            this.Position = Vec3(this.Position.X, this.FloorY, this.Position.Z);
            this.TriggerGameOver("Hit the ground");
            return;
        }
        if (this.Position.Y >= this.CeilingY) {
            this.Position = Vec3(this.Position.X, this.CeilingY, this.Position.Z);
            this.vy = 0.0f;
        }
    }

    public void TriggerGameOver(string reason) {
        if (this.gameOver) {
            return;
        }
        this.gameOver = true;
        this.vy = 0.0f;
        this.Color = Color(0.98f, 0.22f, 0.25f, 1.0f);
        this.Label = "X";
        Log("GAME OVER (" + reason + ")! Final Score: " + this.score + " | Press R or SPACE to restart.");
        emit GameOver(this.score, this.bestScore);
    }

    on EntityClicked(Entity target) {
        this.manualControl = true;
        if (this.gameOver) {
            this.ResetGame();
        } else {
            this.Flap();
        }
    }
}
`

const FlappyScoreHUDScript = `namespace FlappyBird;

component FlappyScoreHUD : Entity {
    private int currentScore = 0;
    private int bestScore = 0;

    signal ScoreIncreased(int newScore, int best);
    signal GameOver(int finalScore, int best);
    signal GameReset();

    public void OnReady() {
        this.currentScore = 0;
        this.Label = "SCORE: 0";
        this.Color = Color(0.20f, 0.68f, 0.98f, 1.0f);
    }

    on ScoreIncreased(int newScore, int best) {
        this.currentScore = newScore;
        this.bestScore = best;
        this.Score = newScore;
        this.Label = "S:" + newScore + " B:" + best;
        this.Color = Color(0.22f, 0.88f, 0.52f, 1.0f);
    }

    on GameOver(int finalScore, int best) {
        this.Label = "DEAD S:" + finalScore;
        this.Color = Color(0.95f, 0.24f, 0.28f, 1.0f);
    }

    on GameReset() {
        this.currentScore = 0;
        this.Label = "S:0 B:" + this.bestScore;
        this.Color = Color(0.20f, 0.68f, 0.98f, 1.0f);
    }
}
`

// BuiltinProjects builds and returns the 5 standalone OSMU Game Projects.
func BuiltinProjects() []*Project {
	return []*Project{
		buildCyberSurvivorsProject(),
		buildFortressSiegeProject(),
		buildDungeonQuestProject(),
		buildMinesweeperProject(),
		buildFlappyBirdProject(),
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

func buildMinesweeperProject() *Project {
	sc := NewScene("Minesweeper 2D - Tactical Grid", Mode2D)
	sc.Description = "Playable 2D/3D Minesweeper with Spatial Entity Queries, Flood-Fill Cascade, Mouse/WASD Controls & AI Deduction Solver"

	mineCoords := map[[2]int]bool{
		{0, 3}: true,
		{1, 5}: true,
		{2, 2}: true,
		{3, 4}: true,
		{4, 1}: true,
		{5, 5}: true,
	}

	for r := 0; r < 6; r++ {
		for c := 0; c < 6; c++ {
			id := fmt.Sprintf("tile_%d_%d", r, c)
			name := fmt.Sprintf("Tile_%d_%d", r, c)
			px := -4.0 + float64(c)*1.6
			py := 4.0 - float64(r)*1.6
			tile := engine.NewEntity(id, name, "Tile", engine.MeshSprite2D, engine.V3(px, py, 0), engine.RGB(0.32, 0.39, 0.52))
			tile.Transform.Scale = engine.V3(1.65, 1.65, 0.45)
			tile.Render.Label = "."
			tile.Physics.Enabled = false
			tile.Physics.IsStatic = true
			tile.Health = 100.0
			tile.MaxHealth = 100.0
			if mineCoords[[2]int{r, c}] {
				tile.Speed = 1.0
			} else {
				tile.Speed = 0.0
			}
			tile.Properties["Damage"] = 0.0
			sc.AddEntity(tile)
		}
	}

	scanner := engine.NewEntity("scanner_hud", "MineScannerHUD", "HUD", engine.MeshSprite2D, engine.V3(-6.8, 3.2, 0), engine.RGB(0.20, 0.65, 0.95))
	scanner.Transform.Scale = engine.V3(2.1, 1.65, 0.5)
	scanner.Physics.Enabled = false
	scanner.Physics.IsStatic = true
	scanner.Render.Label = "M:6"
	scanner.Properties["Score"] = 0.0
	scanner.Scripts = []engine.ScriptAttachment{
		{ScriptPath: "scripts/MineScannerHUD.osmu", ComponentName: "MineScannerHUD", Properties: map[string]interface{}{"TotalMines": 6}},
	}
	sc.AddEntity(scanner)

	cursor := engine.NewEntity("board_cursor", "BoardCursor", "Player", engine.MeshSprite2D, engine.V3(-4.0, 4.0, 0.2), engine.RGB(0.25, 0.95, 1.0))
	cursor.Transform.Scale = engine.V3(1.78, 1.78, 0.55)
	cursor.Render.Wireframe = true
	cursor.Physics.Enabled = false
	cursor.Physics.IsStatic = true
	cursor.Properties["Score"] = 0.0
	cursor.Scripts = []engine.ScriptAttachment{
		{
			ScriptPath:    "scripts/MinesweeperBoard.osmu",
			ComponentName: "MinesweeperBoard",
			Properties: map[string]interface{}{
				"StepSize":         1.6,
				"BoardLimit":       4.0,
				"AutoHintInterval": 0.55,
			},
		},
	}
	sc.AddEntity(cursor)

	g := sc.NodeGraph
	nSigRev := nodegraph.CreateNodeFromTemplate("ms_sig_rev", "OnSignal", 16, 18)
	nSigRev.Params["Signal"] = "TileRevealed"
	nSigFlg := nodegraph.CreateNodeFromTemplate("ms_sig_flg", "OnSignal", 16, 148)
	nSigFlg.Params["Signal"] = "FlagToggled"

	nQrySafe := nodegraph.CreateNodeFromTemplate("ms_qry_safe", "QueryEntities", 260, 18)
	nQrySafe.Params["Query"] = "Tag == \"Tile\" && Health == 0"

	nQryMine := nodegraph.CreateNodeFromTemplate("ms_qry_flg", "QueryEntities", 260, 148)
	nQryMine.Params["Query"] = "Tag == \"Tile\" && Health == 50"

	nScore := nodegraph.CreateNodeFromTemplate("ms_add_scr", "ModifyProperty", 508, 18)
	nScore.Params["TargetTag"] = "Player"
	nScore.Params["Property"] = "Score"
	nScore.Params["Mode"] = "Add"
	nScore.Params["Value"] = "10"

	nEmit := nodegraph.CreateNodeFromTemplate("ms_emit_scan", "EmitSignal", 508, 148)
	nEmit.Params["Signal"] = "SectorScanned"
	nEmit.Params["Arg"] = "OK"

	g.Nodes = []*nodegraph.Node{nSigRev, nSigFlg, nQrySafe, nQryMine, nScore, nEmit}
	g.AddLink("ms_sig_rev", "Trigger", "ms_qry_safe", "In", nodegraph.SocketFlow)
	g.AddLink("ms_sig_flg", "Trigger", "ms_qry_safe", "In", nodegraph.SocketFlow)
	g.AddLink("ms_sig_flg", "Trigger", "ms_qry_flg", "In", nodegraph.SocketFlow)
	g.AddLink("ms_qry_safe", "Found", "ms_add_scr", "In", nodegraph.SocketFlow)
	g.AddLink("ms_qry_safe", "Entities", "ms_add_scr", "Targets", nodegraph.SocketEntityList)
	g.AddLink("ms_qry_safe", "Found", "ms_emit_scan", "In", nodegraph.SocketFlow)
	g.AddLink("ms_qry_flg", "Found", "ms_emit_scan", "In", nodegraph.SocketFlow)
	g.AddLink("ms_qry_flg", "Nearest", "ms_emit_scan", "Target", nodegraph.SocketEntity)

	sc.ScriptFiles = []*ScriptFile{
		{Path: "scripts/MinesweeperBoard.osmu", Source: MinesweeperBoardScript},
		{Path: "scripts/MineScannerHUD.osmu", Source: MineScannerHUDScript},
	}
	sc.CompileScripts()

	rootDir := filepath.Join("projects", "04_minesweeper_2d")
	sc.FilePath = filepath.Join(rootDir, "scenes", "main.osmu.json")
	return &Project{
		RootDir:      rootDir,
		ManifestPath: filepath.Join(rootDir, "project.osmu.json"),
		Manifest: ProjectManifest{
			Name:         "Minesweeper 2D",
			Version:      "1.0.0",
			Description:  sc.Description,
			Genre:        "2D Puzzle / Strategy",
			DefaultMode:  "2D",
			DefaultScene: "scenes/main.osmu.json",
			Scenes:       []string{"scenes/main.osmu.json"},
			Scripts:      []string{"scripts/MinesweeperBoard.osmu", "scripts/MineScannerHUD.osmu"},
		},
		LoadedScenes: []*Scene{sc},
		ActiveScene:  sc,
	}
}

func buildFlappyBirdProject() *Project {
	sc := NewScene("Flappy Bird 2D - Sky Run", Mode2D)
	sc.Description = "Playable 2D/3D Flappy Bird built in .osmu with Gravity/Flap Physics, Procedural Scrolling Pipes, Spatial Queries & Auto-Pilot AI"

	// Parallax Sky Clouds
	cloudPositions := [][3]float64{
		{-7.5, 4.2, -0.5},
		{-1.5, 4.8, -0.5},
		{4.5, 3.9, -0.5},
		{10.5, 4.5, -0.5},
	}
	for i, cp := range cloudPositions {
		id := fmt.Sprintf("cloud_%d", i+1)
		name := fmt.Sprintf("SkyCloud_0%d", i+1)
		cl := engine.NewEntity(id, name, "Cloud", engine.MeshCircle2D, engine.V3(cp[0], cp[1], cp[2]), engine.RGB(0.22, 0.30, 0.44))
		cl.Transform.Scale = engine.V3(2.2, 1.1, 0.4)
		cl.Physics.Enabled = false
		cl.Physics.IsStatic = true
		sc.AddEntity(cl)
	}

	// 4 Scrolling Pipe Pairs (Top + Bottom)
	initialGaps := []float64{0.6, -1.2, 1.4, -0.5}
	for i := 0; i < 4; i++ {
		px := 3.5 + float64(i)*5.5
		gapY := initialGaps[i]
		halfGap := 1.8
		pipeH := 8.0
		halfH := pipeH * 0.5

		topID := fmt.Sprintf("pipe_top_%d", i+1)
		topName := fmt.Sprintf("PipeTop_0%d", i+1)
		pt := engine.NewEntity(topID, topName, "PipeTop", engine.MeshSprite2D, engine.V3(px, gapY+halfGap+halfH, 0.0), engine.RGB(0.22, 0.78, 0.35))
		pt.Transform.Scale = engine.V3(1.6, pipeH, 1.2)
		pt.Physics.Enabled = false
		pt.Physics.IsStatic = true
		pt.Health = 100.0
		pt.MaxHealth = 0.0
		pt.Properties["Damage"] = gapY
		sc.AddEntity(pt)

		botID := fmt.Sprintf("pipe_bot_%d", i+1)
		botName := fmt.Sprintf("PipeBottom_0%d", i+1)
		pb := engine.NewEntity(botID, botName, "PipeBottom", engine.MeshSprite2D, engine.V3(px, gapY-halfGap-halfH, 0.0), engine.RGB(0.22, 0.78, 0.35))
		pb.Transform.Scale = engine.V3(1.6, pipeH, 1.2)
		pb.Physics.Enabled = false
		pb.Physics.IsStatic = true
		pb.Health = 100.0
		pb.MaxHealth = 0.0
		pb.Properties["Damage"] = gapY
		sc.AddEntity(pb)
	}

	// Scrolling Ground Segments (drawn on top of bottom pipe bases)
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("ground_seg_%d", i+1)
		name := fmt.Sprintf("GroundStrip_0%d", i+1)
		gx := -10.0 + float64(i)*4.0
		gr := engine.NewEntity(id, name, "GroundTile", engine.MeshSprite2D, engine.V3(gx, -7.2, 0.1), engine.RGB(0.32, 0.62, 0.26))
		gr.Transform.Scale = engine.V3(4.8, 2.4, 0.8)
		gr.Physics.Enabled = false
		gr.Physics.IsStatic = true
		sc.AddEntity(gr)
	}

	// Scoreboard HUD Entity
	hud := engine.NewEntity("flappy_hud", "FlappyScoreHUD", "HUD", engine.MeshSprite2D, engine.V3(-7.2, 4.8, 0.3), engine.RGB(0.20, 0.68, 0.98))
	hud.Transform.Scale = engine.V3(2.8, 1.3, 0.5)
	hud.Physics.Enabled = false
	hud.Physics.IsStatic = true
	hud.Render.Label = "SCORE: 0"
	hud.Properties["Score"] = 0.0
	hud.Scripts = []engine.ScriptAttachment{
		{ScriptPath: "scripts/FlappyScoreHUD.osmu", ComponentName: "FlappyScoreHUD"},
	}
	sc.AddEntity(hud)

	// Playable Flappy Bird Entity
	bird := engine.NewEntity("flappy_bird", "FlappyBird", "Player", engine.MeshCircle2D, engine.V3(-3.5, 0.5, 0.2), engine.RGB(1.0, 0.85, 0.15))
	bird.Transform.Scale = engine.V3(1.15, 1.15, 1.15)
	bird.Physics.Enabled = false
	bird.Physics.IsStatic = false
	bird.Render.Label = "0"
	bird.Properties["Score"] = 0.0
	bird.Scripts = []engine.ScriptAttachment{
		{
			ScriptPath:    "scripts/FlappyBirdController.osmu",
			ComponentName: "FlappyBirdController",
			Properties: map[string]interface{}{
				"Gravity":      22.0,
				"FlapVelocity": 8.2,
				"PipeSpeed":    4.5,
				"PipeGap":      3.6,
				"PipeWidth":    1.6,
				"PipeHeight":   8.0,
				"BirdRadius":   0.55,
				"FloorY":       -5.8,
				"CeilingY":     5.8,
			},
		},
	}
	sc.AddEntity(bird)

	// Visual NodeGraph for Flappy Bird Signals & Queries
	g := sc.NodeGraph
	nSigScore := nodegraph.CreateNodeFromTemplate("fb_sig_score", "OnSignal", 16, 18)
	nSigScore.Params["Signal"] = "ScoreIncreased"
	nSigFlap := nodegraph.CreateNodeFromTemplate("fb_sig_flap", "OnSignal", 16, 148)
	nSigFlap.Params["Signal"] = "BirdFlapped"

	nQryPipes := nodegraph.CreateNodeFromTemplate("fb_qry_pipes", "QueryEntities", 260, 18)
	nQryPipes.Params["Query"] = "Tag == \"PipeTop\" orderby Distance asc"

	nQryHud := nodegraph.CreateNodeFromTemplate("fb_qry_hud", "QueryEntities", 260, 148)
	nQryHud.Params["Query"] = "Tag == \"HUD\""

	nAddScore := nodegraph.CreateNodeFromTemplate("fb_mod_score", "ModifyProperty", 508, 18)
	nAddScore.Params["TargetTag"] = "HUD"
	nAddScore.Params["Property"] = "Score"
	nAddScore.Params["Mode"] = "Add"
	nAddScore.Params["Value"] = "1"

	nEmitPass := nodegraph.CreateNodeFromTemplate("fb_emit_pass", "EmitSignal", 508, 148)
	nEmitPass.Params["Signal"] = "GateCleared"
	nEmitPass.Params["Arg"] = " +1"

	g.Nodes = []*nodegraph.Node{nSigScore, nSigFlap, nQryPipes, nQryHud, nAddScore, nEmitPass}
	g.AddLink("fb_sig_score", "Trigger", "fb_qry_pipes", "In", nodegraph.SocketFlow)
	g.AddLink("fb_sig_score", "Trigger", "fb_qry_hud", "In", nodegraph.SocketFlow)
	g.AddLink("fb_sig_flap", "Trigger", "fb_qry_pipes", "In", nodegraph.SocketFlow)
	g.AddLink("fb_qry_hud", "Found", "fb_mod_score", "In", nodegraph.SocketFlow)
	g.AddLink("fb_qry_hud", "Entities", "fb_mod_score", "Targets", nodegraph.SocketEntityList)
	g.AddLink("fb_qry_pipes", "Found", "fb_emit_pass", "In", nodegraph.SocketFlow)
	g.AddLink("fb_qry_pipes", "Nearest", "fb_emit_pass", "Target", nodegraph.SocketEntity)

	sc.ScriptFiles = []*ScriptFile{
		{Path: "scripts/FlappyBirdController.osmu", Source: FlappyBirdControllerScript},
		{Path: "scripts/FlappyScoreHUD.osmu", Source: FlappyScoreHUDScript},
	}
	sc.CompileScripts()

	rootDir := filepath.Join("projects", "05_flappy_bird_2d")
	sc.FilePath = filepath.Join(rootDir, "scenes", "main.osmu.json")
	return &Project{
		RootDir:      rootDir,
		ManifestPath: filepath.Join(rootDir, "project.osmu.json"),
		Manifest: ProjectManifest{
			Name:         "Flappy Bird 2D",
			Version:      "1.0.0",
			Description:  sc.Description,
			Genre:        "2D Arcade / Endless Flyer",
			DefaultMode:  "2D",
			DefaultScene: "scenes/main.osmu.json",
			Scenes:       []string{"scenes/main.osmu.json"},
			Scripts:      []string{"scripts/FlappyBirdController.osmu", "scripts/FlappyScoreHUD.osmu"},
		},
		LoadedScenes: []*Scene{sc},
		ActiveScene:  sc,
	}
}

// MaterializeExamples writes the 5 standalone example projects to `projects/` (and cleans up legacy `examples/`).
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
