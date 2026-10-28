package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"osmu/pkg/osmulang"
)

// RPCMessage represents a generic JSON-RPC 2.0 request/notification.
type RPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCResponse represents a JSON-RPC 2.0 response.
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCNotification represents a server-to-client JSON-RPC 2.0 notification.
type RPCNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// LSP types
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"` // 1 = Error, 2 = Warning
	Code     string `json:"code"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

type CompletionItem struct {
	Label      string `json:"label"`
	Kind       int    `json:"kind"` // 6=Variable, 2=Method, 10=Property, 14=Keyword, 24=Event
	Detail     string `json:"detail,omitempty"`
	InsertText string `json:"insertText,omitempty"`
}

// Server implements a Language Server Protocol (LSP) server for .osmu files.
type Server struct {
	mu      sync.Mutex
	docs    map[string]string
	checked map[string]*osmulang.CheckResult
	out     io.Writer
}

// NewServer creates a new .osmu LSP server writing responses to out.
func NewServer(out io.Writer) *Server {
	return &Server{
		docs:    make(map[string]string),
		checked: make(map[string]*osmulang.CheckResult),
		out:     out,
	}
}

// Serve reads JSON-RPC 2.0 framed messages (`Content-Length: ...\r\n\r\n{...}`) from r until EOF.
func (s *Server) Serve(r io.Reader) error {
	br := bufio.NewReader(r)
	for {
		contentLen := 0
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			if strings.HasPrefix(strings.ToLower(line), "content-length:") {
				val := strings.TrimSpace(line[len("content-length:"):])
				contentLen, _ = strconv.Atoi(val)
			}
		}
		if contentLen <= 0 {
			continue
		}
		body := make([]byte, contentLen)
		if _, err := io.ReadFull(br, body); err != nil {
			return err
		}
		if stop := s.HandleRawJSON(body); stop {
			return nil
		}
	}
}

// HandleRawJSON processes a single JSON-RPC 2.0 payload and returns true if `exit` was called.
func (s *Server) HandleRawJSON(payload []byte) bool {
	var msg RPCMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return false
	}

	switch msg.Method {
	case "initialize":
		s.sendResponse(msg.ID, map[string]interface{}{
			"capabilities": map[string]interface{}{
				"textDocumentSync": 1, // Full sync
				"hoverProvider":    true,
				"definitionProvider": true,
				"completionProvider": map[string]interface{}{
					"triggerCharacters": []string{".", " "},
				},
			},
			"serverInfo": map[string]interface{}{
				"name":    "osmu-lsp",
				"version": "1.0.0",
			},
		})

	case "initialized":
		// No response needed

	case "shutdown":
		s.sendResponse(msg.ID, nil)

	case "exit":
		return true

	case "textDocument/didOpen":
		var params struct {
			TextDocument struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(msg.Params, &params) == nil {
			s.updateDocument(params.TextDocument.URI, params.TextDocument.Text)
		}

	case "textDocument/didChange":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if json.Unmarshal(msg.Params, &params) == nil && len(params.ContentChanges) > 0 {
			latest := params.ContentChanges[len(params.ContentChanges)-1].Text
			s.updateDocument(params.TextDocument.URI, latest)
		}

	case "textDocument/hover":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Position Position `json:"position"`
		}
		if json.Unmarshal(msg.Params, &params) == nil {
			s.handleHover(msg.ID, params.TextDocument.URI, params.Position)
		}

	case "textDocument/completion":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Position Position `json:"position"`
		}
		if json.Unmarshal(msg.Params, &params) == nil {
			s.handleCompletion(msg.ID, params.TextDocument.URI)
		}

	case "textDocument/definition":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Position Position `json:"position"`
		}
		if json.Unmarshal(msg.Params, &params) == nil {
			s.handleDefinition(msg.ID, params.TextDocument.URI, params.Position)
		}
	}

	return false
}

func (s *Server) updateDocument(uri, text string) {
	res := osmulang.CompileAndCheck(uri, text)
	s.mu.Lock()
	s.docs[uri] = text
	s.checked[uri] = res
	s.mu.Unlock()

	diags := make([]Diagnostic, 0, len(res.Diagnostics))
	for _, d := range res.Diagnostics {
		line := d.Pos.Line - 1
		if line < 0 {
			line = 0
		}
		col := d.Pos.Column - 1
		if col < 0 {
			col = 0
		}
		endCol := d.EndCol - 1
		if endCol <= col {
			endCol = col + 1
		}
		sev := 1
		if d.Severity == "warning" {
			sev = 2
		}
		diags = append(diags, Diagnostic{
			Range: Range{
				Start: Position{Line: line, Character: col},
				End:   Position{Line: line, Character: endCol},
			},
			Severity: sev,
			Code:     d.Code,
			Source:   "osmu",
			Message:  d.Message,
		})
	}

	s.sendNotification("textDocument/publishDiagnostics", map[string]interface{}{
		"uri":         uri,
		"diagnostics": diags,
	})
}

func (s *Server) handleHover(id json.RawMessage, uri string, pos Position) {
	s.mu.Lock()
	text := s.docs[uri]
	res := s.checked[uri]
	s.mu.Unlock()

	word := wordAtPosition(text, pos.Line, pos.Character)
	if word != "" && res != nil {
		if sym, ok := res.Symbols[word]; ok {
			s.sendResponse(id, map[string]interface{}{
				"contents": map[string]interface{}{
					"kind":  "markdown",
					"value": fmt.Sprintf("```csharp\n%s\n```\n*(%s in .osmu)*", sym.Detail, sym.Kind),
				},
			})
			return
		}
	}
	s.sendResponse(id, nil)
}

func (s *Server) handleDefinition(id json.RawMessage, uri string, pos Position) {
	s.mu.Lock()
	text := s.docs[uri]
	res := s.checked[uri]
	s.mu.Unlock()

	word := wordAtPosition(text, pos.Line, pos.Character)
	if word != "" && res != nil {
		if sym, ok := res.Symbols[word]; ok && sym.DefPos.Line > 0 {
			line := sym.DefPos.Line - 1
			col := sym.DefPos.Column - 1
			s.sendResponse(id, Location{
				URI: uri,
				Range: Range{
					Start: Position{Line: line, Character: col},
					End:   Position{Line: line, Character: col + len(sym.Name)},
				},
			})
			return
		}
	}
	s.sendResponse(id, nil)
}

func (s *Server) handleCompletion(id json.RawMessage, uri string) {
	s.mu.Lock()
	res := s.checked[uri]
	s.mu.Unlock()

	items := []CompletionItem{
		{Label: "component", Kind: 14, Detail: "Declare an OSMU component class", InsertText: "component MyBehavior : Entity {\n\t\n}"},
		{Label: "signal", Kind: 24, Detail: "Declare a typed event signal", InsertText: "signal MySignal(Entity target);"},
		{Label: "emit", Kind: 14, Detail: "Emit a signal event", InsertText: "emit "},
		{Label: "on", Kind: 24, Detail: "Subscribe handler to signal", InsertText: "on MySignal(Entity target) {\n\t\n}"},
		{Label: "query Entities", Kind: 14, Detail: "Declarative entity query", InsertText: "query Entities where Tag == \"Enemy\" orderby DistanceTo(this.Position) asc"},
		{Label: "OnReady", Kind: 2, Detail: "public void OnReady() lifecycle method", InsertText: "public void OnReady() {\n\t\n}"},
		{Label: "OnUpdate", Kind: 2, Detail: "public void OnUpdate(float dt) frame tick", InsertText: "public void OnUpdate(float dt) {\n\t\n}"},
		{Label: "OnCollision", Kind: 2, Detail: "public void OnCollision(Entity other) collision callback", InsertText: "public void OnCollision(Entity other) {\n\t\n}"},
	}

	if res != nil {
		for _, sym := range res.Symbols {
			kind := 6
			switch sym.Kind {
			case "method", "builtin":
				kind = 2
			case "property", "field":
				kind = 10
			case "signal":
				kind = 24
			}
			items = append(items, CompletionItem{
				Label:  sym.Name,
				Kind:   kind,
				Detail: sym.Detail,
			})
		}
	}

	s.sendResponse(id, items)
}

func (s *Server) sendResponse(id json.RawMessage, result interface{}) {
	resp := RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeFramed(resp)
}

func (s *Server) sendNotification(method string, params interface{}) {
	notif := RPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	s.writeFramed(notif)
}

func (s *Server) writeFramed(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n%s", len(data), data)
}

func wordAtPosition(text string, lineIdx, charIdx int) string {
	lines := strings.Split(text, "\n")
	if lineIdx < 0 || lineIdx >= len(lines) {
		return ""
	}
	runes := []rune(lines[lineIdx])
	if charIdx < 0 || charIdx >= len(runes) {
		if charIdx == len(runes) && len(runes) > 0 {
			charIdx = len(runes) - 1
		} else {
			return ""
		}
	}
	isWord := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}
	if !isWord(runes[charIdx]) {
		return ""
	}
	start := charIdx
	for start > 0 && isWord(runes[start-1]) {
		start--
	}
	end := charIdx
	for end < len(runes) && isWord(runes[end]) {
		end++
	}
	return string(runes[start:end])
}
