package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"osmu/pkg/engine"
	"osmu/pkg/nodegraph"
	"osmu/pkg/osmulang"
)

// ViewportMode specifies whether a scene defaults to 3D or 2D camera projection.
type ViewportMode string

const (
	Mode3D ViewportMode = "3D"
	Mode2D ViewportMode = "2D"
)

// ProjectManifest represents a standalone OSMU project's `project.osmu.json` file on disk.
type ProjectManifest struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	Genre        string   `json:"genre"`
	DefaultMode  string   `json:"default_mode"`
	DefaultScene string   `json:"default_scene"`
	Scenes       []string `json:"scenes"`
	Scripts      []string `json:"scripts"`
}

// Project represents a loaded OSMU game project with its directory path, manifest, scenes, and scripts.
type Project struct {
	RootDir      string           `json:"root_dir"`
	ManifestPath string           `json:"manifest_path"`
	Manifest     ProjectManifest  `json:"manifest"`
	LoadedScenes []*Scene         `json:"-"`
	ActiveScene  *Scene           `json:"-"`
}

// ScriptFile represents a loaded .osmu script and its latest compiler diagnostics.
type ScriptFile struct {
	Path        string                `json:"path"`
	Source      string                `json:"source"`
	CheckResult *osmulang.CheckResult `json:"-"`
	Diagnostics []osmulang.Diagnostic `json:"diagnostics"`
}

// Scene represents a complete OSMU 2D/3D game scene including entities, visual node graph, and .osmu scripts.
type Scene struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Mode        ViewportMode     `json:"mode"`
	EntityList  []*engine.Entity `json:"entities"`
	NodeGraph   *nodegraph.Graph `json:"node_graph"`
	ScriptFiles []*ScriptFile    `json:"scripts,omitempty"`
	FilePath    string           `json:"-"`

	// Runtime-only state
	mu          sync.RWMutex                  `json:"-"`
	bus         *engine.EventBus              `json:"-"`
	instances   []*osmulang.ComponentInstance `json:"-"`
	keysDown    map[string]bool               `json:"-"`
	logs        []string                      `json:"-"`
	snapshot    []byte                        `json:"-"`
	Playing     bool                          `json:"-"`
	Paused      bool                          `json:"-"`
	FrameCount  uint64                        `json:"-"`
	lastDt      float64                       `json:"-"`
	ElapsedTime float64                       `json:"-"`
	spawnSeq    int                           `json:"-"`
}

// NewScene creates an empty initialized Scene.
func NewScene(name string, mode ViewportMode) *Scene {
	return &Scene{
		Name:       name,
		Mode:       mode,
		EntityList: []*engine.Entity{},
		NodeGraph:  nodegraph.NewGraph(),
		bus:        engine.NewEventBus(),
		keysDown:   make(map[string]bool),
		lastDt:     0.016,
	}
}

// SetInputKeys updates the live keyboard state for player-controlled .osmu scripts.
func (s *Scene) SetInputKeys(keys map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keysDown == nil {
		s.keysDown = make(map[string]bool)
	}
	for k := range s.keysDown {
		delete(s.keysDown, k)
	}
	for k, v := range keys {
		if v {
			s.keysDown[strings.ToUpper(k)] = true
		}
	}
}

// InputAxisX implements osmulang.InteractiveWorld (-1 for A/Left, +1 for D/Right).
func (s *Scene) InputAxisX() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	x := 0.0
	if s.keysDown["A"] || s.keysDown["LEFT"] {
		x -= 1.0
	}
	if s.keysDown["D"] || s.keysDown["RIGHT"] {
		x += 1.0
	}
	return x
}

// InputAxisY implements osmulang.InteractiveWorld (-1 for S/Down, +1 for W/Up).
func (s *Scene) InputAxisY() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	y := 0.0
	if s.keysDown["S"] || s.keysDown["DOWN"] {
		y -= 1.0
	}
	if s.keysDown["W"] || s.keysDown["UP"] {
		y += 1.0
	}
	return y
}

