# OSMU Engine: Build and Terminal Usage Instructions

OSMU (Overengineered System Manipulation Userinterface) is a deterministic 2D/3D game engine, graphical editor, and custom language runtime built in pure Go with zero external dependencies.

---

## 1. Prerequisites

* **Go 1.24 or newer** installed (`go version`).
* **Zero External Dependencies**: No Cgo, C/C++ compilers, or external graphics SDKs are required.

---

## 2. Building from Terminal

### Build for Your Current OS (Linux / macOS)

```bash
go build -o osmu cmd/osmu/main.go
```

### Build for Windows (`.exe`)

```bash
GOOS=windows GOARCH=amd64 go build -o osmu.exe cmd/osmu/main.go
```

### Run Without Building (Direct Execution)

You can also run any command directly using `go run`:

```bash
go run cmd/osmu/main.go <subcommand> [arguments]
```

---

## 3. Terminal Commands Reference

Once compiled, invoke `./osmu` (or `osmu.exe` on Windows) with any of the following subcommands:

### 3.1 Launch the 2D/3D Graphical Editor (`editor` / `open`)

Launches the native desktop graphical studio and automatically starts the embedded Web Studio server in the background on `http://127.0.0.1:8420`.

```bash
# Launch editor with default startup project
./osmu
./osmu editor

# Open a specific project directory or manifest in the editor
./osmu open projects/01_cyber_survivors_2d
./osmu open projects/02_fortress_siege_3d/project.osmu.json
./osmu projects/03_dungeon_quest_3d
```

**Editor Flags:**
* `--web`: Run in headless Web Studio mode without opening a native OS window.
* `--web-port <port>`: Specify the HTTP port for the Web Studio server (default: `8420`).
* `--frames <N>`: Run for `N` frames and exit automatically (useful for automated testing).

```bash
# Start Web Studio only on port 9000
./osmu editor --web --web-port 9000
```

---

### 3.2 Launch Standalone Game Player (`play` / `game`)

Launches a project directly in full-window standalone player mode without the surrounding editor panels, and immediately starts the scene simulation.

```bash
# Play by project path
./osmu play projects/04_minesweeper_2d

# Play by project name or substring match
./osmu play "Cyber Survivors"
./osmu play minesweeper

# Play by project index (from 'osmu projects')
./osmu play 0

# Play in Web Browser mode only
./osmu play projects/01_cyber_survivors_2d --web --web-port 8420
```

*Note: If you run `./osmu play` inside a directory containing a `project.osmu.json` file, it will automatically play that project.*

---

### 3.3 List and Initialize Projects (`projects` / `init-projects`)

```bash
# List all discovered projects in the projects/ directory
./osmu projects

# Materialize/restore default reference projects on disk and list them
./osmu init-projects
```

**Included Reference Projects:**
* `[0]` `projects/01_cyber_survivors_2d` (2D Action Survival)
* `[1]` `projects/02_fortress_siege_3d` (3D Tactical Defense)
* `[2]` `projects/03_dungeon_quest_3d` (3D Dungeon Adventure)
* `[3]` `projects/04_minesweeper_2d` (2D Logic Puzzle)

---

### 3.4 Compile and Type-Check `.osmu` Scripts (`check` / `compile` / `osmuc`)

Statically parses, compiles, and verifies type contracts, component fields, and signals across `.osmu` script files or directories.

```bash
# Check all scripts inside the projects/ directory (default)
./osmu check
./osmu check projects/

# Check a specific project directory
./osmu check projects/01_cyber_survivors_2d

# Check a single .osmu script file
./osmu check projects/01_cyber_survivors_2d/scripts/cyber_survivors.osmu
./osmu projects/01_cyber_survivors_2d/scripts/cyber_survivors.osmu
```

---

### 3.5 Run Headless Simulation (`run`)

Executes a project's active scene simulation deterministically in the terminal without opening any GUI window, then prints emitted signals and `.osmu` script logs.

```bash
# Run default project (index 0) for 40 steps
./osmu run

# Run a specific project path for N simulation steps (dt = 0.05s per step)
./osmu run projects/01_cyber_survivors_2d --steps 50

# Run by project index
./osmu run 2 --steps 100
```

---

### 3.6 Render Headless PNG Screenshots (`screenshot`)

Uses the built-in pure-Go software rasterizer to render a 1280x760 frame of the editor or standalone game directly to a `.png` file.

```bash
# Syntax: ./osmu screenshot <output.png> [projectIndex] [simulationSteps] [flags]

# Render default project (index 0) after 12 simulation steps
./osmu screenshot preview.png

# Render project index 1 (Fortress Siege 3D) after 30 simulation steps
./osmu screenshot fortress.png 1 30

# Render with the Project Hub modal open
./osmu screenshot hub.png 0 12 --modal

# Render in standalone game player mode (no editor UI)
./osmu screenshot game_view.png 3 5 --play
```

---

### 3.7 Start the Language Server (`lsp`)

Starts the `.osmu` Language Server Protocol (LSP) server over `stdio` (JSON-RPC 2.0) for integration with code editors like VS Code, Neovim, or Helix.

```bash
./osmu lsp
```

---

## 4. Testing and Verification

To run the full automated unit test suite and verify all project scripts from the terminal:

```bash
# Run all Go unit tests across all packages
go test -v ./...

# Verify all .osmu scripts compile with 0 errors
./osmu check projects/
```
