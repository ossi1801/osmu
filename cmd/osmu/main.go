package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"osmu/pkg/editor"
	"osmu/pkg/lsp"
	"osmu/pkg/osmulang"
	"osmu/pkg/platform"
	"osmu/pkg/runtime"
)

func main() {
	if _, err := os.Stat("projects"); os.IsNotExist(err) {
		_ = runtime.MaterializeExamples(".")
	}

	args := os.Args[1:]
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
  osmu [editor] [path/to/project]       Launch the 2D/3D Graphical Editor (optionally loading a project)
  osmu open <path/to/project>           Open a specific project folder or project.osmu.json in the Editor
  osmu projects                         List all discovered OSMU projects on disk
  osmu check [dir | file.osmu]          Compile & statically type-check .osmu scripts
  osmu lsp                              Start the .osmu Language Server Protocol (LSP) over stdio
  osmu run [path/to/project] [--steps N] Run a project simulation
  osmu screenshot <out.png> [projIdx]   Render a frame of the OSMU Editor to a PNG file`)
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
			fmt.Printf("✓ %s — OK (%d component(s) verified)\n", sf.Path, comps)
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
		projs = runtime.BuiltinProjects()
	}
	proj := projs[0]
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
	for i, a := range args {
		if a == "--modal" {
			showModal = true
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
	app.ActivateProject(projIdx)
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
	fmt.Printf("Saved OSMU Editor screenshot to %s (Project %d: %s)\n", outPath, projIdx, app.CurrentProject.Manifest.Name)
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
				time.Sleep(16 * time.Millisecond)
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