// IsKeyDown implements osmulang.InteractiveWorld.
func (s *Scene) IsKeyDown(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keysDown[strings.ToUpper(key)]
}

// SpawnEntity implements osmulang.InteractiveWorld, dynamically spawning a projectile/entity at runtime.
func (s *Scene) SpawnEntity(name, tag, mesh string, pos, vel engine.Vec3) *engine.Entity {
	s.spawnSeq++
	id := fmt.Sprintf("spawn_%d_%d", s.FrameCount, s.spawnSeq)
	col := engine.RGB(0.35, 0.95, 1.0)
	if strings.EqualFold(tag, "Enemy") {
		col = engine.RGB(0.98, 0.30, 0.38)
	} else if strings.EqualFold(tag, "Gem") || strings.EqualFold(tag, "Coin") {
		col = engine.RGB(1.0, 0.88, 0.22)
	} else if strings.EqualFold(tag, "Projectile") {
		col = engine.RGB(0.25, 1.0, 0.85)
	}
	e := engine.NewEntity(id, name, tag, engine.MeshType(mesh), pos, col)
	e.Transform.Scale = engine.V3(0.45, 0.45, 0.45)
	e.Physics.Velocity = vel
	e.Physics.Radius = 0.45
	e.Health = 35.0
	e.MaxHealth = 35.0
	s.EntityList = append(s.EntityList, e)
	return e
}

// Entities implements nodegraph.SceneContext and osmulang.RuntimeWorld.
func (s *Scene) Entities() []*engine.Entity {
	return s.EntityList
}

// EventBus returns the scene's signal/event bus.
func (s *Scene) EventBus() *engine.EventBus {
	if s.bus == nil {
		s.bus = engine.NewEventBus()
	}
	return s.bus
}

// DeltaTime returns the most recent frame delta time in seconds.
func (s *Scene) DeltaTime() float64 {
	if s.lastDt <= 0 {
		return 0.016
	}
	return s.lastDt
}

// LogScript records a message from a .osmu script or compiler into the scene log.
func (s *Scene) LogScript(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stamp := time.Now().Format("15:04:05")
	s.logs = append(s.logs, fmt.Sprintf("[%s] %s", stamp, msg))
	if len(s.logs) > 80 {
		s.logs = s.logs[len(s.logs)-80:]
	}
}

// RecentLogs returns the most recent script/engine logs.
func (s *Scene) RecentLogs(n int) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n <= 0 || n > len(s.logs) {
		n = len(s.logs)
	}
	out := make([]string, n)
	copy(out, s.logs[len(s.logs)-n:])
	return out
}

// FindEntityByID finds an entity by ID.
func (s *Scene) FindEntityByID(id string) *engine.Entity {
	for _, e := range s.EntityList {
		if e != nil && e.ID == id {
			return e
		}
	}
	return nil
}

// AddEntity appends an entity to the scene.
func (s *Scene) AddEntity(e *engine.Entity) {
	s.EntityList = append(s.EntityList, e)
}

// RemoveEntity removes an entity by ID.
func (s *Scene) RemoveEntity(id string) {
	kept := s.EntityList[:0]
	for _, e := range s.EntityList {
		if e != nil && e.ID != id {
			kept = append(kept, e)
		}
	}
	s.EntityList = kept
}

// CompileScripts compiles and statically type-checks all attached .osmu scripts in the scene.
func (s *Scene) CompileScripts() []osmulang.Diagnostic {
	var allDiags []osmulang.Diagnostic
	for _, sf := range s.ScriptFiles {
		res := osmulang.CompileAndCheck(sf.Path, sf.Source)
		sf.CheckResult = res
		sf.Diagnostics = res.Diagnostics
		allDiags = append(allDiags, res.Diagnostics...)
	}
	return allDiags
}

