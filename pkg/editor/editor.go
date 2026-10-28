package editor

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"osmu/pkg/engine"
	"osmu/pkg/nodegraph"
	"osmu/pkg/platform"
	"osmu/pkg/runtime"
)

// Rect represents a 2D UI rectangle in screen pixels.
type Rect struct {
	X, Y, W, H float64
}

func (r Rect) Contains(px, py float64) bool {
	return px >= r.X && px <= r.X+r.W && py >= r.Y && py <= r.Y+r.H
}

// EditorApp manages the state of the OSMU Graphical Editor and Project Manager.
type EditorApp struct {
	Projects          []*runtime.Project
	CurrentProjectIdx int
	CurrentProject    *runtime.Project
	Scenes            []*runtime.Scene
	CurrentSceneIdx   int
	Scene             *runtime.Scene
	Win               platform.Window

	SelectedEntityID string
	SelectedNodeID   string

	// Project Browser / Loader Modal state
	ShowProjectModal  bool
	ProjectPathInput  string
	PathInputFocused  bool
	StatusBanner      string
	StatusBannerTime  time.Time

	// 3D Orbit Camera
	CamYaw    float64
	CamPitch  float64
	CamDist   float64
	CamTarget engine.Vec3

	// 2D Viewport Camera
	Cam2DOffset engine.Vec2
	Cam2DZoom   float64

	// Interactive dragging states
	DraggingOrbit     bool
	DraggingGizmoAxis int // 0=none, 1=X, 2=Y, 3=Z
	DraggingNodeID    string
	NodeDragOffset    engine.Vec2
	NodeCanvasPan     engine.Vec2

	// Interactive socket wiring state
	WiringFromNode   string
	WiringFromSocket string
	WiringKind       nodegraph.SocketKind

	// Live Query Bar state
	QueryText       string
	QueryFocused    bool
	QueryMatches    map[string]bool
	QueryMatchCount int
	QueryError      string

	// Render command list for current frame
	cmds []platform.DrawCmd
}

// NewEditorApp initializes the OSMU Graphical Editor with discovered or built-in projects.
func NewEditorApp(scenes []*runtime.Scene) *EditorApp {
	projs := runtime.DiscoverProjects("projects")
	if len(projs) == 0 {
		projs = runtime.BuiltinProjects()
	}
	app := &EditorApp{
		Projects:          projs,
		CurrentProjectIdx: 0,
		CamYaw:            36.0,
		CamPitch:          28.0,
		CamDist:           17.5,
		CamTarget:         engine.V3(0, 0.5, 0),
		Cam2DZoom:         25.0,
		QueryText:         "Tag == \"Enemy\" && Health > 0 orderby Distance asc",
		QueryMatches:      make(map[string]bool),
		ProjectPathInput:  "projects/02_fortress_siege_3d",
	}
	if len(projs) > 0 {
		app.ActivateProject(0)
	} else if len(scenes) > 0 {
		app.Scenes = scenes
		app.Scene = scenes[0]
		app.selectDefaultEntity()
		app.runLiveQuery()
	}
	return app
}

// ActivateProject switches the editor to one of the loaded projects by index.
func (app *EditorApp) ActivateProject(idx int) {
	if idx < 0 || idx >= len(app.Projects) {
		return
	}
	if app.Scene != nil && app.Scene.Playing {
		app.Scene.StopPlay()
	}
	app.CurrentProjectIdx = idx
	proj := app.Projects[idx]
	app.CurrentProject = proj
	app.Scenes = proj.LoadedScenes
	app.CurrentSceneIdx = 0
	app.Scene = proj.ActiveScene
	app.SelectedNodeID = ""
	app.ProjectPathInput = proj.RootDir
	app.selectDefaultEntity()
	app.runLiveQuery()
	app.setStatus(fmt.Sprintf("Loaded Project: %s (%s)", proj.Manifest.Name, proj.RootDir))
	if app.Win != nil {
		app.Win.SetTitle(fmt.Sprintf("OSMU Engine — %s [%s]", proj.Manifest.Name, proj.RootDir))
	}
}

// LoadProjectFromPath loads a project folder or `project.osmu.json` from disk and activates it.
func (app *EditorApp) LoadProjectFromPath(path string) error {
	path = strings.TrimSpace(path)
	proj, err := runtime.LoadProject(path)
	if err != nil {
		app.setStatus("Load Error: " + err.Error())
		return err
	}
	// Replace if already in list, else append
	for i, existing := range app.Projects {
		if filepath.Clean(existing.RootDir) == filepath.Clean(proj.RootDir) {
			app.Projects[i] = proj
			app.ActivateProject(i)
			return nil
		}
	}
	app.Projects = append(app.Projects, proj)
	app.ActivateProject(len(app.Projects) - 1)
	return nil
}

// CreateNewProject creates a new standalone project on disk under `projects/` and loads it.
func (app *EditorApp) CreateNewProject(mode runtime.ViewportMode) {
	idx := len(app.Projects) + 1
	folder := fmt.Sprintf("projects/0%d_custom_%s_game", idx, strings.ToLower(string(mode)))
	title := fmt.Sprintf("Custom %s Game #%d", mode, idx)
	sc := runtime.NewScene(title+" Main Scene", mode)
	sc.Description = "Custom user project created in OSMU Studio"

	if mode == runtime.Mode3D {
		floor := engine.NewEntity("floor_1", "ArenaFloor", "Environment", engine.MeshPlane, engine.V3(0, 0, 0), engine.RGB(0.15, 0.18, 0.24))
		floor.Transform.Scale = engine.V3(16, 1, 16)
		floor.Physics.IsStatic = true
		sc.AddEntity(floor)
	}
	hero := engine.NewEntity("player_1", "HeroUnit", "Player", engine.MeshPyramid, engine.V3(0, 0.8, 0), engine.RGB(0.20, 0.90, 1.0))
	hero.Scripts = []engine.ScriptAttachment{
		{ScriptPath: "scripts/HeroController.osmu", ComponentName: "HeroController"},
	}
	sc.AddEntity(hero)

	enemy := engine.NewEntity("enemy_1", "TargetDummy", "Enemy", engine.MeshCube, engine.V3(3.5, 0.8, 0), engine.RGB(0.95, 0.32, 0.38))
	sc.AddEntity(enemy)

	scriptSrc := `namespace CustomProject;

component HeroController : Entity {
    [Export] public float Speed = 5.5f;

    public void OnReady() {
        Log("HeroController initialized in " + this.Name);
    }

    public void OnUpdate(float dt) {
        float ix = InputAxisX();
        float iy = InputAxisY();
        this.Position = this.Position + Vec3(ix * this.Speed * dt, 0.0f, -iy * this.Speed * dt);
    }
}
`
	sc.ScriptFiles = []*runtime.ScriptFile{
		{Path: "scripts/HeroController.osmu", Source: scriptSrc},
	}
	sc.CompileScripts()

	proj := &runtime.Project{
		RootDir:      folder,
		ManifestPath: filepath.Join(folder, "project.osmu.json"),
		Manifest: runtime.ProjectManifest{
			Name:         title,
			Version:      "1.0.0",
			Description:  sc.Description,
			Genre:        "Custom Project",
			DefaultMode:  string(mode),
			DefaultScene: "scenes/main.osmu.json",
			Scenes:       []string{"scenes/main.osmu.json"},
			Scripts:      []string{"scripts/HeroController.osmu"},
		},
		LoadedScenes: []*runtime.Scene{sc},
		ActiveScene:  sc,
	}
	_ = proj.SaveProject()
	app.Projects = append(app.Projects, proj)
	app.ActivateProject(len(app.Projects) - 1)
}

func (app *EditorApp) setStatus(msg string) {
	app.StatusBanner = msg
	app.StatusBannerTime = time.Now()
}

func (app *EditorApp) SwitchScene(idx int) {
	if idx >= 0 && idx < len(app.Projects) {
		app.ActivateProject(idx)
	}
}

func (app *EditorApp) selectDefaultEntity() {
	if app.Scene == nil || len(app.Scene.EntityList) == 0 {
		app.SelectedEntityID = ""
		return
	}
	for _, e := range app.Scene.EntityList {
		if e != nil && e.Tag == "Player" {
			app.SelectedEntityID = e.ID
			return
		}
	}
	for _, e := range app.Scene.EntityList {
		if e != nil && e.Tag != "Environment" && e.Tag != "Wall" {
			app.SelectedEntityID = e.ID
			return
		}
	}
	app.SelectedEntityID = app.Scene.EntityList[0].ID
}

func (app *EditorApp) SelectedEntity() *engine.Entity {
	if app.Scene == nil {
		return nil
	}
	return app.Scene.FindEntityByID(app.SelectedEntityID)
}

func (app *EditorApp) runLiveQuery() {
	app.QueryMatches = make(map[string]bool)
	app.QueryError = ""
	if app.Scene == nil {
		return
	}
	origin := engine.V3(0, 0, 0)
	if sel := app.SelectedEntity(); sel != nil {
		origin = sel.Transform.Position
	}
	matched, err := engine.EvalQueryString(app.Scene.Entities(), app.QueryText, origin)
	if err != nil {
		app.QueryError = err.Error()
		app.QueryMatchCount = 0
		return
	}
	app.QueryMatchCount = len(matched)
	for _, e := range matched {
		app.QueryMatches[e.ID] = true
	}
}

