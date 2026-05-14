// cmd/todo-server/main.go
//
// Backend TODO 100% LinLang-driven con persistencia transparente.
//
// Cada endpoint HTTP genera un fragmento de código LinLang y lo ejecuta
// en un Runtime fresco.  Toda la lógica de negocio vive en todo.lin;
// el servidor es un thin-shell HTTP.
//
// Variables de entorno:
//
//	LINLANG_DB   DSN del backend de persistencia (default: memory://)
//	             Ejemplos:
//	               sqlite://linlang.db
//	               redis://localhost:6379/0
//	PORT         Puerto HTTP (default: 8080)
//
// Uso:
//
//	go run ./cmd/todo-server/ [ruta/a/todo.lin]
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"linlang-go/core"
	"linlang-go/parser"
	"linlang-go/store"
)

// ── Constantes del dominio ────────────────────────────────────────────────────

const (
	StatusPending    = 0
	StatusInProgress = 1
	StatusDone       = 2
	PriorityLow      = 1
	PriorityMedium   = 2
	PriorityHigh     = 3
	CategoryWork     = 1
	CategoryPersonal = 2
	CategoryUrgent   = 3
)

var statusNames = map[int]string{0: "pendiente", 1: "en_progreso", 2: "completada"}
var priorityNames = map[int]string{1: "baja", 2: "media", 3: "alta"}
var categoryNames = map[int]string{1: "trabajo", 2: "personal", 3: "urgente"}

// ── Server ────────────────────────────────────────────────────────────────────

// Server mantiene:
//   - schemaRT: Runtime base con spaces y transforms cargados desde todo.lin
//   - db: backend de persistencia (SQLite / Redis / Memory)
//   - titles: mapa id→título (strings viven fuera de LinLang)
type Server struct {
	schemaRT *parser.Runtime
	db       store.Backend
	titles   map[int64]string
	nextID   int64
	mu       sync.RWMutex
}

// newExecRT devuelve un Runtime fresco por cada request, con los spaces
// y transforms del schema precargado y el store inyectado.
func (s *Server) newExecRT() *parser.Runtime {
	rt := parser.NewRuntime()
	rt.SetStore(s.db)
	for k, v := range s.schemaRT.Spaces {
		rt.Spaces[k] = v
	}
	for k, v := range s.schemaRT.Transforms {
		rt.Transforms[k] = v
	}
	return rt
}

