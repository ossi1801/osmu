package editor

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"

	"osmu/pkg/nodegraph"
)

// WebServer exposes a live browser studio mirror of the EditorApp.
type WebServer struct {
	mu  sync.Mutex
	App *EditorApp
}

// NewWebServer creates a new WebServer bound to app.
func NewWebServer(app *EditorApp) *WebServer {
	return &WebServer{App: app}
}

// Handler returns an http.Handler serving the live studio UI and JSON state/control API.
func (ws *WebServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", ws.handleState)
	mux.HandleFunc("/api/action", ws.handleAction)
	mux.HandleFunc("/", ws.handleIndex)
	return mux
}

func (ws *WebServer) handleState(w http.ResponseWriter, r *http.Request) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	sc := ws.App.Scene
	var bundles []nodegraph.WireBundle
	if sc != nil && sc.NodeGraph != nil {
		bundles = sc.NodeGraph.ComputeWireBundles()
	}

	resp := map[string]interface{}{
		"scene_index":   ws.App.CurrentSceneIdx,
		"scene":         sc,
		"selected_id":   ws.App.SelectedEntityID,
		"query_text":    ws.App.QueryText,
		"query_matches": ws.App.QueryMatches,
		"query_count":   ws.App.QueryMatchCount,
		"wire_bundles":  bundles,
		"events":        sc.EventBus().RecentEvents(8),
		"logs":          sc.RecentLogs(8),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (ws *WebServer) handleAction(w http.ResponseWriter, r *http.Request) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	act := r.URL.Query().Get("cmd")
	sc := ws.App.Scene
	switch act {
	case "play":
		if sc != nil {
			sc.StartPlay()
		}
	case "pause":
		if sc != nil {
			sc.Paused = !sc.Paused
		}
	case "stop":
		if sc != nil {
			sc.StopPlay()
			ws.App.runLiveQuery()
		}
	case "step":
		if sc != nil {
			sc.Step(0.05)
			ws.App.runLiveQuery()
		}
	case "scene":
		idx, _ := strconv.Atoi(r.URL.Query().Get("idx"))
		ws.App.SwitchScene(idx)
	case "query":
		ws.App.QueryText = r.URL.Query().Get("q")
		ws.App.runLiveQuery()
	case "select":
		ws.App.SelectedEntityID = r.URL.Query().Get("id")
		ws.App.runLiveQuery()
	case "toggle_group":
		if sc != nil && sc.NodeGraph != nil {
			sc.NodeGraph.AutoGroup = !sc.NodeGraph.AutoGroup
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (ws *WebServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>OSMU Engine — Live Graphical Studio</title>
<style>
  body { margin:0; background:#12151c; color:#e4ecfa; font-family:system-ui,sans-serif; overflow:hidden; }
  header { height:42px; background:#1a1f2b; display:flex; align-items:center; padding:0 12px; gap:8px; border-bottom:1px solid #2e374a; }
  button { background:#273042; color:#eef4ff; border:1px solid #3d4b66; border-radius:4px; padding:5px 10px; cursor:pointer; font-size:12px; }
  button.active { background:#2d6cdf; border-color:#6ca0ff; }
  .layout { display:grid; grid-template-columns:250px 1fr 300px; height:calc(100vh - 42px); }
  .panel { background:#161a23; border-right:1px solid #2a3242; padding:8px; overflow-y:auto; }
  .center { display:grid; grid-template-rows:55% 45%; }
  canvas { width:100%; height:100%; display:block; background:#0e1117; }
  .row { padding:5px 8px; margin-bottom:4px; background:#1e2430; border-radius:4px; cursor:pointer; font-size:12px; display:flex; justify-content:space-between; }
  .row.sel { background:#2b579a; }
</style>
</head>
<body>
<header>
  <strong style="color:#58a6ff;margin-right:8px;">OSMU ENGINE</strong>
  <button onclick="act('scene&idx=0')">1: Arena 3D</button>
  <button onclick="act('scene&idx=1')">2: Neon 2D</button>
  <button onclick="act('scene&idx=2')">3: Puzzle 3D</button>
  <button onclick="act('play')" style="background:#238636">PLAY</button>
  <button onclick="act('step')">STEP</button>
  <button onclick="act('stop')" style="background:#a4262c">STOP</button>
  <button onclick="act('toggle_group')">TOGGLE WIRE GROUPING</button>
  <input id="qbox" style="flex:1;background:#0d1117;color:#fff;border:1px solid #3d4b66;padding:4px 8px;" value='Tag == "Enemy" && Health > 0 orderby Distance asc' onchange="act('query&q='+encodeURIComponent(this.value))"/>
</header>
<div class="layout">
  <div class="panel" id="hier"></div>
  <div class="center">
    <canvas id="vp"></canvas>
    <canvas id="ng"></canvas>
  </div>
  <div class="panel" id="insp"></div>
</div>
<script>
let state = null;
function act(q){ fetch('/api/action?cmd='+q).then(poll); }
function poll(){
  fetch('/api/state').then(r=>r.json()).then(s=>{ state=s; render(); });
}
function render(){
  if(!state || !state.scene) return;
  const h = document.getElementById('hier');
  h.innerHTML = '<h4>SCENE HIERARCHY</h4>' + state.scene.entities.map(e =>
    '<div class="row '+(e.id===state.selected_id?'sel':'')+'" onclick="act(\'select&id='+e.id+'\')"><span>'+e.name+'</span><span>['+e.tag+']</span></div>'
  ).join('');
  const insp = document.getElementById('insp');
  const sel = state.scene.entities.find(e=>e.id===state.selected_id);
  if(sel){
    insp.innerHTML = '<h4>INSPECTOR: '+sel.name+'</h4><pre style="font-size:11px;color:#9cdcfe">'+JSON.stringify(sel,null,2)+'</pre>';
  }
  drawNodeCanvas();
}
function drawNodeCanvas(){
  const c = document.getElementById('ng');
  c.width = c.clientWidth; c.height = c.clientHeight;
  const ctx = c.getContext('2d');
  ctx.fillStyle = '#11141c'; ctx.fillRect(0,0,c.width,c.height);
  if(!state.wire_bundles) return;
  for(const b of state.wire_bundles){
    ctx.strokeStyle = b.Active ? '#ffd33d' : '#ff9436';
    if(!b.IsBundled){
      const w = b.Wires[0];
      ctx.lineWidth = 2; ctx.beginPath(); ctx.moveTo(w.Start.x, w.Start.y); ctx.lineTo(w.End.x, w.End.y); ctx.stroke();
    } else {
      ctx.lineWidth = 4; ctx.beginPath(); ctx.moveTo(b.TrunkStart.x, b.TrunkStart.y); ctx.lineTo(b.TrunkEnd.x, b.TrunkEnd.y); ctx.stroke();
      ctx.lineWidth = 1.5;
      for(const w of b.Wires){
        ctx.beginPath(); ctx.moveTo(w.Start.x, w.Start.y); ctx.lineTo(b.TrunkStart.x, b.TrunkStart.y); ctx.stroke();
        ctx.beginPath(); ctx.moveTo(b.TrunkEnd.x, b.TrunkEnd.y); ctx.lineTo(w.End.x, w.End.y); ctx.stroke();
      }
      ctx.fillStyle = '#fff'; ctx.fillText(b.BadgeLabel, b.BadgePos.x-8, b.BadgePos.y-6);
    }
  }
  for(const n of state.scene.node_graph.nodes){
    ctx.fillStyle = '#212633'; ctx.fillRect(n.x, n.y, n.width, n.height);
    ctx.fillStyle = '#335c85'; ctx.fillRect(n.x, n.y, n.width, 22);
    ctx.fillStyle = '#fff'; ctx.fillText(n.title, n.x+8, n.y+15);
  }
}
setInterval(poll, 120); poll();
</script>
</body>
</html>`))
}