// UpdateAndBuildDrawList processes input, steps the simulation if playing, and generates OpenGL DrawCmds.
func (app *EditorApp) UpdateAndBuildDrawList(in *platform.InputState, dt float64) []platform.DrawCmd {
	if app.Scene != nil {
		app.Scene.SetInputKeys(in.KeysDown)
		if app.Scene.Playing && !app.Scene.Paused {
			app.Scene.Step(dt)
			app.runLiveQuery()
		}
	}

	w := float64(in.Width)
	h := float64(in.Height)
	if w < 640 {
		w = 640
	}
	if h < 480 {
		h = 480
	}

	app.cmds = app.cmds[:0]
	app.emitClear(0.08, 0.09, 0.12, 1.0)

	topH := 42.0
	leftW := 255.0
	rightW := 315.0
	centerW := w - leftW - rightW
	if centerW < 240 {
		centerW = 240
	}
	viewH := math.Floor((h - topH) * 0.55)
	nodeH := (h - topH) - viewH

	topRect := Rect{0, 0, w, topH}
	leftRect := Rect{0, topH, leftW, h - topH}
	viewRect := Rect{leftW, topH, centerW, viewH}
	nodeRect := Rect{leftW, topH + viewH, centerW, nodeH}
	rightRect := Rect{leftW + centerW, topH, rightW, h - topH}

	// Handle text input for Project Path box or Live Query Bar
	if app.ShowProjectModal && app.PathInputFocused {
		if in.TypedText != "" {
			app.ProjectPathInput += in.TypedText
		}
		if in.KeysPressed["Backspace"] && len(app.ProjectPathInput) > 0 {
			app.ProjectPathInput = app.ProjectPathInput[:len(app.ProjectPathInput)-1]
		}
		if in.KeysPressed["Enter"] {
			if err := app.LoadProjectFromPath(app.ProjectPathInput); err == nil {
				app.ShowProjectModal = false
			}
		}
		if in.KeysPressed["Escape"] {
			app.ShowProjectModal = false
		}
	} else if app.QueryFocused {
		if in.TypedText != "" {
			app.QueryText += in.TypedText
			app.runLiveQuery()
		}
		if in.KeysPressed["Backspace"] && len(app.QueryText) > 0 {
			app.QueryText = app.QueryText[:len(app.QueryText)-1]
			app.runLiveQuery()
		}
		if in.KeysPressed["Enter"] || in.KeysPressed["Escape"] {
			app.QueryFocused = false
			app.runLiveQuery()
		}
	} else {
		if in.KeysPressed["Space"] && app.Scene != nil {
			if app.Scene.Playing {
				app.Scene.Paused = !app.Scene.Paused
			} else {
				app.Scene.StartPlay()
			}
		}
		if in.KeysPressed["G"] && app.Scene != nil && app.Scene.NodeGraph != nil {
			app.Scene.NodeGraph.AutoGroup = !app.Scene.NodeGraph.AutoGroup
		}
		if in.KeysPressed["Escape"] && app.ShowProjectModal {
			app.ShowProjectModal = false
		}
	}

	// Pass subtle nil-ed clicks to underlying panels if modal is open so clicks inside modal don't click behind it
	bgIn := in
	if app.ShowProjectModal {
		copied := *in
		copied.MouseClicked = [4]bool{}
		bgIn = &copied
	}

	app.drawTopToolbar(topRect, bgIn)
	app.drawLeftHierarchy(leftRect, bgIn)
	app.drawCenterViewport(viewRect, bgIn)
	app.drawNodeAndQueryPanel(nodeRect, bgIn)
	app.drawRightInspector(rightRect, bgIn)

	if app.ShowProjectModal {
		app.drawProjectBrowserModal(w, h, in)
	}

	return app.cmds
}

// --- Top Toolbar (with Project Loader, Scene Controls, and Compiler Status) ---

func (app *EditorApp) drawTopToolbar(r Rect, in *platform.InputState) {
	app.emitRect(r.X, r.Y, r.W, r.H, 0.11, 0.13, 0.17, 1.0)
	app.emitLine(r.X, r.Y+r.H, r.X+r.W, r.Y+r.H, 1.5, 0.22, 0.26, 0.34, 1.0)

	// Brand logo badge
	app.emitRect(8, 7, 64, 28, 0.16, 0.48, 0.85, 1.0)
	app.emitText(14, 15, "OSMU", 1.65, 1.0, 1.0, 1.0, 1.0)

	// Project Manager / Loader & Save buttons
	x := 78.0
	if app.drawButtonColor(Rect{x, 7, 114, 28}, "OPEN PROJECT", app.ShowProjectModal, 0.24, 0.38, 0.62, in) {
		app.ShowProjectModal = !app.ShowProjectModal
		app.PathInputFocused = true
	}
	x += 118
	if app.drawButton(Rect{x, 7, 56, 28}, "SAVE", false, in) && app.CurrentProject != nil {
		if err := app.CurrentProject.SaveProject(); err == nil {
			app.setStatus("Saved project to " + app.CurrentProject.RootDir)
		}
	}
	x += 62

	// Quick Project switcher tabs
	maxTabs := 4
	bw := 104.0
	nameTrunc := 11
	if len(app.Projects) >= 4 {
		bw = 82.0
		nameTrunc = 8
	}
	for i, proj := range app.Projects {
		if i >= maxTabs {
			break
		}
		active := app.CurrentProjectIdx == i
		short := fmt.Sprintf("P%d:%s", i+1, truncateStr(proj.Manifest.Name, nameTrunc))
		if app.drawButton(Rect{x, 7, bw, 28}, short, active, in) {
			app.ActivateProject(i)
		}
		x += bw + 4
	}

	// 3D / 2D Viewport Toggle
	x += 6
	is3D := app.Scene != nil && app.Scene.Mode == runtime.Mode3D
	if app.drawButton(Rect{x, 7, 42, 28}, "3D", is3D, in) && app.Scene != nil {
		app.Scene.Mode = runtime.Mode3D
	}
	if app.drawButton(Rect{x + 45, 7, 42, 28}, "2D", !is3D, in) && app.Scene != nil {
		app.Scene.Mode = runtime.Mode2D
	}

	// Play / Pause / Step / Stop Transport Controls
	x += 94
	playing := app.Scene != nil && app.Scene.Playing
	paused := app.Scene != nil && app.Scene.Paused
	if app.drawButtonColor(Rect{x, 7, 58, 28}, "PLAY", playing && !paused, 0.18, 0.62, 0.34, in) && app.Scene != nil {
		app.Scene.StartPlay()
	}
	if app.drawButton(Rect{x + 62, 7, 58, 28}, "PAUSE", paused, in) && app.Scene != nil && app.Scene.Playing {
		app.Scene.Paused = !app.Scene.Paused
	}
	if app.drawButton(Rect{x + 124, 7, 52, 28}, "STEP", false, in) && app.Scene != nil {
		if !app.Scene.Playing {
			app.Scene.StartPlay()
			app.Scene.Paused = true
		}
		app.Scene.Step(0.05)
		app.runLiveQuery()
	}
	if app.drawButtonColor(Rect{x + 180, 7, 54, 28}, "STOP", false, 0.68, 0.24, 0.28, in) && app.Scene != nil {
		app.Scene.StopPlay()
		app.runLiveQuery()
	}

	// Auto-Group Node Wires toggle
	x += 242
	autoGrp := app.Scene != nil && app.Scene.NodeGraph != nil && app.Scene.NodeGraph.AutoGroup
	grpLabel := "WIRES:GROUPED"
	if !autoGrp {
		grpLabel = "WIRES:RAW"
	}
	if app.drawButton(Rect{x, 7, 114, 28}, grpLabel, autoGrp, in) && app.Scene != nil && app.Scene.NodeGraph != nil {
		app.Scene.NodeGraph.AutoGroup = !app.Scene.NodeGraph.AutoGroup
	}

	// Compiler status badge
	diags := 0
	if app.Scene != nil {
		for _, sf := range app.Scene.ScriptFiles {
			diags += len(sf.Diagnostics)
		}
	}
	compRect := Rect{r.W - 110, 7, 102, 28}
	if compRect.X > x+118 {
		if diags == 0 {
			app.emitRect(compRect.X, compRect.Y, compRect.W, compRect.H, 0.13, 0.36, 0.24, 1.0)
			app.emitText(compRect.X+8, compRect.Y+9, ".osmu: OK", 1.3, 0.45, 0.98, 0.65, 1.0)
		} else {
			app.emitRect(compRect.X, compRect.Y, compRect.W, compRect.H, 0.48, 0.18, 0.20, 1.0)
			app.emitText(compRect.X+8, compRect.Y+9, fmt.Sprintf("Err: %d", diags), 1.3, 1.0, 0.65, 0.65, 1.0)
		}
	}
}

// --- Interactive Project Browser & Loader Modal ---

