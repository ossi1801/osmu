package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	"osmu/pkg/editor"
	"osmu/pkg/engine"
	"osmu/pkg/lsp"
	"osmu/pkg/osmulang"
	"osmu/pkg/platform"
	"osmu/pkg/runtime"
)

var embeddedProject string

func main() {
	args := os.Args[1:]
	if embeddedProject != "" && (len(args) == 0 || strings.HasPrefix(args[0], "-")) {
		runPlayGame(append([]string{embeddedProject}, args...))
		return
	}
	cmd := "editor"
	var subArgs []string
	if len(args) > 0 {
		if strings.HasPrefix(args[0], "-") {
			cmd = "editor"
			subArgs = args
		} else {
			cmd = args[0]
			subArgs = args[1:]
		}
	}

	switch cmd {
	case "editor", "gui", "open":
		runEditor(subArgs)
	case "play", "game":
		runPlayGame(subArgs)
	case "export", "build-exe":
		runExportGame(subArgs)
	case "projects", "list":
		runListProjects()
	case "init-projects":
		_ = runtime.MaterializeExamples(".")
		runListProjects()
	case "check", "compile", "osmuc":
		runCheck(subArgs)
	case "lsp":
		runLSP()
	case "run":
		runSimulation(subArgs)
	case "screenshot":
		runScreenshot(subArgs)
	case "help", "--help", "-h":
		printUsage()
	default:
		if strings.HasSuffix(cmd, ".osmu") {
			runCheck(args)
			return
		}
		// If user passed a project folder or project.osmu.json directly, open it in the editor!
		if _, err := os.Stat(cmd); err == nil {
			runEditor(args)
			return
		}
		printUsage()
	}
}

func printUsage() {
	fmt.Println(`OSMU — Overengineered System Manipulation Userinterface (2D/3D Game Engine & Editor)

Usage:
  osmu [editor] [path/to/project]              Launch the 2D/3D Graphical Editor (optionally loading a project)
  osmu play <path/to/project>                  Launch the game in standalone player mode (full window, no editor UI)
  osmu export <path/to/project> [out] [--os X] Export a standalone executable binary (.exe / Linux binary) for a game
  osmu open <path/to/project>                  Open a specific project folder or project.osmu.json in the Editor
  osmu projects                                List all discovered OSMU projects on disk
  osmu check [dir | file.osmu]                 Compile & statically type-check .osmu scripts
  osmu lsp                                     Start the .osmu Language Server Protocol (LSP) over stdio
  osmu run [path/to/project] [--steps N]        Run a project simulation
  osmu screenshot <out.png> [projIdx]          Render a frame of the OSMU Editor to a PNG file`)
}

func runListProjects() {
	projs := runtime.DiscoverProjects("projects")
	fmt.Printf("Discovered %d OSMU Project(s):\n", len(projs))
	for i, p := range projs {
		fmt.Printf("  [%d] %s (%s) — %s\n      Manifest: %s | Scenes: %v | Scripts: %v\n",
			i, p.Manifest.Name, p.Manifest.DefaultMode, p.Manifest.Genre,
			p.ManifestPath, p.Manifest.Scenes, p.Manifest.Scripts)
	}
}

func runCheck(args []string) {
	target := "projects"
	if len(args) > 0 {
		target = args[0]
	}
	files, diags, err := runtime.CheckDirectoryOrFile(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", target, err)
		os.Exit(1)
	}

	hasErr := false
	for _, sf := range files {
		if len(sf.Diagnostics) == 0 {
			comps := 0
			if sf.CheckResult != nil && sf.CheckResult.AST != nil {
				comps = len(sf.CheckResult.AST.Components)
			}
			fmt.Printf("[OK] %s — OK (%d component(s) verified)\n", sf.Path, comps)
			continue
		}
		lines := strings.Split(sf.Source, "\n")
		for _, d := range sf.Diagnostics {
			if d.Severity == "error" {
				hasErr = true
			}
			fmt.Println(formatDiagnostic(d, lines))
		}
	}

	if hasErr {
		fmt.Fprintf(os.Stderr, "\nCompilation failed with %d diagnostic(s).\n", len(diags))
		os.Exit(1)
	}
	fmt.Printf("\nAll %d .osmu file(s) compiled and type-checked with 0 errors.\n", len(files))
}

