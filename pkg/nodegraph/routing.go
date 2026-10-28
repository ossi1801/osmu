package nodegraph

import (
	"fmt"
	"math"
	"time"

	"osmu/pkg/engine"
)

// RoutedWire represents a single resolved connection with its start/end socket coordinates.
type RoutedWire struct {
	Link   *Link
	Start  engine.Vec2
	End    engine.Vec2
	Kind   SocketKind
	Active bool
}

// WireBundle represents an automatically grouped set of node connections that share a clean highway trunk.
type WireBundle struct {
	ID         string
	Wires      []RoutedWire
	IsBundled  bool
	TrunkStart engine.Vec2
	TrunkEnd   engine.Vec2
	BadgePos   engine.Vec2
	BadgeLabel string
	Kind       SocketKind
	Active     bool
}

// ComputeWireBundles analyzes all links in the graph and automatically groups nearby/parallel
// connections into clean highway trunks with fan-in and fan-out branches.
func (g *Graph) ComputeWireBundles() []WireBundle {
	now := time.Now()
	resolved := make([]RoutedWire, 0, len(g.Links))
	for _, l := range g.Links {
		src := g.FindNode(l.FromNode)
		dst := g.FindNode(l.ToNode)
		if src == nil || dst == nil {
			continue
		}
		sx, sy := src.SocketPos(l.FromSocket, true)
		dx, dy := dst.SocketPos(l.ToSocket, false)
		active := now.Sub(l.LastFired) < 450*time.Millisecond
		resolved = append(resolved, RoutedWire{
			Link:   l,
			Start:  engine.V2(sx, sy),
			End:    engine.V2(dx, dy),
			Kind:   l.Kind,
			Active: active,
		})
	}

	if !g.AutoGroup || len(resolved) <= 1 {
		out := make([]WireBundle, 0, len(resolved))
		for i, rw := range resolved {
			out = append(out, WireBundle{
				ID:        fmt.Sprintf("single_%d", i),
				Wires:     []RoutedWire{rw},
				IsBundled: false,
				Kind:      rw.Kind,
				Active:    rw.Active,
			})
		}
		return out
	}

	thresh := g.GroupThresh
	if thresh <= 10 {
		thresh = 110.0
	}

	used := make([]bool, len(resolved))
	var bundles []WireBundle

	for i := 0; i < len(resolved); i++ {
		if used[i] {
			continue
		}
		used[i] = true
		seed := resolved[i]
		group := []RoutedWire{seed}

		for j := i + 1; j < len(resolved); j++ {
			if used[j] {
				continue
			}
			cand := resolved[j]
			if shouldGroupWires(seed, cand, thresh) {
				used[j] = true
				group = append(group, cand)
			}
		}

		if len(group) == 1 {
			bundles = append(bundles, WireBundle{
				ID:        fmt.Sprintf("bundle_%d", len(bundles)+1),
				Wires:     group,
				IsBundled: false,
				Kind:      seed.Kind,
				Active:    seed.Active,
			})
			continue
		}

		// Compute average start and end centroids for the grouped wires
		var avgStart, avgEnd engine.Vec2
		anyActive := false
		for _, w := range group {
			avgStart = avgStart.Add(w.Start)
			avgEnd = avgEnd.Add(w.End)
			if w.Active {
				anyActive = true
			}
		}
		n := float64(len(group))
		avgStart = avgStart.Scale(1.0 / n)
		avgEnd = avgEnd.Scale(1.0 / n)

		// Trunk spans the middle 52% of the horizontal corridor between average start and average end
		span := avgEnd.Sub(avgStart)
		trunkStart := avgStart.Add(engine.V2(span.X*0.24, span.Y*0.35))
		trunkEnd := avgStart.Add(engine.V2(span.X*0.76, span.Y*0.65))
		// Keep trunk horizontal when nodes are primarily left-to-right for extra visual cleanliness
		if math.Abs(span.X) > 100 {
			midY := (avgStart.Y + avgEnd.Y) * 0.5
			trunkStart.Y = midY
			trunkEnd.Y = midY
		}
		badgePos := trunkStart.Lerp(trunkEnd, 0.5)

		bundles = append(bundles, WireBundle{
			ID:         fmt.Sprintf("bundle_%d", len(bundles)+1),
			Wires:      group,
			IsBundled:  true,
			TrunkStart: trunkStart,
			TrunkEnd:   trunkEnd,
			BadgePos:   badgePos,
			BadgeLabel: fmt.Sprintf("x%d", len(group)),
			Kind:       seed.Kind,
			Active:     anyActive,
		})
	}

	return bundles
}

func shouldGroupWires(a, b RoutedWire, thresh float64) bool {
	// Same direction (left-to-right or right-to-left)
	dirA := a.End.X - a.Start.X
	dirB := b.End.X - b.Start.X
	if dirA*dirB < 0 {
		return false
	}

	// Case 1: Shared source node or very close output sockets fanning out to nearby targets
	if a.Link.FromNode == b.Link.FromNode && a.End.Dist(b.End) <= thresh*2.4 {
		return true
	}
	// Case 2: Shared destination node receiving multiple inputs from nearby sources
	if a.Link.ToNode == b.Link.ToNode && a.Start.Dist(b.Start) <= thresh*2.4 {
		return true
	}
	// Case 3: Parallel corridor wires whose starts and ends are both within threshold
	if a.Start.Dist(b.Start) <= thresh*1.5 && a.End.Dist(b.End) <= thresh*1.5 {
		return true
	}
	// Case 4: Midpoints are close and slopes are similar
	midA := a.Start.Lerp(a.End, 0.5)
	midB := b.Start.Lerp(b.End, 0.5)
	if midA.Dist(midB) <= thresh*0.95 {
		return true
	}
	return false
}

// SampleCubicBezier samples a smooth cubic Bezier curve between p0 and p3 into N polyline points.
func SampleCubicBezier(p0, p3 engine.Vec2, steps int) []engine.Vec2 {
	if steps < 4 {
		steps = 12
	}
	dx := math.Max(32.0, math.Abs(p3.X-p0.X)*0.45)
	p1 := engine.V2(p0.X+dx, p0.Y)
	p2 := engine.V2(p3.X-dx, p3.Y)

	pts := make([]engine.Vec2, steps+1)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		u := 1.0 - t
		tt := t * t
		uu := u * u
		uuu := uu * u
		ttt := tt * t

		pt := p0.Scale(uuu).
			Add(p1.Scale(3 * uu * t)).
			Add(p2.Scale(3 * u * tt)).
			Add(p3.Scale(ttt))
		pts[i] = pt
	}
	return pts
}