func (app *EditorApp) drawProjectBrowserModal(w, h float64, in *platform.InputState) {
	// Dim backdrop
	app.emitRect(0, 0, w, h, 0.02, 0.03, 0.05, 0.72)

	mw := 680.0
	mh := 430.0
	mx := (w - mw) * 0.5
	my := (h - mh) * 0.5
	app.emitRect(mx, my, mw, mh, 0.12, 0.15, 0.20, 0.98)
	app.emitRectOutline(mx, my, mw, mh, 2.0, 0.32, 0.58, 0.92, 1.0)

	// Modal Header
	app.emitRect(mx, my, mw, 36, 0.16, 0.22, 0.32, 1.0)
	app.emitText(mx+16, my+12, "OSMU PROJECT MANAGER - LOAD OR CREATE PROJECT", 1.4, 0.95, 0.98, 1.0, 1.0)
	if app.drawButtonColor(Rect{mx + mw - 76, my + 5, 66, 26}, "CLOSE", false, 0.65, 0.22, 0.26, in) {
		app.ShowProjectModal = false
		return
	}

	// Custom Project Path Input Box + Load From Disk Button
	py := my + 48
	app.emitText(mx+16, py+6, "Project Folder / Manifest Path:", 1.2, 0.78, 0.86, 0.96, 1.0)
	py += 22
	pathBox := Rect{mx + 16, py, mw - 230, 28}
	if in.MouseClicked[platform.MouseLeft] {
		app.PathInputFocused = pathBox.Contains(in.MouseX, in.MouseY)
	}
	app.emitRect(pathBox.X, pathBox.Y, pathBox.W, pathBox.H, 0.07, 0.09, 0.13, 1.0)
	borderC := engine.RGB(0.30, 0.40, 0.56)
	if app.PathInputFocused {
		borderC = engine.RGB(0.35, 0.85, 1.0)
	}
	app.emitRectOutline(pathBox.X, pathBox.Y, pathBox.W, pathBox.H, 1.5, borderC.R, borderC.G, borderC.B, 1.0)
	pathText := app.ProjectPathInput
	if app.PathInputFocused {
		pathText += "_"
	}
	app.emitText(pathBox.X+8, pathBox.Y+8, truncateStr(pathText, 48), 1.25, 0.95, 0.98, 1.0, 1.0)

	if app.drawButtonColor(Rect{pathBox.X + pathBox.W + 8, py, 102, 28}, "LOAD PATH", false, 0.20, 0.58, 0.36, in) {
		if err := app.LoadProjectFromPath(app.ProjectPathInput); err == nil {
			app.ShowProjectModal = false
			return
		}
	}
	if app.drawButton(Rect{pathBox.X + pathBox.W + 114, py, 82, 28}, "RESCAN", false, in) {
		discovered := runtime.DiscoverProjects(".")
		if len(discovered) > 0 {
			app.Projects = discovered
		}
	}

	// Discovered Projects List
	py += 40
	app.emitText(mx+16, py, "AVAILABLE PROJECTS ON DISK (Click to Open):", 1.25, 0.75, 0.85, 0.98, 1.0)
	py += 20

	for i, proj := range app.Projects {
		if py+64 > my+mh-52 {
			break
		}
		card := Rect{mx + 16, py, mw - 32, 58}
		isCur := i == app.CurrentProjectIdx
		bg := engine.RGB(0.15, 0.18, 0.25)
		if isCur {
			bg = engine.RGB(0.18, 0.30, 0.48)
		}
		app.emitRect(card.X, card.Y, card.W, card.H, bg.R, bg.G, bg.B, 1.0)
		app.emitRectOutline(card.X, card.Y, card.W, card.H, 1.2, 0.30, 0.42, 0.60, 1.0)

		title := fmt.Sprintf("%s  [%s | %s]", proj.Manifest.Name, proj.Manifest.DefaultMode, proj.Manifest.Genre)
		app.emitText(card.X+12, card.Y+8, title, 1.35, 1.0, 0.94, 0.68, 1.0)
		app.emitText(card.X+12, card.Y+26, "Path: "+proj.ManifestPath, 1.1, 0.65, 0.82, 0.96, 1.0)
		app.emitText(card.X+12, card.Y+41, truncateStr(proj.Manifest.Description, 72), 1.1, 0.78, 0.82, 0.88, 1.0)

		btnLbl := "OPEN"
		if isCur {
			btnLbl = "ACTIVE"
		}
		if app.drawButtonColor(Rect{card.X + card.W - 88, card.Y + 14, 76, 30}, btnLbl, isCur, 0.22, 0.52, 0.82, in) {
			app.ActivateProject(i)
			app.ShowProjectModal = false
			return
		}
		py += 64
	}

	// Bottom Footer: Create New Project Buttons
	fy := my + mh - 42
	app.emitLine(mx+12, fy-6, mx+mw-12, fy-6, 1.0, 0.25, 0.32, 0.44, 1.0)
	if app.drawButtonColor(Rect{mx + 16, fy, 175, 30}, "+ NEW 3D PROJECT", false, 0.22, 0.46, 0.72, in) {
		app.CreateNewProject(runtime.Mode3D)
		app.ShowProjectModal = false
	}
	if app.drawButtonColor(Rect{mx + 200, fy, 175, 30}, "+ NEW 2D PROJECT", false, 0.22, 0.46, 0.72, in) {
		app.CreateNewProject(runtime.Mode2D)
		app.ShowProjectModal = false
	}
	if app.StatusBanner != "" {
		app.emitText(mx+390, fy+9, truncateStr(app.StatusBanner, 36), 1.1, 0.45, 0.95, 0.65, 1.0)
	}
}

// --- Left Panel: Active Project Info, Scene Hierarchy & .osmu Scripts ---

func (app *EditorApp) drawLeftHierarchy(r Rect, in *platform.InputState) {
	app.emitRect(r.X, r.Y, r.W, r.H, 0.10, 0.12, 0.15, 1.0)
	app.emitLine(r.X+r.W, r.Y, r.X+r.W, r.Y+r.H, 1.5, 0.22, 0.26, 0.34, 1.0)

	// Project Header Banner
	app.emitRect(r.X, r.Y, r.W, 40, 0.13, 0.17, 0.24, 1.0)
	projName := "Standalone Scene"
	projDir := "."
	if app.CurrentProject != nil {
		projName = app.CurrentProject.Manifest.Name
		projDir = app.CurrentProject.RootDir
	}
	app.emitText(r.X+8, r.Y+6, "PROJECT: "+truncateStr(projName, 21), 1.25, 0.98, 0.90, 0.55, 1.0)
	app.emitText(r.X+8, r.Y+23, truncateStr(projDir+"/project.osmu.json", 32), 1.05, 0.62, 0.78, 0.95, 1.0)

	// Scene Hierarchy Section
	hy := r.Y + 42
	app.emitRect(r.X, hy, r.W, 24, 0.14, 0.16, 0.21, 1.0)
	app.emitText(r.X+10, hy+7, "SCENE HIERARCHY", 1.3, 0.75, 0.84, 0.96, 1.0)

	by := hy + 28
	if app.drawButton(Rect{r.X + 8, by, 74, 22}, "+Cube", false, in) && app.Scene != nil {
		id := fmt.Sprintf("ent_cube_%d", len(app.Scene.EntityList)+1)
		e := engine.NewEntity(id, fmt.Sprintf("Cube_%d", len(app.Scene.EntityList)+1), "Enemy", engine.MeshCube, engine.V3(0, 1.0, 0), engine.RGB(0.95, 0.40, 0.35))
		app.Scene.AddEntity(e)
		app.SelectedEntityID = e.ID
		app.runLiveQuery()
	}
	if app.drawButton(Rect{r.X + 86, by, 80, 22}, "+Sphere", false, in) && app.Scene != nil {
		id := fmt.Sprintf("ent_sph_%d", len(app.Scene.EntityList)+1)
		e := engine.NewEntity(id, fmt.Sprintf("Core_%d", len(app.Scene.EntityList)+1), "Coin", engine.MeshSphere, engine.V3(1.5, 0.8, 1.5), engine.RGB(1.0, 0.85, 0.25))
		app.Scene.AddEntity(e)
		app.SelectedEntityID = e.ID
		app.runLiveQuery()
	}
	if app.drawButton(Rect{r.X + 170, by, 76, 22}, "Delete", false, in) && app.Scene != nil && app.SelectedEntityID != "" {
		app.Scene.RemoveEntity(app.SelectedEntityID)
		app.selectDefaultEntity()
		app.runLiveQuery()
	}

	y := by + 28
	if app.Scene != nil {
		for _, ent := range app.Scene.EntityList {
			if ent == nil {
				continue
			}
			if y > r.Y+r.H-150 {
				break
			}
			rowRect := Rect{r.X + 6, y, r.W - 12, 21}
			isSel := ent.ID == app.SelectedEntityID
			isMatched := app.QueryMatches[ent.ID]

			if isSel {
				app.emitRect(rowRect.X, rowRect.Y, rowRect.W, rowRect.H, 0.20, 0.42, 0.72, 1.0)
			} else if isMatched {
				app.emitRect(rowRect.X, rowRect.Y, rowRect.W, rowRect.H, 0.16, 0.28, 0.24, 1.0)
			} else {
				app.emitRect(rowRect.X, rowRect.Y, rowRect.W, rowRect.H, 0.13, 0.15, 0.19, 1.0)
			}

			if in.MouseClicked[platform.MouseLeft] && rowRect.Contains(in.MouseX, in.MouseY) {
				app.SelectedEntityID = ent.ID
				app.runLiveQuery()
			}

			app.emitCircle(rowRect.X+11, rowRect.Y+10.5, 4.5, ent.Render.Color.R, ent.Render.Color.G, ent.Render.Color.B, 1.0)

			label := truncateStr(ent.Name, 17)
			if !ent.Active {
				label += " (off)"
			}
			textCol := engine.RGB(0.92, 0.94, 0.98)
			if !ent.Active {
				textCol = engine.RGB(0.50, 0.52, 0.58)
			}
			app.emitText(rowRect.X+22, rowRect.Y+6, label, 1.2, textCol.R, textCol.G, textCol.B, 1.0)

			tagStr := "[" + truncateStr(ent.Tag, 7) + "]"
			app.emitText(rowRect.X+rowRect.W-float64(len(tagStr))*7.2-4, rowRect.Y+6, tagStr, 1.1, 0.62, 0.78, 0.95, 1.0)

			y += 23
		}
	}

	// Bottom of Left Dock: .osmu Script Assets section
	scriptTop := r.Y + r.H - 140
	app.emitRect(r.X, scriptTop, r.W, 24, 0.14, 0.16, 0.21, 1.0)
	app.emitText(r.X+10, scriptTop+7, "PROJECT .OSMU SCRIPTS", 1.25, 0.75, 0.84, 0.96, 1.0)

	sy := scriptTop + 28
	if app.Scene != nil {
		for _, sf := range app.Scene.ScriptFiles {
			box := Rect{r.X + 8, sy, r.W - 16, 36}
			app.emitRect(box.X, box.Y, box.W, box.H, 0.14, 0.17, 0.22, 1.0)
			app.emitRectOutline(box.X, box.Y, box.W, box.H, 1.0, 0.26, 0.34, 0.46, 1.0)
			app.emitText(box.X+8, box.Y+6, truncateStr(sf.Path, 28), 1.2, 0.95, 0.88, 0.55, 1.0)
			status := "TypeCheck: 0 errors (LSP Ready)"
			col := engine.RGB(0.45, 0.92, 0.62)
			if len(sf.Diagnostics) > 0 {
				status = fmt.Sprintf("Errors: %d", len(sf.Diagnostics))
				col = engine.RGB(0.98, 0.45, 0.45)
			}
			app.emitText(box.X+8, box.Y+21, status, 1.1, col.R, col.G, col.B, 1.0)
			sy += 40
		}
	}
}

