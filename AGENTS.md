# Agent Guidelines: OSMU Engine

OSMU (Overengineered System Manipulation Userinterface) is a deterministic 2D/3D game engine, graphical editor, and custom language runtime built in pure Go (Go 1.24+, zero external dependencies).

## Architectural Map
* `cmd/osmu`: CLI binary supporting `editor`, `open`, `play`, `export`, `projects`, `check`, `lsp`, `run`, `screenshot`.
* `pkg/engine`: Core ECS, spatial math (`Vec2`, `Vec3`, `Mat4`), reactive `SignalBus`, and declarative LINQ-style query engine (`pkg/engine/query.go`).
* `pkg/osmulang`: C#-like strongly-typed language (.osmu). Includes position-accurate lexer, AST parser, static type checker (`checker.go`), and bytecode VM (`vm.go`).
* `pkg/nodegraph`: Visual node graph logic with automatic highway trunk wire bundling (`routing.go`) to prevent spaghetti graphs.
* `pkg/editor`: 2D/3D graphical editor, native input/gizmos, embedded Web Studio (`/api/state`, `/api/action`), and software rasterizer (`screenshot.go`).
* `pkg/platform`: Zero-Cgo cross-platform windowing (Linux X11, Windows Win32, macOS) and embedded 5x7 bitmap font.
* `pkg/lsp`: Stdio JSON-RPC Language Server Protocol server.
* `pkg/runtime`: Project manifests (`project.osmu.json`), scene loader/serializer, and project runner.
* `projects/`: Five reference projects: `01_cyber_survivors_2d`, `02_fortress_siege_3d`, `03_dungeon_quest_3d`, `04_minesweeper_2d`, `05_flappy_bird_2d`.

## Essential Commands
* Run all unit tests: `go test ./...`
* Validate and compile `.osmu` scripts: `go run cmd/osmu/main.go check projects/`
* Headless simulation test: `go run cmd/osmu/main.go run projects/01_cyber_survivors_2d --steps 50`
* Headless PNG capture: `go run cmd/osmu/main.go screenshot <out.png> [projIdx] [steps]`
* Build binary: `go build -o osmu cmd/osmu/main.go`
* Export standalone game binary / `.exe`: `./osmu export <path/to/project> [out] [--os linux|windows]`

## Engineering Invariants for Agents
* **Zero External Dependencies**: Standard library only. Do not introduce Cgo, heavy graphical SDKs, or external Go modules unless explicitly approved.
* **Preserve Static Type Contracts**: Any change to `pkg/osmulang` must maintain 100% backwards compatibility with existing `.osmu` scripts in `projects/`.
* **Zero Emojis**: Do not add emojis to code, UI text, CLI logs, or documentation.
* **Verification Protocol**: After modifying any engine, editor, or script files, always verify with `go test ./...` and `go run cmd/osmu/main.go check projects/`.