func formatDiagnostic(d osmulang.Diagnostic, lines []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s: %s [%s]: %s\n", d.Pos.String(), strings.ToUpper(d.Severity), d.Code, d.Message))
	if d.Pos.Line >= 1 && d.Pos.Line <= len(lines) {
		srcLine := strings.ReplaceAll(lines[d.Pos.Line-1], "\t", " ")
		sb.WriteString(fmt.Sprintf("  %4d | %s\n", d.Pos.Line, srcLine))
		col := d.Pos.Column - 1
		if col < 0 {
			col = 0
		}
		span := d.EndCol - d.Pos.Column
		if span < 1 {
			span = 1
		}
		sb.WriteString(fmt.Sprintf("       | %s%s", strings.Repeat(" ", col), strings.Repeat("^", span)))
	}
	return sb.String()
}

func runLSP() {
	srv := lsp.NewServer(os.Stdout)
	if err := srv.Serve(os.Stdin); err != nil {
		fmt.Fprintf(os.Stderr, "osmu lsp error: %v\n", err)
		os.Exit(1)
	}
}

func runSimulation(args []string) {
	projs := runtime.DiscoverProjects("projects")
	if len(projs) == 0 {
		projs = runtime.DiscoverProjects(".")
	}
	var proj *runtime.Project
	if len(projs) > 0 {
		proj = projs[0]
	}
	steps := 40
	for i := 0; i < len(args); i++ {
		if args[i] == "--steps" && i+1 < len(args) {
			steps, _ = strconv.Atoi(args[i+1])
			i++
		} else if idx, err := strconv.Atoi(args[i]); err == nil && idx >= 0 && idx < len(projs) {
			proj = projs[idx]
		} else if loaded, err := runtime.LoadProject(args[i]); err == nil {
			proj = loaded
		}
	}

	if proj == nil || proj.ActiveScene == nil {
		fmt.Fprintln(os.Stderr, "Error: No project found to run simulation. Please specify a project path: osmu run <path/to/project>")
		os.Exit(1)
	}

	sc := proj.ActiveScene
	fmt.Printf("Running Project %q [%s] — Scene %q (%s mode) for %d steps...\n",
		proj.Manifest.Name, proj.RootDir, sc.Name, sc.Mode, steps)
	sc.StartPlay()
	for i := 0; i < steps; i++ {
		sc.Step(0.05)
	}
	fmt.Println("--- Recent Signals Emitted ---")
	for _, ev := range sc.EventBus().RecentEvents(10) {
		fmt.Println(" ", ev.Summary())
	}
	fmt.Println("--- Recent .osmu Script Logs ---")
	for _, l := range sc.RecentLogs(10) {
		fmt.Println(" ", l)
	}
}

func runScreenshot(args []string) {
	outPath := "osmu_editor.png"
	projIdx := 0
	steps := 12
	showModal := false
	standalonePlay := false
	for i, a := range args {
		if a == "--modal" {
			showModal = true
			continue
		}
		if a == "--play" {
			standalonePlay = true
			continue
		}
		switch i {
		case 0:
			outPath = a
		case 1:
			projIdx, _ = strconv.Atoi(a)
		case 2:
			steps, _ = strconv.Atoi(a)
		}
	}

	app := editor.NewEditorApp(nil)
	if projIdx >= 0 && projIdx < len(app.Projects) {
		app.ActivateProject(projIdx)
	}
	if standalonePlay {
		app.StandalonePlay = true
		if app.Scene != nil && strings.Contains(strings.ToLower(app.Scene.Name), "minesweeper") {
			app.Cam2DZoom = 34.0
			app.Cam2DOffset = engine.V2(25.0, 0.0)
		} else if app.Scene != nil && strings.Contains(strings.ToLower(app.Scene.Name), "flappy") {
			app.Cam2DZoom = 34.0
			app.Cam2DOffset = engine.V2(0.0, 0.0)
		}
	}
	if steps > 0 && app.Scene != nil {
		app.Scene.StartPlay()
		for i := 0; i < steps; i++ {
			app.Scene.Step(0.05)
		}
	}
	app.ShowProjectModal = showModal
	if err := editor.SaveScreenshotPNG(app, 1280, 760, outPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save screenshot: %v\n", err)
		os.Exit(1)
	}
	projName := "None"
	if app.CurrentProject != nil {
		projName = app.CurrentProject.Manifest.Name
	}
	fmt.Printf("Saved OSMU screenshot to %s (Project %d: %s)\n", outPath, projIdx, projName)
}