// --- Center Top: Live 3D / 2D Graphical Viewport with Solid Shaded 3D Rendering ---

func (app *EditorApp) drawCenterViewport(r Rect, in *platform.InputState) {
	app.emitRect(r.X, r.Y, r.W, r.H, 0.06, 0.08, 0.11, 1.0)
	app.emitRectOutline(r.X, r.Y, r.W, r.H, 1.0, 0.22, 0.26, 0.34, 1.0)

	if app.Scene == nil {
		return
	}

	if app.Scene.Mode == runtime.Mode2D {
		app.drawViewport2D(r, in)
	} else {
		app.drawViewport3D(r, in)
	}

	// Viewport Top HUD Overlay
	app.emitRect(r.X+8, r.Y+8, 390, 22, 0.09, 0.11, 0.15, 0.88)
	modeText := fmt.Sprintf("VIEWPORT [%s] | %s", app.Scene.Mode, truncateStr(app.Scene.Name, 34))
	app.emitText(r.X+14, r.Y+14, modeText, 1.25, 0.82, 0.90, 1.0, 1.0)

	// Bottom Controls Legend + Status Toast
	helpRect := Rect{r.X + 8, r.Y + r.H - 24, r.W - 16, 18}
	app.emitRect(helpRect.X, helpRect.Y, helpRect.W, helpRect.H, 0.08, 0.10, 0.14, 0.85)
	legend := "WASD/Arrows: Move Hero (in Play Mode) | LMB: Select/Gizmo | RMB: Orbit/Pan | Scroll: Zoom"
	if app.StatusBanner != "" && time.Since(app.StatusBannerTime) < 4*time.Second {
		legend = "STATUS: " + app.StatusBanner
	}
	app.emitText(helpRect.X+6, helpRect.Y+4, truncateStr(legend, 82), 1.15, 0.65, 0.86, 0.96, 1.0)
}

func (app *EditorApp) cameraEye3D() engine.Vec3 {
	yawRad := app.CamYaw * math.Pi / 180.0
	pitchRad := app.CamPitch * math.Pi / 180.0
	cx := app.CamDist * math.Cos(pitchRad) * math.Sin(yawRad)
	cy := app.CamDist * math.Sin(pitchRad)
	cz := app.CamDist * math.Cos(pitchRad) * math.Cos(yawRad)
	return app.CamTarget.Add(engine.V3(cx, cy, cz))
}

func (app *EditorApp) project3D(pt engine.Vec3, vpMat engine.Mat4, r Rect) (engine.Vec2, bool) {
	ndc, w := vpMat.TransformPoint(pt)
	if w <= 0.1 {
		return engine.Vec2{}, false
	}
	sx := r.X + (ndc.X*0.5+0.5)*r.W
	sy := r.Y + (1.0-(ndc.Y*0.5+0.5))*r.H
	inBounds := sx >= r.X-80 && sx <= r.X+r.W+80 && sy >= r.Y-80 && sy <= r.Y+r.H+80
	return engine.V2(sx, sy), inBounds
}

func (app *EditorApp) drawViewport3D(r Rect, in *platform.InputState) {
	if r.Contains(in.MouseX, in.MouseY) {
		if in.MouseClicked[platform.MouseRight] || in.MouseClicked[platform.MouseMiddle] {
			app.DraggingOrbit = true
		}
		if in.ScrollY != 0 {
			app.CamDist = engine.Clamp(app.CamDist-in.ScrollY*1.2, 4.0, 42.0)
		}
	}
	if !in.MouseDown[platform.MouseRight] && !in.MouseDown[platform.MouseMiddle] {
		app.DraggingOrbit = false
	}
	if app.DraggingOrbit {
		app.CamYaw -= in.MouseDX * 0.45
		app.CamPitch = engine.Clamp(app.CamPitch+in.MouseDY*0.35, 8.0, 82.0)
	}

	aspect := r.W / math.Max(1.0, r.H)
	proj := engine.PerspectiveMat4(50.0, aspect, 0.1, 100.0)
	eye := app.cameraEye3D()
	view := engine.LookAtMat4(eye, app.CamTarget, engine.V3(0, 1, 0))
	vp := proj.Mul(view)

	// Draw 3D ground grid
	for i := -9; i <= 9; i++ {
		fi := float64(i)
		p1, ok1 := app.project3D(engine.V3(fi, 0, -9), vp, r)
		p2, ok2 := app.project3D(engine.V3(fi, 0, 9), vp, r)
		if ok1 && ok2 {
			if i == 0 {
				app.emitLineClipped(r, p1, p2, 2.0, 0.25, 0.45, 0.85, 0.9)
			} else {
				app.emitLineClipped(r, p1, p2, 1.0, 0.15, 0.17, 0.22, 0.7)
			}
		}
		q1, ok3 := app.project3D(engine.V3(-9, 0, fi), vp, r)
		q2, ok4 := app.project3D(engine.V3(9, 0, fi), vp, r)
		if ok3 && ok4 {
			if i == 0 {
				app.emitLineClipped(r, q1, q2, 2.0, 0.85, 0.28, 0.32, 0.9)
			} else {
				app.emitLineClipped(r, q1, q2, 1.0, 0.15, 0.17, 0.22, 0.7)
			}
		}
	}

	selEnt := app.SelectedEntity()
	if !in.MouseDown[platform.MouseLeft] {
		app.DraggingGizmoAxis = 0
	}
	if app.DraggingGizmoAxis != 0 && selEnt != nil {
		sens := 0.035
		switch app.DraggingGizmoAxis {
		case 1:
			selEnt.Transform.Position.X += in.MouseDX * sens
		case 2:
			selEnt.Transform.Position.Y -= in.MouseDY * sens
		case 3:
			selEnt.Transform.Position.Z += in.MouseDY * sens
		}
		app.runLiveQuery()
	}

	// Sort active 3D entities back-to-front by distance from camera eye (Painter's algorithm)
	sorted := make([]*engine.Entity, 0, len(app.Scene.EntityList))
	for _, ent := range app.Scene.EntityList {
		if ent != nil && ent.Active && ent.Render.Visible && ent.Render.Mesh != engine.MeshPlane {
			sorted = append(sorted, ent)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Transform.Position.Dist(eye) > sorted[j].Transform.Position.Dist(eye)
	})

	var clickedID string
	bestClickDist := 28.0

	for _, ent := range sorted {
		center2D, ok := app.project3D(ent.Transform.Position, vp, r)
		if !ok {
			continue
		}

		if in.MouseClicked[platform.MouseLeft] && r.Contains(in.MouseX, in.MouseY) && app.DraggingGizmoAxis == 0 {
			d := center2D.Dist(engine.V2(in.MouseX, in.MouseY))
			if d < bestClickDist {
				bestClickDist = d
				clickedID = ent.ID
			}
		}

		if app.QueryMatches[ent.ID] {
			app.emitRing3D(r, vp, ent.Transform.Position, 0.95, 0.25, 0.98, 0.65, 0.9)
		}

		app.drawEntityMesh3D(r, vp, eye, ent, ent.ID == app.SelectedEntityID)

		if ent.Tag == "Enemy" || ent.Tag == "Relay" || ent.Tag == "Citadel" {
			topPos, tok := app.project3D(ent.Transform.Position.Add(engine.V3(0, ent.Transform.Scale.Y*0.65+0.4, 0)), vp, r)
			if tok && r.Contains(topPos.X, topPos.Y) {
				bw := 36.0
				ratio := engine.Clamp(ent.Health/math.Max(1.0, ent.MaxHealth), 0, 1)
				app.emitRect(topPos.X-bw*0.5, topPos.Y-10, bw, 4.5, 0.12, 0.12, 0.15, 0.92)
				hpCol := engine.RGB(0.25, 0.95, 0.48)
				if ent.Tag == "Enemy" {
					hpCol = engine.RGB(0.98, 0.35, 0.38)
				}
				app.emitRect(topPos.X-bw*0.5, topPos.Y-10, bw*ratio, 4.5, hpCol.R, hpCol.G, hpCol.B, 0.95)
			}
		}

		if ent.Render.Label != "" && r.Contains(center2D.X, center2D.Y) {
			app.emitText(center2D.X-float64(len(ent.Render.Label))*4.2, center2D.Y-4, ent.Render.Label, 1.45, 1.0, 1.0, 1.0, 1.0)
		} else if ent.Tag != "Projectile" && ent.Tag != "Tile" && r.Contains(center2D.X, center2D.Y+16) {
			app.emitText(center2D.X-float64(len(ent.Name))*3.2, center2D.Y+14, ent.Name, 1.1, 0.86, 0.90, 0.98, 0.95)
		}
	}

	// Draw 3D Transform Gizmo on selected entity
	if selEnt != nil && selEnt.Active {
		orig2D, ok := app.project3D(selEnt.Transform.Position, vp, r)
		if ok && r.Contains(orig2D.X, orig2D.Y) {
			gizmoLen := 1.5
			xTip, okX := app.project3D(selEnt.Transform.Position.Add(engine.V3(gizmoLen, 0, 0)), vp, r)
			yTip, okY := app.project3D(selEnt.Transform.Position.Add(engine.V3(0, gizmoLen, 0)), vp, r)
			zTip, okZ := app.project3D(selEnt.Transform.Position.Add(engine.V3(0, 0, gizmoLen)), vp, r)

			if okX {
				app.emitLineClipped(r, orig2D, xTip, 3.0, 0.98, 0.28, 0.32, 1.0)
				app.emitCircle(xTip.X, xTip.Y, 5.5, 0.98, 0.28, 0.32, 1.0)
				if in.MouseClicked[platform.MouseLeft] && xTip.Dist(engine.V2(in.MouseX, in.MouseY)) < 12 {
					app.DraggingGizmoAxis = 1
					clickedID = ""
				}
			}
			if okY {
				app.emitLineClipped(r, orig2D, yTip, 3.0, 0.30, 0.95, 0.45, 1.0)
				app.emitCircle(yTip.X, yTip.Y, 5.5, 0.30, 0.95, 0.45, 1.0)
				if in.MouseClicked[platform.MouseLeft] && yTip.Dist(engine.V2(in.MouseX, in.MouseY)) < 12 {
					app.DraggingGizmoAxis = 2
					clickedID = ""
				}
			}
			if okZ {
				app.emitLineClipped(r, orig2D, zTip, 3.0, 0.32, 0.58, 0.98, 1.0)
				app.emitCircle(zTip.X, zTip.Y, 5.5, 0.32, 0.58, 0.98, 1.0)
				if in.MouseClicked[platform.MouseLeft] && zTip.Dist(engine.V2(in.MouseX, in.MouseY)) < 12 {
					app.DraggingGizmoAxis = 3
					clickedID = ""
				}
			}
		}
	}

	if clickedID != "" {
		app.SelectedEntityID = clickedID
		app.runLiveQuery()
		if app.Scene.Playing && app.Scene.EventBus() != nil {
			if clickedEnt := app.Scene.FindEntityByID(clickedID); clickedEnt != nil {
				app.Scene.EventBus().Emit("EntityClicked", "", clickedEnt)
			}
		}
	}
}

