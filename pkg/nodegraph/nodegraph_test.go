package nodegraph_test

import (
	"testing"

	"osmu/pkg/nodegraph"
)

func TestAutomaticWireGrouping(t *testing.T) {
	g := nodegraph.NewGraph()
	n1 := nodegraph.CreateNodeFromTemplate("n1", "QueryEntities", 40, 50)
	n2 := nodegraph.CreateNodeFromTemplate("n2", "MoveTowards", 420, 55)

	g.Nodes = []*nodegraph.Node{n1, n2}
	g.AddLink("n1", "Found", "n2", "In", nodegraph.SocketFlow)
	g.AddLink("n1", "Entities", "n2", "Actors", nodegraph.SocketEntityList)
	g.AddLink("n1", "Nearest", "n2", "Goal", nodegraph.SocketEntity)

	bundles := g.ComputeWireBundles()
	if len(bundles) != 1 {
		t.Fatalf("expected 3 parallel links between n1 and n2 to be grouped into 1 bundle, got %d", len(bundles))
	}
	if !bundles[0].IsBundled || bundles[0].BadgeLabel != "x3" {
		t.Fatalf("expected bundle with BadgeLabel 'x3', got IsBundled=%v label=%q", bundles[0].IsBundled, bundles[0].BadgeLabel)
	}
}