func runEditor(args []string) {
	app := editor.NewEditorApp(nil)
	webPort := "8420"
	webOnly := false
	maxFrames := 0

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--web":
			webOnly = true
		case "--web-port":
			if i+1 < len(args) {
				webPort = args[i+1]
				i++
			}
		case "--frames":
			if i+1 < len(args) {
				maxFrames, _ = strconv.Atoi(args[i+1])
				i++
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				if err := app.LoadProjectFromPath(args[i]); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not load project %q: %v\n", args[i], err)
				}
			}
		}
	}

	ws := editor.NewWebServer(app)
	go func() {
		_ = http.ListenAndServe("127.0.0.1:"+webPort, ws.Handler())
	}()

	if !webOnly {
		title := "OSMU — Overengineered System Manipulation Userinterface"
		if app.CurrentProject != nil {
			title = fmt.Sprintf("OSMU Engine — %s [%s]", app.CurrentProject.Manifest.Name, app.CurrentProject.RootDir)
		}
		win, err := platform.NewWindow(title, 1280, 760)
		if err == nil {
			app.Win = win
			defer win.Close()
			in := platform.NewInputState(1280, 760)
			last := time.Now()
			frames := 0
			for !in.ShouldClose {
				now := time.Now()
				dt := now.Sub(last).Seconds()
				last = now

				in.ResetFrameDeltas()
				win.PollEvents(in)
				ops := app.UpdateAndBuildDrawList(in, dt)
				win.DrawOps(in.Width, in.Height, ops)
				win.SwapBuffers()

				frames++
				if maxFrames > 0 && frames >= maxFrames {
					break
				}
				if elapsed := time.Since(now); elapsed < 8*time.Millisecond {
					time.Sleep(8*time.Millisecond - elapsed)
				}
			}
			return
		}
		fmt.Printf("Note: Native desktop display not active (%v); running Web Studio server on http://127.0.0.1:%s\n", err, webPort)
	} else {
		fmt.Printf("OSMU Web Studio running at http://127.0.0.1:%s\n", webPort)
	}

	if maxFrames > 0 {
		return
	}
	select {}
}