func (app *EditorApp) drawEntityMesh3D(r Rect, vp engine.Mat4, eye engine.Vec3, ent *engine.Entity, selected bool) {
	pos := ent.Transform.Position
	sc := ent.Transform.Scale
	col := ent.Render.Color
	lineWidth := 1.4
	if selected {
		lineWidth = 2.6
	}

	hx := math.Max(0.22, sc.X*0.5)
	hy := math.Max(0.22, sc.Y*0.5)
	hz := math.Max(0.22, sc.Z*0.5)

	switch ent.Render.Mesh {
	case engine.MeshSphere, engine.MeshCircle2D, engine.MeshRing2D:
		c2d, ok := app.project3D(pos, vp, r)
		edge2d, ok2 := app.project3D(pos.Add(engine.V3(hx, 0, 0)), vp, r)
		if ok && ok2 && r.Contains(c2d.X, c2d.Y) {
			rad := math.Max(5.0, c2d.Dist(edge2d))
			app.emitCircle(c2d.X, c2d.Y, rad, col.R*0.78, col.G*0.78, col.B*0.78, 0.88)
			app.emitCircle(c2d.X-rad*0.25, c2d.Y-rad*0.25, rad*0.35, math.Min(1, col.R+0.25), math.Min(1, col.G+0.25), math.Min(1, col.B+0.25), 0.85)
		}
		app.emitRing3D(r, vp, pos, hx, col.R, col.G, col.B, 0.95)

	case engine.MeshPyramid:
		top := pos.Add(engine.V3(0, hy*1.1, 0))
		corners := [4]engine.Vec3{
			pos.Add(engine.V3(-hx, -hy, -hz)),
			pos.Add(engine.V3(hx, -hy, -hz)),
			pos.Add(engine.V3(hx, -hy, hz)),
			pos.Add(engine.V3(-hx, -hy, hz)),
		}
		top2D, tok := app.project3D(top, vp, r)
		var c2D [4]engine.Vec2
		var cok [4]bool
		for i := 0; i < 4; i++ {
			c2D[i], cok[i] = app.project3D(corners[i], vp, r)
		}
		shades := [4]float64{0.85, 0.72, 0.92, 0.65}
		for i := 0; i < 4; i++ {
			next := (i + 1) % 4
			if tok && cok[i] && cok[next] {
				sh := shades[i]
				app.emitTriangleClipped(r, top2D, c2D[i], c2D[next], col.R*sh, col.G*sh, col.B*sh, 0.78)
				app.emitLineClipped(r, c2D[i], c2D[next], lineWidth, col.R, col.G, col.B, 0.95)
				app.emitLineClipped(r, c2D[i], top2D, lineWidth, col.R, col.G, col.B, 0.95)
			}
		}

	default: // MeshCube, MeshCylinder
		v := [8]engine.Vec3{
			pos.Add(engine.V3(-hx, -hy, -hz)),
			pos.Add(engine.V3(hx, -hy, -hz)),
			pos.Add(engine.V3(hx, -hy, hz)),
			pos.Add(engine.V3(-hx, -hy, hz)),
			pos.Add(engine.V3(-hx, hy, -hz)),
			pos.Add(engine.V3(hx, hy, -hz)),
			pos.Add(engine.V3(hx, hy, hz)),
			pos.Add(engine.V3(-hx, hy, hz)),
		}
		var p [8]engine.Vec2
		var okArr [8]bool
		for i := 0; i < 8; i++ {
			p[i], okArr[i] = app.project3D(v[i], vp, r)
		}
		// Render shaded top, front, and side faces
		faces := [][5]int{
			{0, 1, 5, 4, 70}, // North
			{1, 2, 6, 5, 78}, // East
			{3, 0, 4, 7, 64}, // West
			{2, 3, 7, 6, 86}, // South
			{4, 5, 6, 7, 98}, // Top
		}
		for _, f := range faces {
			if okArr[f[0]] && okArr[f[1]] && okArr[f[2]] && okArr[f[3]] {
				sh := float64(f[4]) / 100.0
				app.emitTriangleClipped(r, p[f[0]], p[f[1]], p[f[2]], col.R*sh, col.G*sh, col.B*sh, 0.76)
				app.emitTriangleClipped(r, p[f[0]], p[f[2]], p[f[3]], col.R*sh, col.G*sh, col.B*sh, 0.76)
			}
		}
		edges := [12][2]int{
			{0, 1}, {1, 2}, {2, 3}, {3, 0},
			{4, 5}, {5, 6}, {6, 7}, {7, 4},
			{0, 4}, {1, 5}, {2, 6}, {3, 7},
		}
		edgeCol := col
		if selected {
			edgeCol = engine.RGB(1.0, 0.95, 0.45)
		}
		for _, e := range edges {
			if okArr[e[0]] && okArr[e[1]] {
				app.emitLineClipped(r, p[e[0]], p[e[1]], lineWidth, edgeCol.R, edgeCol.G, edgeCol.B, 0.95)
			}
		}
	}

	// Forward aim vector for Turrets & Players
	if ent.Tag == "Turret" || ent.Tag == "Player" {
		yawRad := ent.Transform.Rotation.Y * math.Pi / 180.0
		fwd := pos.Add(engine.V3(math.Sin(yawRad)*1.4, 0, math.Cos(yawRad)*1.4))
		p0, ok0 := app.project3D(pos, vp, r)
		p1, ok1 := app.project3D(fwd, vp, r)
		if ok0 && ok1 {
			app.emitLineClipped(r, p0, p1, 2.2, 1.0, 0.95, 0.35, 0.95)
		}
	}
}

func (app *EditorApp) emitRing3D(r Rect, vp engine.Mat4, center engine.Vec3, radius, cr, cg, cb, ca float64) {
	steps := 18
	for i := 0; i < steps; i++ {
		a1 := float64(i) * 2 * math.Pi / float64(steps)
		a2 := float64(i+1) * 2 * math.Pi / float64(steps)
		p1, ok1 := app.project3D(center.Add(engine.V3(math.Cos(a1)*radius, 0, math.Sin(a1)*radius)), vp, r)
		p2, ok2 := app.project3D(center.Add(engine.V3(math.Cos(a2)*radius, 0, math.Sin(a2)*radius)), vp, r)
		if ok1 && ok2 {
			app.emitLineClipped(r, p1, p2, 1.8, cr, cg, cb, ca)
		}
	}
}

