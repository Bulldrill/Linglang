// cmd/lsp/integration_test.go drives a real, compiled linlang-lsp binary
// over stdin/stdout with actual framed JSON-RPC messages — the same way a
// real editor would — rather than calling server.handle directly, so this
// test also catches framing bugs (Content-Length, header parsing) that an
// in-process call would never exercise.
package main

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func buildLSPBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "linlang-lsp")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building linlang-lsp: %v\n%s", err, out)
	}
	return bin
}

type lspClient struct {
	t   *testing.T
	in  *bufio.Writer
	out *bufio.Reader
}

func (c *lspClient) send(msg map[string]any) {
	c.t.Helper()
	msg["jsonrpc"] = "2.0"
	if err := writeMessage(c.in, msg); err != nil {
		c.t.Fatalf("send: %v", err)
	}
	if err := c.in.Flush(); err != nil {
		c.t.Fatalf("flush: %v", err)
	}
}

func (c *lspClient) recv() *rpcMessage {
	c.t.Helper()
	msg, err := readMessage(c.out)
	if err != nil {
		c.t.Fatalf("recv: %v", err)
	}
	return msg
}

func TestLSPServerFullSession(t *testing.T) {
	bin := buildLSPBinary(t)
	cmd := exec.Command(bin)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting linlang-lsp: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	c := &lspClient{t: t, in: bufio.NewWriter(stdin), out: bufio.NewReader(stdout)}

	// 1. initialize
	c.send(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{}})
	initResp := c.recv()
	var initResult struct {
		Capabilities struct {
			HoverProvider bool `json:"hoverProvider"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(mustMarshal(initResp.Result), &initResult); err != nil {
		t.Fatalf("decoding initialize result: %v", err)
	}
	if !initResult.Capabilities.HoverProvider {
		t.Fatal("expected hoverProvider: true in initialize capabilities")
	}

	c.send(map[string]any{"method": "initialized", "params": map[string]any{}})

	// 2. didOpen a document with one real error on line 2 (0-indexed)
	docText := "space Tarea: id: Real, prioridad: Real\nlet t1 = Tarea[1, 3]\nlet bad = Nope[1, 2]\n"
	c.send(map[string]any{
		"method": "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///test.lin", "text": docText},
		},
	})
	diagMsg := c.recv()
	if diagMsg.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics notification, got method=%q", diagMsg.Method)
	}
	diagParams := decodePublishParams(t, diagMsg)
	if len(diagParams.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diagParams.Diagnostics), diagParams.Diagnostics)
	}
	if diagParams.Diagnostics[0].Range.Start.Line != 2 {
		t.Fatalf("expected the diagnostic on line 2, got line %d", diagParams.Diagnostics[0].Range.Start.Line)
	}

	// 3. hover over "Tarea" on line 0
	c.send(map[string]any{
		"id":     2,
		"method": "textDocument/hover",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///test.lin"},
			"position":     map[string]any{"line": 0, "character": 7},
		},
	})
	hoverResp := c.recv()
	var hoverResult struct {
		Contents struct {
			Value string `json:"value"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(mustMarshal(hoverResp.Result), &hoverResult); err != nil {
		t.Fatalf("decoding hover result: %v", err)
	}
	if hoverResult.Contents.Value == "" {
		t.Fatal("expected non-empty hover content for 'Tarea'")
	}

	// 4. didChange fixing the error — diagnostics must clear to empty, not stay stale.
	fixedText := "space Tarea: id: Real, prioridad: Real\nlet t1 = Tarea[1, 3]\n"
	c.send(map[string]any{
		"method": "textDocument/didChange",
		"params": map[string]any{
			"textDocument":   map[string]any{"uri": "file:///test.lin"},
			"contentChanges": []map[string]any{{"text": fixedText}},
		},
	})
	diagMsg2 := c.recv()
	diagParams2 := decodePublishParams(t, diagMsg2)
	if len(diagParams2.Diagnostics) != 0 {
		t.Fatalf("expected diagnostics to clear after fixing the document, got %+v", diagParams2.Diagnostics)
	}

	// 5. shutdown + exit
	c.send(map[string]any{"id": 3, "method": "shutdown", "params": map[string]any{}})
	c.recv()
	c.send(map[string]any{"method": "exit", "params": map[string]any{}})

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("expected clean exit, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not exit within 3s of 'exit' notification")
	}
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// decodePublishParams re-reads a publishDiagnostics notification's params,
// since rpcMessage decodes notifications' payload into Params, not Result.
func decodePublishParams(t *testing.T, msg *rpcMessage) publishDiagnosticsParams {
	t.Helper()
	// msg was decoded once already by readMessage; re-marshal+remarshal via
	// the raw Params field captured at decode time.
	var p publishDiagnosticsParams
	if msg.Params != nil {
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			t.Fatalf("decoding publishDiagnostics params: %v", err)
		}
	}
	return p
}