// BindRuntimeInstances instantiates .osmu components for all entities and wires up the EventBus.
func (s *Scene) BindRuntimeInstances() {
	s.CompileScripts()
	s.instances = nil
	s.EventBus().ClearSubscriptions()

	compMap := make(map[string]*osmulang.ComponentDecl)
	for _, sf := range s.ScriptFiles {
		if sf.CheckResult != nil && sf.CheckResult.AST != nil && !sf.CheckResult.HasErrors() {
			for _, comp := range sf.CheckResult.AST.Components {
				compMap[comp.Name] = comp
			}
		}
	}

	for _, ent := range s.EntityList {
		if ent == nil || !ent.Active {
			continue
		}
		for _, att := range ent.Scripts {
			decl, ok := compMap[att.ComponentName]
			if !ok {
				continue
			}
			inst := osmulang.NewComponentInstance(decl, ent, s, att.Properties)
			s.instances = append(s.instances, inst)
		}
	}

	s.EventBus().On("*", "", func(ev engine.SignalEvent) {
		var srcEnt *engine.Entity
		if ev.SourceID != "" {
			srcEnt = s.FindEntityByID(ev.SourceID)
		}
		if s.NodeGraph != nil {
			s.NodeGraph.ExecuteEvent(s, "OnSignal", map[string]interface{}{
				"Signal": ev.Signal,
				"Source": srcEnt,
				"Target": srcEnt,
			})
		}
		for _, inst := range s.instances {
			if inst.Entity != nil && inst.Entity.Active && inst.Entity.ID != ev.SourceID {
				inst.HandleSignal(ev.Signal, ev.Args...)
			}
		}
	})
}

// StartPlay saves an editor snapshot of the scene, binds scripts, and starts Play Mode.
func (s *Scene) StartPlay() {
	if s.Playing {
		s.Paused = false
		return
	}
	data, err := json.Marshal(s)
	if err == nil {
		s.snapshot = data
	}
	s.Playing = true
	s.Paused = false
	s.FrameCount = 0
	s.ElapsedTime = 0
	s.BindRuntimeInstances()

	for _, inst := range s.instances {
		if inst.Entity != nil && inst.Entity.Active {
			inst.CallMethod("OnReady")
		}
	}
	if s.NodeGraph != nil {
		s.NodeGraph.ExecuteEvent(s, "OnStart", map[string]interface{}{})
	}
	s.LogScript("Entered Play Mode: " + s.Name)
}

// StopPlay exits Play Mode and restores the pre-play editor state snapshot.
func (s *Scene) StopPlay() {
	if !s.Playing {
		return
	}
	s.Playing = false
	s.Paused = false
	if len(s.snapshot) > 0 {
		var restored Scene
		if err := json.Unmarshal(s.snapshot, &restored); err == nil {
			s.EntityList = restored.EntityList
			if restored.NodeGraph != nil {
				s.NodeGraph = restored.NodeGraph
			}
		}
	}
	s.CompileScripts()
	s.LogScript("Stopped Play Mode (scene state restored)")
}

// Step advances the simulation by dt seconds (physics, .osmu scripts, collisions, and visual NodeGraph).
func (s *Scene) Step(dt float64) {
	if dt <= 0 || dt > 0.1 {
		dt = 0.016
	}
	s.lastDt = dt
	s.FrameCount++
	s.ElapsedTime += dt
	s.EventBus().SetFrame(s.FrameCount)

	for _, inst := range s.instances {
		if inst.Entity != nil && inst.Entity.Active {
			inst.CallMethod("OnUpdate", dt)
		}
	}

	if s.NodeGraph != nil {
		s.NodeGraph.ExecuteEvent(s, "OnUpdate", map[string]interface{}{
			"Dt": dt,
		})
	}

	s.stepPhysicsAndCollisions(dt)
}

