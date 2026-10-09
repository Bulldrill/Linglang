package parser

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTempLin(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestIncludeLoadsDeclarationsIntoSameRuntime(t *testing.T) {
	dir := t.TempDir()
	schema := writeTempLin(t, dir, "schema.lin", "space Tarea: id: Real, prioridad: Real\n")

	rt := NewRuntime()
	rt.Parse(`include "` + schema + `"
let t1 = Tarea[1, 3]
`)
	if rt.Spaces["Tarea"] == nil {
		t.Fatal("expected 'Tarea' space to be declared via include")
	}
	if rt.Vectors["t1"] == nil {
		t.Fatal("expected t1 to be constructible using the included space")
	}
}

func TestIncludeTwiceIsANoOp(t *testing.T) {
	dir := t.TempDir()
	// A space declared twice in the same Runtime would just overwrite
	// itself harmlessly, so make the no-op observable a different way:
	// count how many times the file's [✔️] side effect fires by checking
	// rt.included only has one entry after including the same path twice.
	schema := writeTempLin(t, dir, "schema.lin", "space Tarea: id: Real\n")

	rt := NewRuntime()
	rt.Parse(`include "` + schema + `"`)
	rt.Parse(`include "` + schema + `"`)

	if len(rt.included) != 1 {
		t.Fatalf("expected exactly 1 tracked include after including the same path twice, got %d", len(rt.included))
	}
}

func TestIncludeCycleTerminates(t *testing.T) {
	dir := t.TempDir()
	pathB := filepath.Join(dir, "b.lin")
	pathA := writeTempLin(t, dir, "a.lin", `include "`+pathB+`"
space A: x: Real
`)
	writeTempLin(t, dir, "b.lin", `include "`+pathA+`"
space B: y: Real
`)

	done := make(chan struct{})
	rt := NewRuntime()
	go func() {
		rt.Parse(`include "` + pathA + `"`)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("include cycle did not terminate within 2s — likely infinite recursion")
	}

	if rt.Spaces["A"] == nil || rt.Spaces["B"] == nil {
		t.Fatal("expected both A and B to be declared despite the cycle")
	}
}

func TestIncludeMissingFileReportsCatchableError(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`include "/nonexistent/path/does-not-exist.lin"`)
	if rt.LastError == nil || rt.LastError.Name != "ImportError" {
		t.Fatalf("expected a catchable ImportError, got %+v", rt.LastError)
	}
}

func TestIncludeMissingFileIsCatchable(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
try {
    include "/nonexistent/path/does-not-exist.lin"
}
catch ImportError {
    space Fallback: id: Real
}
`)
	if rt.Spaces["Fallback"] == nil {
		t.Fatal("expected the catch body to run for a missing include")
	}
}