func runPlayGame(args []string) {
	app := editor.NewEditorApp(nil)
	app.StandalonePlay = true

	target := ""
	webPort := "8420"
	webOnly := false
	maxFrames := 0

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--web":
			webOnly = true
		case "--web-port":
			if i+1 < len(args) {
				webPort = args[i+1]
				i++
			}
		case "--frames":
			if i+1 < len(args) {
				maxFrames, _ = strconv.Atoi(args[i+1])
				i++
			}
		default:
			if !strings.HasPrefix(args[i], "-") && target == "" {
				target = args[i]
			}
		}
	}

	if target == "" {
		if _, err := os.Stat("project.osmu.json"); err == nil {
			target = "."
		}
	}

	if target != "" {
		matched := false
		for i, p := range app.Projects {
			if strings.EqualFold(p.Manifest.Name, target) ||
				strings.Contains(strings.ToLower(p.Manifest.Name), strings.ToLower(target)) ||
				strings.Contains(strings.ToLower(p.RootDir), strings.ToLower(target)) {
				app.ActivateProject(i)
				matched = true
				break
			}
		}
		if !matched {
			if idx, err := strconv.Atoi(target); err == nil && idx >= 0 && idx < len(app.Projects) {
				app.ActivateProject(idx)
				matched = true
			}
		}
		if !matched {
			if err := app.LoadProjectFromPath(target); err == nil {
				matched = true
			}
		}
		if !matched {
			builtins := runtime.BuiltinProjects()
			for _, bp := range builtins {
				if strings.EqualFold(bp.Manifest.Name, target) ||
					strings.Contains(strings.ToLower(bp.Manifest.Name), strings.ToLower(target)) ||
					strings.Contains(strings.ToLower(bp.RootDir), strings.ToLower(target)) {
					app.Projects = append(app.Projects, bp)
					app.ActivateProject(len(app.Projects) - 1)
					matched = true
					break
				}
			}
		}
		if !matched {
			fmt.Fprintf(os.Stderr, "Error: could not load project %q\n", target)
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Error: No project specified to play.")
		fmt.Fprintln(os.Stderr, "Usage: osmu play <path/to/project>")
		fmt.Fprintln(os.Stderr, "Example: osmu play projects/05_flappy_bird_2d")
		os.Exit(1)
	}

	if app.Scene == nil {
		fmt.Fprintln(os.Stderr, "Error: Selected project has no valid scenes to play.")
		os.Exit(1)
	}

	app.Scene.StartPlay()
	if strings.Contains(strings.ToLower(app.Scene.Name), "minesweeper") {
		app.Cam2DZoom = 34.0
		app.Cam2DOffset = engine.V2(25.0, 0.0)
	} else if strings.Contains(strings.ToLower(app.Scene.Name), "flappy") {
		app.Cam2DZoom = 34.0
		app.Cam2DOffset = engine.V2(0.0, 0.0)
	}

	ws := editor.NewWebServer(app)
	go func() {
		_ = http.ListenAndServe("127.0.0.1:"+webPort, ws.Handler())
	}()

	if !webOnly {
		title := "OSMU Game Player"
		if app.CurrentProject != nil {
			title = fmt.Sprintf("%s — OSMU Game", app.CurrentProject.Manifest.Name)
		}
		win, err := platform.NewWindow(title, 1280, 760)
		if err == nil {
			app.Win = win
			defer win.Close()
			in := platform.NewInputState(1280, 760)
			last := time.Now()
			frames := 0
			for !in.ShouldClose {
				now := time.Now()
				dt := now.Sub(last).Seconds()
				last = now

				in.ResetFrameDeltas()
				win.PollEvents(in)
				ops := app.UpdateAndBuildDrawList(in, dt)
				win.DrawOps(in.Width, in.Height, ops)
				win.SwapBuffers()

				frames++
				if maxFrames > 0 && frames >= maxFrames {
					break
				}
				if elapsed := time.Since(now); elapsed < 8*time.Millisecond {
					time.Sleep(8*time.Millisecond - elapsed)
				}
			}
			return
		}
		fmt.Printf("Note: Native desktop display not active (%v); running Web Studio on http://127.0.0.1:%s\n", err, webPort)
	} else {
		fmt.Printf("OSMU Game running at http://127.0.0.1:%s\n", webPort)
	}

	if maxFrames > 0 {
		return
	}
	select {}
}

func runExportGame(args []string) {
	target := ""
	outPath := ""
	targetOS := goruntime.GOOS
	targetArch := goruntime.GOARCH

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--os", "-os":
			if i+1 < len(args) {
				targetOS = strings.ToLower(args[i+1])
				i++
			}
		case "--arch", "-arch":
			if i+1 < len(args) {
				targetArch = strings.ToLower(args[i+1])
				i++
			}
		case "-o", "--out":
			if i+1 < len(args) {
				outPath = args[i+1]
				i++
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				if target == "" {
					target = args[i]
				} else if outPath == "" {
					outPath = args[i]
				}
			}
		}
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "Usage: osmu export <path/to/project> [output_binary] [--os linux|windows]")
		fmt.Fprintln(os.Stderr, "Example (Linux binary): ./osmu export projects/05_flappy_bird_2d flappy_bird")
		fmt.Fprintln(os.Stderr, "Example (Windows .exe): ./osmu export projects/05_flappy_bird_2d flappy_bird.exe --os windows")
		os.Exit(1)
	}

	if strings.HasSuffix(strings.ToLower(outPath), ".exe") && targetOS == goruntime.GOOS && goruntime.GOOS != "windows" {
		targetOS = "windows"
	}

	if outPath == "" {
		base := filepath.Base(strings.TrimRight(target, "/\\"))
		if base == "." || base == "" {
			base = "osmu_game"
		}
		outPath = base
		if targetOS == "windows" && !strings.HasSuffix(strings.ToLower(outPath), ".exe") {
			outPath += ".exe"
		}
	}

	ldflags := fmt.Sprintf("-s -w -X 'main.embeddedProject=%s'", target)
	cmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", outPath, "./cmd/osmu")
	env := os.Environ()
	env = append(env, "GOOS="+targetOS, "GOARCH="+targetArch)
	if targetOS == "windows" && goruntime.GOOS != "windows" {
		env = append(env, "CGO_ENABLED=0")
	}
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Exporting standalone OSMU game %q for %s/%s -> %s ...\n", target, targetOS, targetArch, outPath)
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Export failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[OK] Standalone executable created: %s\n", outPath)
}