func (s *Scene) stepPhysicsAndCollisions(dt float64) {
	for _, e := range s.EntityList {
		if e == nil || !e.Active || !e.Physics.Enabled || e.Physics.IsStatic {
			continue
		}
		if e.Physics.UseGravity {
			e.Physics.Velocity.Y -= 9.8 * dt
		}
		e.Transform.Position = e.Transform.Position.Add(e.Physics.Velocity.Scale(dt))
		e.Transform.Rotation = e.Transform.Rotation.Add(e.Physics.AngularVel.Scale(dt))
		if e.Physics.UseGravity && e.Transform.Position.Y < 0.5 {
			e.Transform.Position.Y = 0.5
			e.Physics.Velocity.Y = 0
		}
		// Auto-cull far projectiles
		if e.Tag == "Projectile" && e.Transform.Position.Len() > 24.0 {
			e.Active = false
		}
	}

	n := len(s.EntityList)
	for i := 0; i < n; i++ {
		a := s.EntityList[i]
		if a == nil || !a.Active || !a.Physics.Enabled {
			continue
		}
		for j := i + 1; j < n; j++ {
			b := s.EntityList[j]
			if b == nil || !b.Active || !b.Physics.Enabled {
				continue
			}
			if a.Physics.IsStatic && b.Physics.IsStatic {
				continue
			}
			ra := a.Physics.Radius
			if ra <= 0 {
				ra = 0.65
			}
			rb := b.Physics.Radius
			if rb <= 0 {
				rb = 0.65
			}
			dist := a.Transform.Position.Dist(b.Transform.Position)
			if dist <= ra+rb {
				s.dispatchCollision(a, b)
				s.dispatchCollision(b, a)
			}
		}
	}
}

func (s *Scene) dispatchCollision(self, other *engine.Entity) {
	// Built-in projectile vs Enemy hit logic
	if self.Tag == "Projectile" && other.Tag == "Enemy" && self.Active && other.Active {
		other.Health -= 35.0
		self.Active = false
		if other.Health <= 0 {
			other.Active = false
			s.EventBus().Emit("EnemyDefeated", self.ID, other.Name)
		}
	}
	if s.NodeGraph != nil {
		s.NodeGraph.ExecuteEvent(s, "OnCollision", map[string]interface{}{
			"Self":  self,
			"Other": other,
		})
	}
	for _, inst := range s.instances {
		if inst.Entity == self && self.Active {
			inst.CallMethod("OnCollision", other)
		}
	}
}

// SaveToFile writes the scene as formatted JSON to path.
func (s *Scene) SaveToFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	s.FilePath = path
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadSceneFromFile loads a scene JSON file and any referenced .osmu scripts.
func LoadSceneFromFile(path string) (*Scene, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc Scene
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, err
	}
	sc.FilePath = path
	sc.bus = engine.NewEventBus()
	sc.keysDown = make(map[string]bool)
	if sc.NodeGraph == nil {
		sc.NodeGraph = nodegraph.NewGraph()
	}
	baseDir := filepath.Dir(path)
	for _, sf := range sc.ScriptFiles {
		if sf.Path != "" {
			candidates := []string{
				sf.Path,
				filepath.Join(baseDir, sf.Path),
				filepath.Join(filepath.Dir(baseDir), sf.Path),
			}
			for _, cand := range candidates {
				if b, err := os.ReadFile(cand); err == nil {
					sf.Source = string(b)
					break
				}
			}
		}
	}
	sc.CompileScripts()
	return &sc, nil
}

// LoadProject loads an OSMU project from a project directory or a `project.osmu.json` file path.
func LoadProject(targetPath string) (*Project, error) {
	info, err := os.Stat(targetPath)
	if err != nil {
		return nil, err
	}
	var rootDir, manifestPath string
	if info.IsDir() {
		rootDir = targetPath
		manifestPath = filepath.Join(rootDir, "project.osmu.json")
	} else if strings.HasSuffix(targetPath, "project.osmu.json") {
		manifestPath = targetPath
		rootDir = filepath.Dir(targetPath)
	} else if strings.HasSuffix(targetPath, ".json") {
		// Loading a standalone scene file directly as a single-scene project
		sc, err := LoadSceneFromFile(targetPath)
		if err != nil {
			return nil, err
		}
		return &Project{
			RootDir:      filepath.Dir(targetPath),
			ManifestPath: targetPath,
			Manifest: ProjectManifest{
				Name:         sc.Name,
				Version:      "1.0.0",
				Description:  sc.Description,
				DefaultMode:  string(sc.Mode),
				DefaultScene: filepath.Base(targetPath),
				Scenes:       []string{filepath.Base(targetPath)},
			},
			LoadedScenes: []*Scene{sc},
			ActiveScene:  sc,
		}, nil
	} else {
		return nil, fmt.Errorf("unsupported project path: %s", targetPath)
	}

	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest ProjectManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}

	proj := &Project{
		RootDir:      rootDir,
		ManifestPath: manifestPath,
		Manifest:     manifest,
	}

	for _, relScene := range manifest.Scenes {
		sceneFull := filepath.Join(rootDir, relScene)
		sc, err := LoadSceneFromFile(sceneFull)
		if err != nil {
			continue
		}
		proj.LoadedScenes = append(proj.LoadedScenes, sc)
		if proj.ActiveScene == nil || relScene == manifest.DefaultScene {
			proj.ActiveScene = sc
		}
	}
	if proj.ActiveScene == nil && len(proj.LoadedScenes) > 0 {
		proj.ActiveScene = proj.LoadedScenes[0]
	}
	if proj.ActiveScene == nil {
		return nil, fmt.Errorf("project %q contains no valid scenes", manifest.Name)
	}
	return proj, nil
}

