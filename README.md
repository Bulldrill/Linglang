# LinLang 🧮

> Un lenguaje de programación algebraico basado en espacios vectoriales, con extensión de computación cuántica y persistencia transparente.

LinLang modela cada dominio de datos como un **espacio vectorial**, cada entidad como un **vector** y cada relación como una **transformación algebraica**. El mismo archivo `.lin` se ejecuta sin modificación sobre SQLite, Redis, Docker o (en la hoja de ruta) backends cuánticos reales.

---

## Características

| Módulo | Estado | Descripción |
|--------|--------|-------------|
| Núcleo algebraico | ✅ | `space`, `let`, `transform`, `when`, `print`, operaciones vectoriales |
| Operaciones nativas | ✅ | `add`, `scale`, `dot`, `norm`, `project` |
| Extensión cuántica (LinLang/Q) | ✅ | `hilbert`, `gate`, `ket`, `apply`, `tensor`, `measure`, `density`, `partial_trace`, `kron` |
| Persistencia transparente | ✅ | `persist`, `drop`, `query`, `query_one` — SQLite y Redis |
| Backend TODO REST | ✅ | 10 endpoints HTTP completamente dirigidos por el runtime LinLang |
| Docker Compose | ✅ | Todo-server + Redis con un solo comando |
| Simulador de tecnología cuántica | 🗺️ | Backlog v2.0 |
| Distribución de qubits | 🗺️ | Backlog v3.0 |

---

## Inicio rápido

```bash
git clone https://github.com/Bulldrill/Linglang.git
cd Linglang
go mod tidy
go run . examples/ejemplo.lin
```

### Correr el servidor TODO con Redis

```bash
docker compose up --build
```

```bash
# Crear una tarea
curl -s -X POST localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{"title":"Implementar LinLang/Q","priority":3,"category":1}'

# Listar todas
curl -s localhost:8080/tasks | python3 -m json.tool

# Completar
curl -s -X PUT localhost:8080/tasks/1/complete

# Estadísticas algebraicas del espacio de tareas
curl -s localhost:8080/tasks/stats | python3 -m json.tool
```

---

## Sintaxis

### Núcleo clásico

```linlang
# Espacios vectoriales (dominios)
space Personas: edad: Real, ingresos: Real, energia: Real
space Adopciones: persona_id: Real, mascota_id: Real, compat: Real

# Vectores (entidades)
let p1 = Personas[25, 40000, 0.8]

# Transform declarativo
transform adoptar: Personas x Mascotas -> Adopciones {
    persona_id = p.id
    mascota_id = m.id
    compat     = p.energia * m.edad / 10
}

# Operaciones vectoriales nativas
let suma      = add(p1, p2)
let similitud = dot(p1, p2)
let magnitud  = norm(p1)
let perfil    = project p1 onto Perfil

# Condicionales algebraicos
when a1.compat >= 0.2:
    approve(a1)
```

### Extensión cuántica (LinLang/Q)

```linlang
hilbert Qubit: dim 2
hilbert QReg2: dim 4

# Puertas unitarias (con verificación U†U = I automática)
gate H: Qubit -> Qubit {
    row [0.7071067811865476,  0.7071067811865476]
    row [0.7071067811865476, -0.7071067811865476]
}

# Estados y operaciones
let ket0    = ket(Qubit, 1, 0)
let plus    = apply(H, ket0)           # H|0⟩ = |+⟩
let q00     = tensor(ket0, ket0)       # |00⟩
let rho     = density(plus)            # ρ = |ψ⟩⟨ψ|
let rho_A   = trace_alice(rho, 2)      # traza parcial

# Puertas compuestas por producto de Kronecker
let HII     = kron(H_Qubit, kron(I_Qubit, I_Qubit))  # H⊗I⊗I

# Condicionales cuánticos
when purity(rho) >= 0.99:
    estado_puro(rho)
```

### Persistencia transparente

```linlang
space Tarea: id: Real, prioridad: Real, estado: Real, categoria: Real

let t1 = Tarea[1, 3, 0, 1]
persist t1                              # INSERT / UPSERT

let urgentes = query Tarea where prioridad == 3   # SELECT WHERE
let t        = query_one Tarea where id == 1      # SELECT LIMIT 1

let t_done = completar(t, t)
persist t_done                          # UPDATE

drop t1                                 # DELETE
```

Selección del backend mediante variable de entorno:

```bash
LINLANG_DB=sqlite://linlang.db  go run . examples/todo_db.lin
LINLANG_DB=redis://localhost:6379/0  go run . examples/todo_db.lin
```

---

## Estructura del proyecto