func (app *EditorApp) drawViewport2D(r Rect, in *platform.InputState) {
	if r.Contains(in.MouseX, in.MouseY) {
		if in.MouseDown[platform.MouseRight] || in.MouseDown[platform.MouseMiddle] {
			app.Cam2DOffset.X += in.MouseDX
			app.Cam2DOffset.Y += in.MouseDY
		}
		if in.ScrollY != 0 {
			app.Cam2DZoom = engine.Clamp(app.Cam2DZoom+in.ScrollY*2.5, 10.0, 75.0)
		}
	}

	cx := r.X + r.W*0.5 + app.Cam2DOffset.X
	cy := r.Y + r.H*0.5 + app.Cam2DOffset.Y
	zm := app.Cam2DZoom

	toScreen := func(v engine.Vec3) engine.Vec2 {
		return engine.V2(cx+v.X*zm, cy-v.Y*zm)
	}

	for i := -12; i <= 12; i++ {
		p1 := toScreen(engine.V3(float64(i), -10, 0))
		p2 := toScreen(engine.V3(float64(i), 10, 0))
		app.emitLineClipped(r, p1, p2, 1.0, 0.12, 0.15, 0.21, 0.8)
		q1 := toScreen(engine.V3(-12, float64(i), 0))
		q2 := toScreen(engine.V3(12, float64(i), 0))
		app.emitLineClipped(r, q1, q2, 1.0, 0.12, 0.15, 0.21, 0.8)
	}

	for _, ent := range app.Scene.EntityList {
		if ent == nil || !ent.Active || !ent.Render.Visible {
			continue
		}
		sp := toScreen(ent.Transform.Position)
		if !r.Contains(sp.X, sp.Y) {
			continue
		}
		rad := math.Max(6.5, ent.Transform.Scale.X*zm*0.42)
		if ent.Tag == "Projectile" {
			rad = 4.5
		}
		if app.QueryMatches[ent.ID] {
			app.emitCircle(sp.X, sp.Y, rad+5.0, 0.20, 0.95, 0.65, 0.35)
		}
		if ent.Render.Mesh == engine.MeshSprite2D {
			if ent.Render.Wireframe {
				app.emitRectOutline(sp.X-rad, sp.Y-rad, rad*2, rad*2, 2.8, ent.Render.Color.R, ent.Render.Color.G, ent.Render.Color.B, 1.0)
			} else {
				app.emitRect(sp.X-rad, sp.Y-rad, rad*2, rad*2, ent.Render.Color.R, ent.Render.Color.G, ent.Render.Color.B, 0.95)
				app.emitRectOutline(sp.X-rad, sp.Y-rad, rad*2, rad*2, 1.5, 1.0, 1.0, 1.0, 0.7)
			}
		} else {
			app.emitCircle(sp.X, sp.Y, rad, ent.Render.Color.R, ent.Render.Color.G, ent.Render.Color.B, 0.95)
		}
		if ent.Tag == "Enemy" {
			bw := 28.0
			ratio := engine.Clamp(ent.Health/math.Max(1.0, ent.MaxHealth), 0, 1)
			app.emitRect(sp.X-bw*0.5, sp.Y-rad-8, bw, 3.5, 0.15, 0.15, 0.18, 0.9)
			app.emitRect(sp.X-bw*0.5, sp.Y-rad-8, bw*ratio, 3.5, 0.98, 0.35, 0.42, 0.95)
		}
		if ent.ID == app.SelectedEntityID {
			app.emitRectOutline(sp.X-rad-4, sp.Y-rad-4, (rad+4)*2, (rad+4)*2, 2.0, 1.0, 0.92, 0.35, 1.0)
		}
		if ent.Render.Label != "" {
			app.emitText(sp.X-float64(len(ent.Render.Label))*4.8, sp.Y-5.5, ent.Render.Label, 1.6, 1.0, 1.0, 1.0, 1.0)
		} else if ent.Tag != "Projectile" && ent.Tag != "Tile" {
			app.emitText(sp.X-float64(len(ent.Name))*3.2, sp.Y+rad+5, ent.Name, 1.1, 0.85, 0.90, 0.98, 1.0)
		}

		if in.MouseClicked[platform.MouseLeft] && sp.Dist(engine.V2(in.MouseX, in.MouseY)) <= rad+3 {
			app.SelectedEntityID = ent.ID
			app.runLiveQuery()
			if app.Scene.Playing && app.Scene.EventBus() != nil {
				app.Scene.EventBus().Emit("EntityClicked", "", ent)
			}
		}
		if in.MouseClicked[platform.MouseRight] && sp.Dist(engine.V2(in.MouseX, in.MouseY)) <= rad+3 {
			if app.Scene.Playing && app.Scene.EventBus() != nil {
				app.Scene.EventBus().Emit("EntityRightClicked", "", ent)
			}
		}
	}
}

// --- Center Bottom: Blender-Style Visual Node Editor & Live Query Bar ---

func (app *EditorApp) drawNodeAndQueryPanel(r Rect, in *platform.InputState) {
	app.emitRect(r.X, r.Y, r.W, r.H, 0.09, 0.10, 0.13, 1.0)
	app.emitLine(r.X, r.Y, r.X+r.W, r.Y, 1.5, 0.24, 0.28, 0.38, 1.0)

	qBarH := 32.0
	app.emitRect(r.X, r.Y, r.W, qBarH, 0.13, 0.15, 0.20, 1.0)
	app.emitText(r.X+10, r.Y+10, "LIVE QUERY:", 1.3, 0.45, 0.88, 1.0, 1.0)

	inputBox := Rect{r.X + 106, r.Y + 4, r.W - 390, 24}
	if inputBox.W < 120 {
		inputBox.W = 120
	}
	if in.MouseClicked[platform.MouseLeft] {
		app.QueryFocused = inputBox.Contains(in.MouseX, in.MouseY)
	}
	bgCol := engine.RGB(0.07, 0.08, 0.11)
	borderCol := engine.RGB(0.28, 0.34, 0.46)
	if app.QueryFocused {
		borderCol = engine.RGB(0.30, 0.82, 1.0)
	}
	app.emitRect(inputBox.X, inputBox.Y, inputBox.W, inputBox.H, bgCol.R, bgCol.G, bgCol.B, 1.0)
	app.emitRectOutline(inputBox.X, inputBox.Y, inputBox.W, inputBox.H, 1.5, borderCol.R, borderCol.G, borderCol.B, 1.0)

	qDisplay := app.QueryText
	if app.QueryFocused {
		qDisplay += "_"
	}
	app.emitText(inputBox.X+8, inputBox.Y+7, truncateStr(qDisplay, int(inputBox.W/8.0)), 1.25, 0.95, 0.96, 1.0, 1.0)

	bx := inputBox.X + inputBox.W + 8
	if app.drawButton(Rect{bx, r.Y + 4, 66, 24}, "Enemies", false, in) {
		app.QueryText = "Tag == \"Enemy\" && Health > 0 orderby Distance asc"
		app.runLiveQuery()
	}
	if app.drawButton(Rect{bx + 70, r.Y + 4, 56, 24}, "Coins", false, in) {
		app.QueryText = "Tag == \"Coin\""
		app.runLiveQuery()
	}
	if app.drawButton(Rect{bx + 130, r.Y + 4, 44, 24}, "All", false, in) {
		app.QueryText = "all"
		app.runLiveQuery()
	}
	matchBadge := fmt.Sprintf("Matched: %d", app.QueryMatchCount)
	app.emitText(bx+182, r.Y+10, matchBadge, 1.25, 0.45, 0.95, 0.65, 1.0)

	subY := r.Y + qBarH
	subH := 26.0
	app.emitRect(r.X, subY, r.W, subH, 0.11, 0.13, 0.17, 1.0)
	app.emitText(r.X+10, subY+7, "BLENDER-STYLE NODE EDITOR (Auto-Bundled Signal & Query Graph)", 1.2, 0.78, 0.84, 0.95, 1.0)

	nx := r.X + r.W - 295
	if app.drawButton(Rect{nx, subY + 2, 68, 22}, "+Signal", false, in) && app.Scene != nil && app.Scene.NodeGraph != nil {
		g := app.Scene.NodeGraph
		id := fmt.Sprintf("node_%d", len(g.Nodes)+1)
		g.Nodes = append(g.Nodes, nodegraph.CreateNodeFromTemplate(id, "OnSignal", 60, 140))
	}
	if app.drawButton(Rect{nx + 72, subY + 2, 68, 22}, "+Query", false, in) && app.Scene != nil && app.Scene.NodeGraph != nil {
		g := app.Scene.NodeGraph
		id := fmt.Sprintf("node_%d", len(g.Nodes)+1)
		g.Nodes = append(g.Nodes, nodegraph.CreateNodeFromTemplate(id, "QueryEntities", 300, 140))
	}
	if app.drawButton(Rect{nx + 144, subY + 2, 68, 22}, "+Action", false, in) && app.Scene != nil && app.Scene.NodeGraph != nil {
		g := app.Scene.NodeGraph
		id := fmt.Sprintf("node_%d", len(g.Nodes)+1)
		g.Nodes = append(g.Nodes, nodegraph.CreateNodeFromTemplate(id, "ModifyProperty", 520, 140))
	}
	if app.drawButton(Rect{nx + 216, subY + 2, 72, 22}, "ClearWire", false, in) && app.Scene != nil && app.Scene.NodeGraph != nil && app.SelectedNodeID != "" {
		app.Scene.NodeGraph.RemoveLinksForNode(app.SelectedNodeID)
	}

	canvasRect := Rect{r.X, subY + subH, r.W, r.H - qBarH - subH}
	app.drawNodeCanvas(canvasRect, in)
}