// SaveProject writes the project manifest, all loaded scenes, and all .osmu scripts to disk.
func (p *Project) SaveProject() error {
	if err := os.MkdirAll(p.RootDir, 0755); err != nil {
		return err
	}
	if strings.HasSuffix(p.ManifestPath, "project.osmu.json") {
		data, err := json.MarshalIndent(p.Manifest, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(p.ManifestPath, data, 0644); err != nil {
			return err
		}
	}
	for i, sc := range p.LoadedScenes {
		scenePath := sc.FilePath
		if scenePath == "" && i < len(p.Manifest.Scenes) {
			scenePath = filepath.Join(p.RootDir, p.Manifest.Scenes[i])
		}
		if scenePath == "" {
			scenePath = filepath.Join(p.RootDir, "scenes", "main.osmu.json")
		}
		for _, sf := range sc.ScriptFiles {
			if sf.Path != "" && sf.Source != "" {
				scriptPath := sf.Path
				if !filepath.IsAbs(scriptPath) && !strings.HasPrefix(scriptPath, p.RootDir) {
					scriptPath = filepath.Join(p.RootDir, sf.Path)
				}
				_ = os.MkdirAll(filepath.Dir(scriptPath), 0755)
				_ = os.WriteFile(scriptPath, []byte(sf.Source), 0644)
			}
		}
		if err := sc.SaveToFile(scenePath); err != nil {
			return err
		}
	}
	return nil
}

// DiscoverProjects scans baseDir (e.g. "." and "./examples" and "./projects") for `project.osmu.json` manifests.
func DiscoverProjects(baseDir string) []*Project {
	var manifestPaths []string
	_ = filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() && len(info.Name()) > 1 && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		if !info.IsDir() && info.Name() == "project.osmu.json" {
			manifestPaths = append(manifestPaths, path)
		}
		return nil
	})
	sort.Strings(manifestPaths)
	var out []*Project
	for _, mp := range manifestPaths {
		if proj, err := LoadProject(mp); err == nil {
			out = append(out, proj)
		}
	}
	return out
}

// CheckDirectoryOrFile compiles a single .osmu file or all .osmu files in a directory.
func CheckDirectoryOrFile(target string) ([]*ScriptFile, []osmulang.Diagnostic, error) {
	info, err := os.Stat(target)
	if err != nil {
		return nil, nil, err
	}
	var paths []string
	if !info.IsDir() {
		paths = append(paths, target)
	} else {
		err = filepath.Walk(target, func(p string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !fi.IsDir() && strings.HasSuffix(strings.ToLower(p), ".osmu") {
				paths = append(paths, p)
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	var files []*ScriptFile
	var allDiags []osmulang.Diagnostic
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, err
		}
		res := osmulang.CompileAndCheck(p, string(raw))
		files = append(files, &ScriptFile{
			Path:        p,
			Source:      string(raw),
			CheckResult: res,
			Diagnostics: res.Diagnostics,
		})
		allDiags = append(allDiags, res.Diagnostics...)
	}
	return files, allDiags, nil
}
