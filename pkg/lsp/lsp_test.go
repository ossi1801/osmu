package lsp_test

import (
	"bytes"
	"strings"
	"testing"

	"osmu/pkg/lsp"
)

func TestLSPDiagnosticsHoverAndCompletion(t *testing.T) {
	var out bytes.Buffer
	srv := lsp.NewServer(&out)

	// 1. Initialize
	srv.HandleRawJSON([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if !strings.Contains(out.String(), `"name":"osmu-lsp"`) {
		t.Fatalf("expected osmu-lsp initialize response, got: %s", out.String())
	}
	out.Reset()

	// 2. didOpen with a valid field and a type error
	srv.HandleRawJSON([]byte(`{
		"jsonrpc":"2.0",
		"method":"textDocument/didOpen",
		"params":{
			"textDocument":{
				"uri":"file:///test.osmu",
				"text":"namespace Demo;\ncomponent Player : Entity {\n  [Export] public float Boost = 10.0f;\n  public int Bad = \"str\";\n}\n"
			}
		}
	}`))
	if !strings.Contains(out.String(), `"method":"textDocument/publishDiagnostics"`) || !strings.Contains(out.String(), "OSMU206") {
		t.Fatalf("expected publishDiagnostics with OSMU206, got: %s", out.String())
	}
	out.Reset()

	// 3. Hover over `Boost` on line 2, character 25
	srv.HandleRawJSON([]byte(`{
		"jsonrpc":"2.0",
		"id":2,
		"method":"textDocument/hover",
		"params":{
			"textDocument":{"uri":"file:///test.osmu"},
			"position":{"line":2,"character":25}
		}
	}`))
	if !strings.Contains(out.String(), "[Export] public float Player.Boost") {
		t.Fatalf("expected hover info for Boost field, got: %s", out.String())
	}
	out.Reset()

	// 4. Completion
	srv.HandleRawJSON([]byte(`{
		"jsonrpc":"2.0",
		"id":3,
		"method":"textDocument/completion",
		"params":{
			"textDocument":{"uri":"file:///test.osmu"},
			"position":{"line":2,"character":5}
		}
	}`))
	if !strings.Contains(out.String(), `"query Entities"`) || !strings.Contains(out.String(), `"Boost"`) {
		t.Fatalf("expected completion items, got: %s", out.String())
	}
}
