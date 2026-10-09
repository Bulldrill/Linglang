// cmd/lsp/main.go — LinLang Language Server (issue #31).
//
// Speaks LSP over stdio, as every editor integration (VS Code, Neovim,
// ...) expects. Supported methods: initialize, initialized,
// textDocument/didOpen, textDocument/didChange, textDocument/didClose,
// textDocument/hover, shutdown, exit.
//
// Run:  go run ./cmd/lsp/
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// server holds every open document's current text, keyed by URI — the
// minimal state an editor integration needs (diagnostics and hover both
// re-run the document through parser.Runtime on demand rather than
// caching any analysis result).
type server struct {
	mu   sync.Mutex
	docs map[string]string
}

func newServer() *server {
	return &server{docs: map[string]string{}}
}

func main() {
	srv := newServer()
	reader := bufio.NewReader(os.Stdin)

	for {
		msg, err := readMessage(reader)
		if err != nil {
			return // stdin closed: client disconnected
		}
		srv.handle(msg)
		if msg.Method == "exit" {
			return
		}
	}
}

func (s *server) handle(msg *rpcMessage) {
	switch msg.Method {
	case "initialize":
		s.reply(msg.ID, map[string]any{
			"capabilities": map[string]any{
				"textDocumentSync": 1, // 1 = Full: client sends the whole document on change
				"hoverProvider":    true,
			},
			"serverInfo": map[string]any{"name": "linlang-lsp", "version": "0.1.0"},
		})

	case "initialized":
		// notification, no reply

	case "shutdown":
		s.reply(msg.ID, nil)

	case "exit":
		// handled in main's loop

	case "textDocument/didOpen":
		var p didOpenParams
		if json.Unmarshal(msg.Params, &p) == nil {
			s.setDoc(p.TextDocument.URI, p.TextDocument.Text)
			s.publishDiagnostics(p.TextDocument.URI)
		}

	case "textDocument/didChange":
		var p didChangeParams
		if json.Unmarshal(msg.Params, &p) == nil && len(p.ContentChanges) > 0 {
			// textDocumentSync=Full (see initialize above): the last
			// change event carries the complete new document text.
			text := p.ContentChanges[len(p.ContentChanges)-1].Text
			s.setDoc(p.TextDocument.URI, text)
			s.publishDiagnostics(p.TextDocument.URI)
		}

	case "textDocument/didClose":
		var p didOpenParams
		if json.Unmarshal(msg.Params, &p) == nil {
			s.deleteDoc(p.TextDocument.URI)
		}

	case "textDocument/hover":
		var p hoverParams
		if json.Unmarshal(msg.Params, &p) != nil {
			s.reply(msg.ID, nil)
			return
		}
		text := s.getDoc(p.TextDocument.URI)
		info := hoverInfo(text, p.Position.Line, p.Position.Character)
		if info == "" {
			s.reply(msg.ID, nil)
			return
		}
		s.reply(msg.ID, map[string]any{
			"contents": map[string]any{"kind": "plaintext", "value": info},
		})

	default:
		if msg.ID != nil {
			s.reply(msg.ID, nil) // unknown request: respond empty rather than hang the client
		}
	}
}

func (s *server) setDoc(uri, text string) {
	s.mu.Lock()
	s.docs[uri] = text
	s.mu.Unlock()
}

func (s *server) getDoc(uri string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.docs[uri]
}

func (s *server) deleteDoc(uri string) {
	s.mu.Lock()
	delete(s.docs, uri)
	s.mu.Unlock()
}

func (s *server) publishDiagnostics(uri string) {
	diags := analyzeDocument(s.getDoc(uri))
	if diags == nil {
		diags = []diagnostic{} // publish an empty array, not JSON null, to actually clear old diagnostics
	}
	writeNotification(os.Stdout, "textDocument/publishDiagnostics", publishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	})
}

func (s *server) reply(id json.RawMessage, result any) {
	if err := writeResult(os.Stdout, id, result); err != nil {
		fmt.Fprintln(os.Stderr, "lsp: error escribiendo respuesta:", err)
	}
}
