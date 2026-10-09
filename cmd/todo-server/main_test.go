package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"linlang-go/parser"
	"linlang-go/store"
)

// testSchema mirrors examples/todo.lin inline so tests don't depend on the
// working directory (go test's CWD is the package directory, not the repo
// root, so a relative "examples/todo.lin" path would not resolve).
const testSchema = `
space Tarea: id: Real, prioridad: Real, estado: Real, categoria: Real

transform completar: Tarea x Tarea -> Tarea {
    id        = p.id
    prioridad = p.prioridad
    estado    = 2
    categoria = p.categoria
}
transform iniciar: Tarea x Tarea -> Tarea {
    id        = p.id
    prioridad = p.prioridad
    estado    = 1
    categoria = p.categoria
}
transform reabrir: Tarea x Tarea -> Tarea {
    id        = p.id
    prioridad = p.prioridad
    estado    = 0
    categoria = p.categoria
}
transform priorizar: Tarea x Tarea -> Tarea {
    id        = p.id
    prioridad = m.prioridad
    estado    = p.estado
    categoria = p.categoria
}
transform recategorizar: Tarea x Tarea -> Tarea {
    id        = p.id
    prioridad = p.prioridad
    estado    = p.estado
    categoria = m.categoria
}
`

func newTestServer(t *testing.T) *Server {
	t.Helper()
	schemaRT := parser.NewRuntime()
	schemaRT.Parse(testSchema)
	if schemaRT.Spaces["Tarea"] == nil {
		t.Fatal("test schema failed to load: no 'Tarea' space")
	}
	return &Server{
		schemaRT: schemaRT,
		db:       store.NewMemoryStore(),
		titles:   make(map[int64]string),
		nextID:   1,
	}
}

func doRequest(s *Server, method, path string, body any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("invalid JSON response (status=%d): %v\nbody=%s", rec.Code, err, rec.Body.String())
	}
}

func TestCreateAndGetTask(t *testing.T) {
	s := newTestServer(t)

	rec := doRequest(s, "POST", "/tasks", map[string]any{"title": "comprar pan", "priority": 3, "category": 1})
	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	decodeJSON(t, rec, &created)
	if created["title"] != "comprar pan" {
		t.Fatalf("expected title 'comprar pan', got %v", created["title"])
	}
	id := int64(created["id"].(float64))

	rec = doRequest(s, "GET", "/tasks/"+strconv.FormatInt(id, 10), nil)
	if rec.Code != 200 {
		t.Fatalf("expected 200 fetching the created task, got %d: %s", rec.Code, rec.Body.String())
	}
	var fetched map[string]any
	decodeJSON(t, rec, &fetched)
	if fetched["title"] != "comprar pan" {
		t.Fatalf("expected fetched title 'comprar pan', got %v", fetched["title"])
	}
}

func TestCreateTaskRejectsMissingTitle(t *testing.T) {
	s := newTestServer(t)
	rec := doRequest(s, "POST", "/tasks", map[string]any{"priority": 2})
	if rec.Code != 400 {
		t.Fatalf("expected 400 for a missing title, got %d", rec.Code)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	s := newTestServer(t)
	rec := doRequest(s, "GET", "/tasks/999", nil)
	if rec.Code != 404 {
		t.Fatalf("expected 404 for a nonexistent task, got %d", rec.Code)
	}
}

func TestListTasksFiltersByQueryParams(t *testing.T) {
	s := newTestServer(t)
	doRequest(s, "POST", "/tasks", map[string]any{"title": "a", "priority": 3, "category": 1})
	doRequest(s, "POST", "/tasks", map[string]any{"title": "b", "priority": 1, "category": 2})
	doRequest(s, "POST", "/tasks", map[string]any{"title": "c", "priority": 3, "category": 2})

	rec := doRequest(s, "GET", "/tasks?priority=3", nil)
	var tasks []map[string]any
	decodeJSON(t, rec, &tasks)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks with priority=3, got %d: %v", len(tasks), tasks)
	}

	rec = doRequest(s, "GET", "/tasks?priority=3&category=2", nil)
	decodeJSON(t, rec, &tasks)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task with priority=3 AND category=2, got %d: %v", len(tasks), tasks)
	}
}