```
Linglang/
├── core/                   # Tipos algebraicos fundamentales
│   ├── spaces.go           # Space (espacio vectorial)
│   ├── vectors.go          # Vector + Scale, Norm, Project, Add, Dot
│   ├── transforms.go       # Transform (función entre espacios)
│   ├── conditionals.go     # Conditional con clausura GetValue
│   ├── expressions.go      # Parser recursivo de expresiones aritméticas
│   ├── hilbert.go          # HilbertSpace, QuantumState, DensityMatrix
│   └── gates.go            # Gate, KronGate, BuiltinH/X/Y/Z/CNOT/I
├── parser/
│   ├── parser.go           # Runtime + dispatch principal
│   ├── db.go               # persist / drop / query / query_one
│   └── quantum.go          # hilbert / gate / ket / apply / tensor / ...
├── store/
│   ├── store.go            # Interfaz Backend + factory Open(dsn)
│   ├── memory.go           # Backend en memoria (tests)
│   ├── sqlite.go           # SQLite (modernc.org/sqlite, puro Go)
│   └── redis.go            # Redis (go-redis/v9)
├── cmd/
│   └── todo-server/
│       └── main.go         # Servidor HTTP 100% LinLang-driven
├── examples/
│   ├── ejemplo.lin         # Demo núcleo clásico
│   ├── ejemplo_quantum.lin # Demo LinLang/Q completo
│   ├── teleportacion.lin   # Protocolo de teleportación cuántica
│   ├── todo.lin            # Schema de la app TODO
│   └── todo_db.lin         # Demo de persistencia nativa
├── scripts/
│   └── gh_import.py        # Importar backlog a GitHub Issues
├── Dockerfile              # Imagen del intérprete LinLang
├── Dockerfile.todo         # Imagen del servidor TODO (multi-stage)
├── docker-compose.yml      # todo-server + Redis
├── monografia.tex          # Tesis doctoral en LaTeX
├── linlang_backlog.xlsx    # Backlog de 48 issues (5 épicas)
└── go.mod
```

---

## API del servidor TODO

| Método | Ruta | Descripción |
|--------|------|-------------|
| `POST` | `/tasks` | Crear tarea `{title, priority, category}` |
| `GET` | `/tasks[?status=&priority=&category=]` | Listar con filtros |
| `GET` | `/tasks/{id}` | Obtener por ID |
| `PUT` | `/tasks/{id}/start` | Iniciar (estado → 1) |
| `PUT` | `/tasks/{id}/complete` | Completar (estado → 2) |
| `PUT` | `/tasks/{id}/reopen` | Reabrir (estado → 0) |
| `PUT` | `/tasks/{id}/priority` | `{priority: 1-3}` |
| `PUT` | `/tasks/{id}/category` | `{category: 1-3}` |
| `DELETE` | `/tasks/{id}` | Eliminar |
| `GET` | `/tasks/urgent` | Prioridad alta y no completadas |
| `GET` | `/tasks/stats` | Estadísticas algebraicas (centroide, norma) |

Cada endpoint genera código LinLang en tiempo de ejecución y lo ejecuta en el runtime. La lógica de negocio vive en `examples/todo.lin`, no en el servidor.

---

## Hoja de ruta

Los 48 issues están en [GitHub Issues](https://github.com/Bulldrill/Linglang/issues) organizados en 5 épicas:

- **⚛️ Quantum Simulator** — backends por tecnología (superconductores, iones atrapados, átomos neutros), QIR, inserción de SWAPs, decoherencia
- **🌐 Distributed Qubits** — teleportación como primitiva de infraestructura, scheduler cuántico, Docker QPU nodes, Kubernetes operator
- **🔧 LinLang Core** — tipo String, colecciones de primera clase, multi-condición en query, módulos/imports, REPL, LSP
- **🐳 Infrastructure** — CI/CD, tests unitarios e integración, multi-arch Docker
- **📚 Thesis** — formalización del álgebra LinLang/Q, prueba de corrección del modelo distribuido, comparativa con Qiskit/Q#/Cirq

---

## Dependencias

- **Go 1.22+**
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — SQLite puro Go (sin CGO)
- [github.com/redis/go-redis/v9](https://github.com/redis/go-redis) — cliente Redis
- Docker + Docker Compose (para el servidor TODO)

```bash
go mod tidy   # descarga todas las dependencias
```

---

## Publicación académica

La monografía doctoral completa en formato LaTeX está disponible en `monografia.tex`. Compila con:

```bash
pdflatex monografia.tex && pdflatex monografia.tex
```

---

<p align="center">
  <em>LinLang — Algebraic Programming · Vector Spaces · Quantum Extension · Transparent Persistence</em>
</p>