// vecToJSON convierte un vector LinLang + título en el JSON de respuesta.
func (s *Server) vecToJSON(v *core.Vector) map[string]any {
	s.mu.RLock()
	title := s.titles[int64(v.Values[0])]
	s.mu.RUnlock()
	pri := int(v.Values[1])
	sta := int(v.Values[2])
	cat := int(v.Values[3])
	return map[string]any{
		"id":            int64(v.Values[0]),
		"title":         title,
		"priority":      pri,
		"priority_name": priorityNames[pri],
		"status":        sta,
		"status_name":   statusNames[sta],
		"category":      cat,
		"category_name": categoryNames[cat],
		"vector_linlang": v.Values,
	}
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// POST /tasks
// Body: {"title":"…","priority":2,"category":1}
func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title    string `json:"title"`
		Priority int    `json:"priority"`
		Category int    `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Title) == "" {
		errJSON(w, 400, "se requiere 'title'")
		return
	}
	if body.Priority < 1 || body.Priority > 3 {
		body.Priority = PriorityMedium
	}
	if body.Category < 1 || body.Category > 3 {
		body.Category = CategoryWork
	}

	id := atomic.AddInt64(&s.nextID, 1) - 1

	// Generar y ejecutar LinLang:
	//   let t = Tarea[id, priority, 0, category]
	//   persist t
	code := fmt.Sprintf(
		"let t = Tarea[%.0f, %d, %d, %d]\npersist t",
		float64(id), body.Priority, StatusPending, body.Category,
	)
	rt := s.newExecRT()
	rt.Parse(code)

	s.mu.Lock()
	s.titles[id] = body.Title
	s.mu.Unlock()

	if v := rt.Vectors["t"]; v != nil {
		writeJSON(w, 201, s.vecToJSON(v))
	} else {
		errJSON(w, 500, "no se pudo crear el vector")
	}
}

// GET /tasks[?status=&priority=&category=]
func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	conds := buildWhereClauses(q.Get("status"), q.Get("priority"), q.Get("category"))

	// Generar LinLang:  let all = query Tarea [where dim OP val [and …]]
	// Nota: LinLang soporta una sola condición por query; para múltiples
	// filtros ejecutamos queries sucesivas y hacemos intersección en Go.
	rt := s.newExecRT()
	if len(conds) == 0 {
		rt.Parse("let all = query Tarea")
	} else {
		// Aplicar primer filtro via LinLang
		rt.Parse(fmt.Sprintf("let all = query Tarea where %s", conds[0]))
		// Filtros adicionales reducen la colección en Go
		for _, c := range conds[1:] {
			rt.Collections["all"] = filterCollection(rt.Collections["all"], c)
		}
	}

	vecs := rt.Collections["all"]
	sort.Slice(vecs, func(i, j int) bool { return vecs[i].Values[0] < vecs[j].Values[0] })

	result := make([]map[string]any, len(vecs))
	for i, v := range vecs {
		result[i] = s.vecToJSON(v)
	}
	writeJSON(w, 200, result)
}

// GET /tasks/{id}
func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r.URL.Path)
	if err != nil {
		errJSON(w, 400, "ID inválido")
		return
	}
	// LinLang: let t = query_one Tarea where id == <id>
	rt := s.newExecRT()
	rt.Parse(fmt.Sprintf("let t = query_one Tarea where id == %.0f", float64(id)))
	if v := rt.Vectors["t"]; v != nil {
		writeJSON(w, 200, s.vecToJSON(v))
	} else {
		errJSON(w, 404, "tarea no encontrada")
	}
}

// stateHandler aplica el transform txName y persiste el resultado.
func (s *Server) stateHandler(txName, suffix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idFromPath(strings.TrimSuffix(r.URL.Path, "/"+suffix))
		if err != nil {
			errJSON(w, 400, "ID inválido")
			return
		}
		// LinLang:
		//   let t     = query_one Tarea where id == <id>
		//   let t_new = txName(t, t)
		//   persist t_new
		code := fmt.Sprintf(
			"let t = query_one Tarea where id == %.0f\nlet t_new = %s(t, t)\npersist t_new",
			float64(id), txName,
		)
		rt := s.newExecRT()
		rt.Parse(code)
		if v := rt.Vectors["t_new"]; v != nil {
			writeJSON(w, 200, s.vecToJSON(v))
		} else {
			errJSON(w, 404, "tarea no encontrada")
		}
	}
}

// PUT /tasks/{id}/priority  {"priority": 3}
func (s *Server) updatePriority(w http.ResponseWriter, r *http.Request) {
	var body struct{ Priority int `json:"priority"` }
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Priority < 1 || body.Priority > 3 {
		errJSON(w, 400, "priority: 1=baja, 2=media, 3=alta")
		return
	}
	id, err := idFromPath(strings.TrimSuffix(r.URL.Path, "/priority"))
	if err != nil {
		errJSON(w, 400, "ID inválido")
		return
	}
	// LinLang:
	//   let t     = query_one Tarea where id == <id>
	//   let delta = Tarea[0, <priority>, 0, 0]
	//   let t_new = priorizar(t, delta)
	//   persist t_new
	code := fmt.Sprintf(
		"let t = query_one Tarea where id == %.0f\nlet delta = Tarea[0, %d, 0, 0]\nlet t_new = priorizar(t, delta)\npersist t_new",
		float64(id), body.Priority,
	)
	rt := s.newExecRT()
	rt.Parse(code)
	if v := rt.Vectors["t_new"]; v != nil {
		writeJSON(w, 200, s.vecToJSON(v))
	} else {
		errJSON(w, 404, "tarea no encontrada")
	}
}

// PUT /tasks/{id}/category  {"category": 3}
func (s *Server) updateCategory(w http.ResponseWriter, r *http.Request) {
	var body struct{ Category int `json:"category"` }
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Category < 1 || body.Category > 3 {
		errJSON(w, 400, "category: 1=trabajo, 2=personal, 3=urgente")
		return
	}
	id, err := idFromPath(strings.TrimSuffix(r.URL.Path, "/category"))
	if err != nil {
		errJSON(w, 400, "ID inválido")
		return
	}
	code := fmt.Sprintf(
		"let t = query_one Tarea where id == %.0f\nlet delta = Tarea[0, 0, 0, %d]\nlet t_new = recategorizar(t, delta)\npersist t_new",
		float64(id), body.Category,
	)
	rt := s.newExecRT()
	rt.Parse(code)
	if v := rt.Vectors["t_new"]; v != nil {
		writeJSON(w, 200, s.vecToJSON(v))
	} else {
		errJSON(w, 404, "tarea no encontrada")
	}
}

// DELETE /tasks/{id}
func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r.URL.Path)
	if err != nil {
		errJSON(w, 400, "ID inválido")
		return
	}
	// LinLang:
	//   let t = query_one Tarea where id == <id>
	//   drop t
	code := fmt.Sprintf(
		"let t = query_one Tarea where id == %.0f\ndrop t",
		float64(id),
	)
	rt := s.newExecRT()
	rt.Parse(code)

	s.mu.Lock()
	delete(s.titles, id)
	s.mu.Unlock()

	w.WriteHeader(204)
}

// GET /tasks/urgent  — prioridad alta y no completadas
func (s *Server) urgentTasks(w http.ResponseWriter, r *http.Request) {
	rt := s.newExecRT()
	rt.Parse("let urgent = query Tarea where prioridad == 3")
	vecs := filterCollection(rt.Collections["urgent"], "estado != 2")
	sort.Slice(vecs, func(i, j int) bool { return vecs[i].Values[0] < vecs[j].Values[0] })
	result := make([]map[string]any, len(vecs))
	for i, v := range vecs {
		result[i] = s.vecToJSON(v)
	}
	writeJSON(w, 200, result)
}

// GET /tasks/stats  — estadísticas algebraicas del espacio de tareas
func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	rt := s.newExecRT()
	rt.Parse("let all = query Tarea")
	vecs := rt.Collections["all"]
	n := len(vecs)
	if n == 0 {
		writeJSON(w, 200, map[string]any{"total": 0, "mensaje": "espacio vacío"})
		return
	}

	// Calcular centroide y estadísticas (excluyendo dim id=0)
	pending, inProgress, done := 0, 0, 0
	sumPri, sumSta, sumCat := 0.0, 0.0, 0.0
	for _, v := range vecs {
		sumPri += v.Values[1]
		sumSta += v.Values[2]
		sumCat += v.Values[3]
		switch int(v.Values[2]) {
		case StatusPending:
			pending++
		case StatusInProgress:
			inProgress++
		case StatusDone:
			done++
		}
	}
	fn := float64(n)
	cPri, cSta, cCat := sumPri/fn, sumSta/fn, sumCat/fn
	// Norma semántica del centroide (sin la dimensión id)
	normSem := math.Sqrt(cPri*cPri + cSta*cSta + cCat*cCat)

	writeJSON(w, 200, map[string]any{
		"total":           n,
		"pendientes":      pending,
		"en_progreso":     inProgress,
		"completadas":     done,
		"pct_completado":  fmt.Sprintf("%.1f%%", float64(done)/fn*100),
		"centroide_linlang": map[string]any{
			"prioridad_media": cPri,
			"estado_medio":    cSta,
			"categoria_media": cCat,
			"norma_semantica": normSem,
		},
		"backend": os.Getenv("LINLANG_DB"),
	})
}

// ── Router ────────────────────────────────────────────────────────────────────

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p, m := r.URL.Path, r.Method
	switch {
	case p == "/tasks" && m == "GET":
		s.listTasks(w, r)
	case p == "/tasks" && m == "POST":
		s.createTask(w, r)
	case p == "/tasks/urgent" && m == "GET":
		s.urgentTasks(w, r)
	case p == "/tasks/stats" && m == "GET":
		s.stats(w, r)
	case strings.HasSuffix(p, "/complete") && m == "PUT":
		s.stateHandler("completar", "complete")(w, r)
	case strings.HasSuffix(p, "/start") && m == "PUT":
		s.stateHandler("iniciar", "start")(w, r)
	case strings.HasSuffix(p, "/reopen") && m == "PUT":
		s.stateHandler("reabrir", "reopen")(w, r)
	case strings.HasSuffix(p, "/priority") && m == "PUT":
		s.updatePriority(w, r)
	case strings.HasSuffix(p, "/category") && m == "PUT":
		s.updateCategory(w, r)
	case strings.HasPrefix(p, "/tasks/") && m == "GET":
		s.getTask(w, r)
	case strings.HasPrefix(p, "/tasks/") && m == "DELETE":
		s.deleteTask(w, r)
	default:
		errJSON(w, 404, fmt.Sprintf("ruta no encontrada: %s %s", m, p))
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func idFromPath(path string) (int64, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return 0, fmt.Errorf("ruta inválida")
	}
	return strconv.ParseInt(parts[1], 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseOptInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// buildWhereClauses converts query string params to LinLang condition strings.
func buildWhereClauses(statusStr, priorityStr, categoryStr string) []string {
	var conds []string
	if s := parseOptInt(statusStr, -1); s >= 0 {
		conds = append(conds, fmt.Sprintf("estado == %d", s))
	}
	if p := parseOptInt(priorityStr, -1); p >= 0 {
		conds = append(conds, fmt.Sprintf("prioridad == %d", p))
	}
	if c := parseOptInt(categoryStr, -1); c >= 0 {
		conds = append(conds, fmt.Sprintf("categoria == %d", c))
	}
	return conds
}

// filterCollection applies a LinLang-style condition string to a vector slice.
// Used for secondary filters when query only accepts one condition.
func filterCollection(vecs []*core.Vector, cond string) []*core.Vector {
	if len(vecs) == 0 || cond == "" {
		return vecs
	}
	space := vecs[0].Space
	op, before, after := extractOperatorLocal(cond)
	if op == "" {
		return vecs
	}
	dimName := strings.TrimSpace(before)
	val, err := strconv.ParseFloat(strings.TrimSpace(after), 64)
	if err != nil {
		return vecs
	}
	dimIdx := -1
	for i, d := range space.Dimensions {
		if d == dimName {
			dimIdx = i
			break
		}
	}
	if dimIdx < 0 {
		return vecs
	}
	var result []*core.Vector
	for _, v := range vecs {
		if applyOp(op, v.Values[dimIdx], val) {
			result = append(result, v)
		}
	}
	return result
}

func extractOperatorLocal(s string) (op, before, after string) {
	for _, o := range []string{">=", "<=", "==", "!="} {
		if i := strings.Index(s, o); i >= 0 {
			return o, s[:i], s[i+2:]
		}
	}
	for _, o := range []string{">", "<"} {
		if i := strings.Index(s, o); i >= 0 {
			return o, s[:i], s[i+1:]
		}
	}
	return "", s, ""
}

func applyOp(op string, x, val float64) bool {
	switch op {
	case ">":
		return x > val
	case "<":
		return x < val
	case ">=":
		return x >= val
	case "<=":
		return x <= val
	case "==":
		return x == val
	case "!=":
		return x != val
	}
	return false
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	schema := "examples/todo.lin"
	if len(os.Args) > 1 {
		schema = os.Args[1]
	}
	port := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		port = ":" + p
	}

	// Cargar schema LinLang
	schemaRT := parser.NewRuntime()
	code, err := os.ReadFile(schema)
	if err != nil {
		log.Fatalf("❌ No se pudo leer schema '%s': %v", schema, err)
	}
	schemaRT.Parse(string(code))
	if schemaRT.Spaces["Tarea"] == nil {
		log.Fatalf("❌ Espacio 'Tarea' no encontrado en '%s'", schema)
	}

	// Abrir backend de persistencia
	db, err := store.Open("")
	if err != nil {
		log.Fatalf("❌ Error abriendo store: %v", err)
	}
	defer db.Close()

	dsn := os.Getenv("LINLANG_DB")
	if dsn == "" {
		dsn = "memory://"
	}

	srv := &Server{
		schemaRT: schemaRT,
		db:       db,
		titles:   make(map[int64]string),
		nextID:   1,
	}

	log.Printf("🚀  LinLang TODO server  —  %s", port)
	log.Printf("📐  Schema: %s  (%d spaces, %d transforms)",
		schema, len(schemaRT.Spaces), len(schemaRT.Transforms))
	log.Printf("💾  Store:  %s", dsn)
	log.Println()
	log.Println("  GET    /tasks[?status=&priority=&category=]")
	log.Println("  POST   /tasks                    {title, priority, category}")
	log.Println("  GET    /tasks/{id}")
	log.Println("  PUT    /tasks/{id}/start")
	log.Println("  PUT    /tasks/{id}/complete")
	log.Println("  PUT    /tasks/{id}/reopen")
	log.Println("  PUT    /tasks/{id}/priority      {priority:1-3}")
	log.Println("  PUT    /tasks/{id}/category      {category:1-3}")
	log.Println("  DELETE /tasks/{id}")
	log.Println("  GET    /tasks/urgent")
	log.Println("  GET    /tasks/stats")

	if err := http.ListenAndServe(port, srv); err != nil {
		log.Fatalf("❌ %v", err)
	}
}