func TestTaskLifecycleTransitions(t *testing.T) {
	s := newTestServer(t)
	rec := doRequest(s, "POST", "/tasks", map[string]any{"title": "tarea", "priority": 2, "category": 1})
	var created map[string]any
	decodeJSON(t, rec, &created)
	id := int64(created["id"].(float64))
	path := func(suffix string) string { return "/tasks/" + strconv.FormatInt(id, 10) + "/" + suffix }

	rec = doRequest(s, "PUT", path("start"), nil)
	var started map[string]any
	decodeJSON(t, rec, &started)
	if started["status"] != float64(StatusInProgress) {
		t.Fatalf("expected status=InProgress after start, got %v", started["status"])
	}

	rec = doRequest(s, "PUT", path("complete"), nil)
	var completed map[string]any
	decodeJSON(t, rec, &completed)
	if completed["status"] != float64(StatusDone) {
		t.Fatalf("expected status=Done after complete, got %v", completed["status"])
	}

	rec = doRequest(s, "PUT", path("reopen"), nil)
	var reopened map[string]any
	decodeJSON(t, rec, &reopened)
	if reopened["status"] != float64(StatusPending) {
		t.Fatalf("expected status=Pending after reopen, got %v", reopened["status"])
	}
}

func TestUpdatePriorityAndCategory(t *testing.T) {
	s := newTestServer(t)
	rec := doRequest(s, "POST", "/tasks", map[string]any{"title": "tarea", "priority": 1, "category": 1})
	var created map[string]any
	decodeJSON(t, rec, &created)
	id := int64(created["id"].(float64))

	rec = doRequest(s, "PUT", "/tasks/"+strconv.FormatInt(id, 10)+"/priority", map[string]any{"priority": 3})
	var updated map[string]any
	decodeJSON(t, rec, &updated)
	if updated["priority"] != float64(3) {
		t.Fatalf("expected priority=3, got %v", updated["priority"])
	}

	rec = doRequest(s, "PUT", "/tasks/"+strconv.FormatInt(id, 10)+"/category", map[string]any{"category": 3})
	decodeJSON(t, rec, &updated)
	if updated["category"] != float64(3) {
		t.Fatalf("expected category=3, got %v", updated["category"])
	}
}

func TestDeleteTaskRemovesIt(t *testing.T) {
	s := newTestServer(t)
	rec := doRequest(s, "POST", "/tasks", map[string]any{"title": "borrar", "priority": 1, "category": 1})
	var created map[string]any
	decodeJSON(t, rec, &created)
	id := int64(created["id"].(float64))

	rec = doRequest(s, "DELETE", "/tasks/"+strconv.FormatInt(id, 10), nil)
	if rec.Code != 204 {
		t.Fatalf("expected 204 on delete, got %d", rec.Code)
	}
	rec = doRequest(s, "GET", "/tasks/"+strconv.FormatInt(id, 10), nil)
	if rec.Code != 404 {
		t.Fatalf("expected 404 after delete, got %d", rec.Code)
	}
}

func TestUrgentTasksExcludesCompleted(t *testing.T) {
	s := newTestServer(t)
	rec := doRequest(s, "POST", "/tasks", map[string]any{"title": "urgente1", "priority": 3, "category": 1})
	var t1 map[string]any
	decodeJSON(t, rec, &t1)
	id1 := int64(t1["id"].(float64))

	doRequest(s, "POST", "/tasks", map[string]any{"title": "urgente2", "priority": 3, "category": 1})
	doRequest(s, "POST", "/tasks", map[string]any{"title": "no-urgente", "priority": 1, "category": 1})

	doRequest(s, "PUT", "/tasks/"+strconv.FormatInt(id1, 10)+"/complete", nil)

	rec = doRequest(s, "GET", "/tasks/urgent", nil)
	var urgent []map[string]any
	decodeJSON(t, rec, &urgent)
	if len(urgent) != 1 {
		t.Fatalf("expected 1 urgent (high-priority, not completed) task, got %d: %v", len(urgent), urgent)
	}
}

func TestStatsReportsCentroidAndCounts(t *testing.T) {
	s := newTestServer(t)
	doRequest(s, "POST", "/tasks", map[string]any{"title": "a", "priority": 3, "category": 1})
	doRequest(s, "POST", "/tasks", map[string]any{"title": "b", "priority": 1, "category": 2})

	rec := doRequest(s, "GET", "/tasks/stats", nil)
	var stats map[string]any
	decodeJSON(t, rec, &stats)
	if stats["total"] != float64(2) {
		t.Fatalf("expected total=2, got %v", stats["total"])
	}
	if stats["pendientes"] != float64(2) {
		t.Fatalf("expected 2 pending tasks, got %v", stats["pendientes"])
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	s := newTestServer(t)
	rec := doRequest(s, "GET", "/nope", nil)
	if rec.Code != 404 {
		t.Fatalf("expected 404 for an unknown route, got %d", rec.Code)
	}
}