func (app *EditorApp) drawNodeCanvas(r Rect, in *platform.InputState) {
	if app.Scene == nil || app.Scene.NodeGraph == nil {
		return
	}
	g := app.Scene.NodeGraph

	step := 32.0
	for gx := r.X + math.Mod(app.NodeCanvasPan.X, step); gx < r.X+r.W; gx += step {
		if gx > r.X {
			app.emitLine(gx, r.Y, gx, r.Y+r.H, 1.0, 0.12, 0.14, 0.18, 1.0)
		}
	}
	for gy := r.Y + math.Mod(app.NodeCanvasPan.Y, step); gy < r.Y+r.H; gy += step {
		if gy > r.Y {
			app.emitLine(r.X, gy, r.X+r.W, gy, 1.0, 0.12, 0.14, 0.18, 1.0)
		}
	}

	if r.Contains(in.MouseX, in.MouseY) && (in.MouseDown[platform.MouseRight] || in.MouseDown[platform.MouseMiddle]) {
		app.NodeCanvasPan.X += in.MouseDX
		app.NodeCanvasPan.Y += in.MouseDY
	}

	offsetX := r.X + app.NodeCanvasPan.X
	offsetY := r.Y + app.NodeCanvasPan.Y

	bundles := g.ComputeWireBundles()
	for _, b := range bundles {
		col := socketColor(b.Kind)
		if b.Active {
			col = engine.RGB(1.0, 0.92, 0.32)
		}
		if !b.IsBundled || len(b.Wires) == 1 {
			w := b.Wires[0]
			p0 := engine.V2(offsetX+w.Start.X, offsetY+w.Start.Y)
			p3 := engine.V2(offsetX+w.End.X, offsetY+w.End.Y)
			pts := nodegraph.SampleCubicBezier(p0, p3, 14)
			app.emitPolyLineClipped(r, pts, 2.2, col.R, col.G, col.B, 0.92)
		} else {
			tStart := engine.V2(offsetX+b.TrunkStart.X, offsetY+b.TrunkStart.Y)
			tEnd := engine.V2(offsetX+b.TrunkEnd.X, offsetY+b.TrunkEnd.Y)

			for _, w := range b.Wires {
				sPos := engine.V2(offsetX+w.Start.X, offsetY+w.Start.Y)
				fanInPts := nodegraph.SampleCubicBezier(sPos, tStart, 10)
				app.emitPolyLineClipped(r, fanInPts, 1.8, col.R*0.85, col.G*0.85, col.B*0.85, 0.85)
			}

			app.emitLineClipped(r, tStart, tEnd, 5.5, col.R, col.G, col.B, 0.30)
			app.emitLineClipped(r, tStart, tEnd, 3.2, col.R, col.G, col.B, 1.0)
			app.emitCircle(tStart.X, tStart.Y, 4.2, col.R, col.G, col.B, 1.0)
			app.emitCircle(tEnd.X, tEnd.Y, 4.2, col.R, col.G, col.B, 1.0)

			for _, w := range b.Wires {
				ePos := engine.V2(offsetX+w.End.X, offsetY+w.End.Y)
				fanOutPts := nodegraph.SampleCubicBezier(tEnd, ePos, 10)
				app.emitPolyLineClipped(r, fanOutPts, 1.8, col.R*0.85, col.G*0.85, col.B*0.85, 0.85)
			}

			badge := engine.V2(offsetX+b.BadgePos.X, offsetY+b.BadgePos.Y)
			if r.Contains(badge.X, badge.Y) {
				app.emitRect(badge.X-13, badge.Y-9, 26, 17, 0.10, 0.13, 0.18, 0.95)
				app.emitRectOutline(badge.X-13, badge.Y-9, 26, 17, 1.2, col.R, col.G, col.B, 1.0)
				app.emitText(badge.X-8, badge.Y-4, b.BadgeLabel, 1.15, 1.0, 1.0, 1.0, 1.0)
			}
		}
	}

	now := time.Now()
	if !in.MouseDown[platform.MouseLeft] {
		app.DraggingNodeID = ""
	}
	if app.DraggingNodeID != "" {
		if dn := g.FindNode(app.DraggingNodeID); dn != nil {
			dn.X = (in.MouseX - offsetX) - app.NodeDragOffset.X
			dn.Y = (in.MouseY - offsetY) - app.NodeDragOffset.Y
		}
	}

	for _, n := range g.Nodes {
		nx := offsetX + n.X
		ny := offsetY + n.Y
		nRect := Rect{nx, ny, n.Width, n.Height}

		if ny+n.Height < r.Y || ny > r.Y+r.H || nx+n.Width < r.X || nx > r.X+r.W {
			continue
		}

		app.emitRect(nx, ny, n.Width, n.Height, 0.14, 0.16, 0.21, 0.96)
		hdrCol := categoryColor(n.Category)
		app.emitRect(nx, ny, n.Width, 24, hdrCol.R, hdrCol.G, hdrCol.B, 1.0)

		firedRecently := now.Sub(n.LastFired) < 400*time.Millisecond
		if firedRecently {
			app.emitRectOutline(nx-1, ny-1, n.Width+2, n.Height+2, 2.2, 1.0, 0.88, 0.30, 1.0)
		} else if n.ID == app.SelectedNodeID {
			app.emitRectOutline(nx, ny, n.Width, n.Height, 2.0, 0.38, 0.78, 1.0, 1.0)
		} else {
			app.emitRectOutline(nx, ny, n.Width, n.Height, 1.0, 0.26, 0.30, 0.38, 1.0)
		}

		app.emitText(nx+8, ny+7, truncateStr(n.Title, 21), 1.2, 1.0, 1.0, 1.0, 1.0)

		for _, sock := range n.Inputs {
			sx, sy := n.SocketPos(sock.Name, false)
			psx, psy := offsetX+sx, offsetY+sy
			scol := socketColor(sock.Kind)
			app.emitCircle(psx, psy, 5.0, scol.R, scol.G, scol.B, 1.0)
			app.emitText(psx+8, psy-4, sock.Name, 1.1, 0.82, 0.86, 0.94, 1.0)

			if app.WiringFromNode != "" && in.MouseReleased[platform.MouseLeft] {
				if engine.V2(psx, psy).Dist(engine.V2(in.MouseX, in.MouseY)) < 12 {
					g.AddLink(app.WiringFromNode, app.WiringFromSocket, n.ID, sock.Name, app.WiringKind)
				}
			}
		}

		for _, sock := range n.Outputs {
			sx, sy := n.SocketPos(sock.Name, true)
			psx, psy := offsetX+sx, offsetY+sy
			scol := socketColor(sock.Kind)
			app.emitCircle(psx, psy, 5.0, scol.R, scol.G, scol.B, 1.0)
			lblW := float64(len(sock.Name)) * 6.8
			app.emitText(psx-lblW-8, psy-4, sock.Name, 1.1, 0.82, 0.86, 0.94, 1.0)

			if in.MouseClicked[platform.MouseLeft] && engine.V2(psx, psy).Dist(engine.V2(in.MouseX, in.MouseY)) < 11 {
				app.WiringFromNode = n.ID
				app.WiringFromSocket = sock.Name
				app.WiringKind = sock.Kind
			}
		}

		maxSockets := len(n.Inputs)
		if len(n.Outputs) > maxSockets {
			maxSockets = len(n.Outputs)
		}
		py := ny + 34.0 + float64(maxSockets)*22.0
		for k, v := range n.Params {
			if py+14 > ny+n.Height {
				break
			}
			line := fmt.Sprintf("%s: %s", k, truncateStr(v, 16))
			app.emitText(nx+8, py, line, 1.05, 0.65, 0.76, 0.90, 1.0)
			py += 15
		}
		if n.LastResult != "" {
			app.emitText(nx+8, ny+n.Height-13, "-> "+truncateStr(n.LastResult, 20), 1.0, 0.45, 0.95, 0.65, 1.0)
		}

		if in.MouseClicked[platform.MouseLeft] && app.WiringFromNode == "" && nRect.Contains(in.MouseX, in.MouseY) && r.Contains(in.MouseX, in.MouseY) {
			app.SelectedNodeID = n.ID
			app.DraggingNodeID = n.ID
			app.NodeDragOffset = engine.V2((in.MouseX-offsetX)-n.X, (in.MouseY-offsetY)-n.Y)
		}
	}

	if app.WiringFromNode != "" {
		if srcNode := g.FindNode(app.WiringFromNode); srcNode != nil {
			sx, sy := srcNode.SocketPos(app.WiringFromSocket, true)
			p0 := engine.V2(offsetX+sx, offsetY+sy)
			p3 := engine.V2(in.MouseX, in.MouseY)
			pts := nodegraph.SampleCubicBezier(p0, p3, 12)
			app.emitPolyLineClipped(r, pts, 2.4, 1.0, 0.90, 0.30, 1.0)
		}
		if !in.MouseDown[platform.MouseLeft] {
			app.WiringFromNode = ""
		}
	}
}

func categoryColor(c nodegraph.NodeCategory) engine.Color {
	switch c {
	case nodegraph.CatEvent:
		return engine.RGB(0.68, 0.22, 0.28)
	case nodegraph.CatQuery:
		return engine.RGB(0.18, 0.52, 0.68)
	case nodegraph.CatAction:
		return engine.RGB(0.22, 0.56, 0.36)
	default:
		return engine.RGB(0.48, 0.36, 0.66)
	}
}

func socketColor(k nodegraph.SocketKind) engine.Color {
	switch k {
	case nodegraph.SocketFlow:
		return engine.RGB(0.95, 0.58, 0.22)
	case nodegraph.SocketEntityList:
		return engine.RGB(0.28, 0.86, 0.96)
	case nodegraph.SocketEntity:
		return engine.RGB(0.42, 0.92, 0.55)
	default:
		return engine.RGB(0.78, 0.78, 0.86)
	}
}

// --- Right Panel: Unity/Godot-Style Inspector & Live Event/Script Monitor ---

func (app *EditorApp) drawRightInspector(r Rect, in *platform.InputState) {
	app.emitRect(r.X, r.Y, r.W, r.H, 0.10, 0.12, 0.15, 1.0)
	app.emitLine(r.X, r.Y, r.X, r.Y+r.H, 1.5, 0.22, 0.26, 0.34, 1.0)

	app.emitRect(r.X, r.Y, r.W, 26, 0.14, 0.16, 0.21, 1.0)
	app.emitText(r.X+10, r.Y+8, "INSPECTOR", 1.35, 0.75, 0.84, 0.96, 1.0)

	y := r.Y + 34
	ent := app.SelectedEntity()
	if ent != nil {
		app.emitText(r.X+10, y, "Entity: "+ent.Name, 1.35, 1.0, 0.95, 0.75, 1.0)
		if app.drawButton(Rect{r.X + r.W - 76, y - 4, 66, 20}, fmt.Sprintf("Act:%v", ent.Active), ent.Active, in) {
			ent.Active = !ent.Active
			app.runLiveQuery()
		}
		y += 22
		app.emitText(r.X+10, y, fmt.Sprintf("Tag: [%s]   Mesh: %s", ent.Tag, ent.Render.Mesh), 1.2, 0.68, 0.82, 0.96, 1.0)
		y += 24

		app.emitRect(r.X+6, y, r.W-12, 20, 0.14, 0.17, 0.22, 1.0)
		app.emitText(r.X+12, y+5, "TRANSFORM", 1.2, 0.75, 0.85, 0.98, 1.0)
		y += 24

		y = app.drawFloatStepper(r.X+10, y, r.W-20, "Pos.X", &ent.Transform.Position.X, 0.5, in)
		y = app.drawFloatStepper(r.X+10, y, r.W-20, "Pos.Y", &ent.Transform.Position.Y, 0.5, in)
		y = app.drawFloatStepper(r.X+10, y, r.W-20, "Pos.Z", &ent.Transform.Position.Z, 0.5, in)
		y = app.drawFloatStepper(r.X+10, y, r.W-20, "Rot.Y", &ent.Transform.Rotation.Y, 15.0, in)

		y += 4
		app.emitRect(r.X+6, y, r.W-12, 20, 0.14, 0.17, 0.22, 1.0)
		app.emitText(r.X+12, y+5, "ATTRIBUTES & .OSMU COMPONENTS", 1.2, 0.75, 0.85, 0.98, 1.0)
		y += 24

		y = app.drawFloatStepper(r.X+10, y, r.W-20, "Health", &ent.Health, 10.0, in)
		y = app.drawFloatStepper(r.X+10, y, r.W-20, "Speed", &ent.Speed, 0.5, in)

		if scoreVal, ok := ent.Properties["Score"]; ok {
			sf, _ := engine.ToFloat64(scoreVal)
			app.emitText(r.X+10, y+4, fmt.Sprintf("Score: %.0f", sf), 1.25, 1.0, 0.88, 0.32, 1.0)
			y += 22
		}

		for _, sc := range ent.Scripts {
			app.emitRect(r.X+8, y, r.W-16, 42, 0.13, 0.18, 0.24, 1.0)
			app.emitRectOutline(r.X+8, y, r.W-16, 42, 1.0, 0.28, 0.45, 0.65, 1.0)
			app.emitText(r.X+14, y+6, "component "+sc.ComponentName+" : Entity", 1.2, 0.45, 0.92, 0.72, 1.0)
			app.emitText(r.X+14, y+22, truncateStr(sc.ScriptPath, 34), 1.1, 0.68, 0.76, 0.88, 1.0)
			y += 48
		}
	}

	logTop := r.Y + r.H - 185
	if logTop < y+10 {
		logTop = y + 10
	}
	app.emitRect(r.X, logTop, r.W, 22, 0.14, 0.16, 0.21, 1.0)
	app.emitText(r.X+10, logTop+6, "LIVE SIGNALS & .OSMU CONSOLE", 1.2, 0.75, 0.84, 0.96, 1.0)

	ly := logTop + 26
	if app.Scene != nil {
		events := app.Scene.EventBus().RecentEvents(4)
		for i := len(events) - 1; i >= 0; i-- {
			if ly+14 > r.Y+r.H {
				break
			}
			ev := events[i]
			app.emitText(r.X+8, ly, "sig> "+truncateStr(ev.Summary(), 36), 1.1, 0.98, 0.76, 0.35, 1.0)
			ly += 15
		}
		logs := app.Scene.RecentLogs(5)
		for i := len(logs) - 1; i >= 0; i-- {
			if ly+14 > r.Y+r.H {
				break
			}
			app.emitText(r.X+8, ly, truncateStr(logs[i], 38), 1.05, 0.68, 0.86, 0.74, 1.0)
			ly += 15
		}
	}
}

func (app *EditorApp) drawFloatStepper(x, y, w float64, label string, target *float64, step float64, in *platform.InputState) float64 {
	app.emitText(x, y+4, fmt.Sprintf("%-7s: %6.2f", label, *target), 1.2, 0.88, 0.92, 0.96, 1.0)
	if app.drawButton(Rect{x + w - 58, y, 26, 19}, "-", false, in) {
		*target -= step
		app.runLiveQuery()
	}
	if app.drawButton(Rect{x + w - 28, y, 26, 19}, "+", false, in) {
		*target += step
		app.runLiveQuery()
	}
	return y + 22
}

// --- UI Widget & Primitive Helpers ---

func (app *EditorApp) drawButton(r Rect, label string, active bool, in *platform.InputState) bool {
	if active {
		return app.drawButtonColor(r, label, true, 0.22, 0.48, 0.82, in)
	}
	return app.drawButtonColor(r, label, false, 0.17, 0.20, 0.26, in)
}

func (app *EditorApp) drawButtonColor(r Rect, label string, active bool, cr, cg, cb float64, in *platform.InputState) bool {
	hovered := r.Contains(in.MouseX, in.MouseY)
	if hovered {
		cr = math.Min(1.0, cr+0.08)
		cg = math.Min(1.0, cg+0.08)
		cb = math.Min(1.0, cb+0.08)
	}
	app.emitRect(r.X, r.Y, r.W, r.H, cr, cg, cb, 1.0)
	borderC := 0.30
	if active || hovered {
		borderC = 0.62
	}
	app.emitRectOutline(r.X, r.Y, r.W, r.H, 1.0, borderC, borderC+0.08, borderC+0.18, 1.0)
	textX := r.X + math.Max(4.0, (r.W-float64(len(label))*7.2)*0.5)
	textY := r.Y + (r.H-8.5)*0.5
	app.emitText(textX, textY, label, 1.2, 0.96, 0.98, 1.0, 1.0)
	return hovered && in.MouseClicked[platform.MouseLeft]
}

func (app *EditorApp) emitClear(r, g, b, a float64) {
	app.cmds = append(app.cmds, platform.DrawCmd{Kind: platform.CmdClear, R: r, G: g, B: b, A: a})
}

func (app *EditorApp) emitRect(x, y, w, h, r, g, b, a float64) {
	app.cmds = append(app.cmds, platform.DrawCmd{Kind: platform.CmdRect, X: x, Y: y, W: w, H: h, R: r, G: g, B: b, A: a})
}

func (app *EditorApp) emitTriangleClipped(bounds Rect, p1, p2, p3 engine.Vec2, r, g, b, a float64) {
	if !bounds.Contains(p1.X, p1.Y) && !bounds.Contains(p2.X, p2.Y) && !bounds.Contains(p3.X, p3.Y) {
		return
	}
	app.cmds = append(app.cmds, platform.DrawCmd{
		Kind: platform.CmdTriangle,
		X:    engine.Clamp(p1.X, bounds.X, bounds.X+bounds.W),
		Y:    engine.Clamp(p1.Y, bounds.Y, bounds.Y+bounds.H),
		X2:   engine.Clamp(p2.X, bounds.X, bounds.X+bounds.W),
		Y2:   engine.Clamp(p2.Y, bounds.Y, bounds.Y+bounds.H),
		X3:   engine.Clamp(p3.X, bounds.X, bounds.X+bounds.W),
		Y3:   engine.Clamp(p3.Y, bounds.Y, bounds.Y+bounds.H),
		R:    r, G: g, B: b, A: a,
	})
}

func (app *EditorApp) emitRectOutline(x, y, w, h, thickness, r, g, b, a float64) {
	app.cmds = append(app.cmds, platform.DrawCmd{Kind: platform.CmdRectOutline, X: x, Y: y, W: w, H: h, Thickness: thickness, R: r, G: g, B: b, A: a})
}

func (app *EditorApp) emitLine(x1, y1, x2, y2, thickness, r, g, b, a float64) {
	app.cmds = append(app.cmds, platform.DrawCmd{Kind: platform.CmdLine, X: x1, Y: y1, X2: x2, Y2: y2, Thickness: thickness, R: r, G: g, B: b, A: a})
}

func (app *EditorApp) emitLineClipped(bounds Rect, p1, p2 engine.Vec2, thickness, r, g, b, a float64) {
	if !bounds.Contains(p1.X, p1.Y) && !bounds.Contains(p2.X, p2.Y) {
		return
	}
	cx1 := engine.Clamp(p1.X, bounds.X, bounds.X+bounds.W)
	cy1 := engine.Clamp(p1.Y, bounds.Y, bounds.Y+bounds.H)
	cx2 := engine.Clamp(p2.X, bounds.X, bounds.X+bounds.W)
	cy2 := engine.Clamp(p2.Y, bounds.Y, bounds.Y+bounds.H)
	app.emitLine(cx1, cy1, cx2, cy2, thickness, r, g, b, a)
}

func (app *EditorApp) emitPolyLineClipped(bounds Rect, pts []engine.Vec2, thickness, r, g, b, a float64) {
	raw := make([][2]float64, 0, len(pts))
	for _, p := range pts {
		cx := engine.Clamp(p.X, bounds.X, bounds.X+bounds.W)
		cy := engine.Clamp(p.Y, bounds.Y, bounds.Y+bounds.H)
		raw = append(raw, [2]float64{cx, cy})
	}
	app.cmds = append(app.cmds, platform.DrawCmd{Kind: platform.CmdPolyLine, Points: raw, Thickness: thickness, R: r, G: g, B: b, A: a})
}

func (app *EditorApp) emitCircle(x, y, rad, r, g, b, a float64) {
	app.cmds = append(app.cmds, platform.DrawCmd{Kind: platform.CmdCircle, X: x, Y: y, Radius: rad, R: r, G: g, B: b, A: a})
}

func (app *EditorApp) emitText(x, y float64, text string, scale, r, g, b, a float64) {
	app.cmds = append(app.cmds, platform.DrawCmd{Kind: platform.CmdText, X: x, Y: y, Text: text, Scale: scale, R: r, G: g, B: b, A: a})
}

func truncateStr(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
