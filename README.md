# LinLang 🧮

[![CI](https://github.com/Bulldrill/Linglang/actions/workflows/ci.yml/badge.svg)](https://github.com/Bulldrill/Linglang/actions/workflows/ci.yml)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)

> Un lenguaje de programación algebraico basado en espacios vectoriales, con extensión de computación cuántica, distribución de qubits por teleportación, y persistencia transparente.

LinLang modela cada dominio de datos como un **espacio vectorial**, cada entidad como un **vector** y cada relación como una **transformación algebraica**. El mismo modelo se extiende, sin cambiar de lenguaje, a espacios de Hilbert sobre ℂ — un qubit es un vector como cualquier otro. El mismo archivo `.lin` se ejecuta sin modificación sobre SQLite, Redis, Docker, un clúster de Kubernetes, o backends cuánticos reales (IBM, IonQ).

📖 **[Referencia completa del lenguaje](https://bulldrill.github.io/Linglang/)** — sintaxis, modelo de memoria, extensión cuántica y runtime distribuido, en un sitio navegable.

---

## Características

Las 5 épicas del backlog original (49 issues) están **completas**:

| Módulo | Estado | Descripción |
|--------|--------|-------------|
| Núcleo algebraico | ✅ | `space` (tipado `Real`/`String`, inferencia), `let`, `transform`, `func`, `when`, `for`/`filter`/`map`, `try`/`catch`, `include`, REPL |
| Operaciones nativas | ✅ | `add`, `scale`, `dot`, `norm`, `project` |
| Extensión cuántica (LinLang/Q) | ✅ | `hilbert`, `gate`, `ket`, `apply`, `tensor`, `measure`, `shots`, `density`, `partial_trace`, `kron`, `teleport`, `technology` |
| Backends cuánticos reales | ✅ | IBM Quantum y IonQ (clientes HTTP reales), GPU/CUDA, simulador con ruido T1/T2 |
| Compilación de circuitos | ✅ | QIR (DAG intermedio), optimizador algebraico, ruteo topológico de SWAP |
| Distribución de qubits | ✅ | `QuantumChannel`, teleportación como primitiva de infraestructura, scheduler, nodo gRPC, operator de Kubernetes |
| Persistencia transparente | ✅ | `persist`, `drop`, `query` (multi-condición `and`/`or`), `query_one` — memoria, SQLite y Redis |
| Backend TODO REST | ✅ | 10 endpoints HTTP completamente dirigidos por el runtime LinLang |
| Herramientas | ✅ | Language Server Protocol, CI/CD con builds multi-arquitectura |
| Docker / Kubernetes | ✅ | Todo-server + Redis con un comando; CRD `HilbertSpace` con operator real |

Detalle completo, incluyendo qué quedó deliberadamente fuera de alcance y por qué (ver `monografia.tex`, capítulo "Hoja de Ruta").

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
├── core/                   # Tipos algebraicos fundamentales + extensión cuántica
│   ├── spaces.go           # Space (espacio vectorial), DimType (Real/String)
│   ├── vectors.go          # Vector + Scale, Norm, Project, Add, Dot
│   ├── transforms.go       # Transform (función entre espacios)
│   ├── conditionals.go     # Conditional con clausura GetValue
│   ├── expressions.go      # Parser recursivo de expresiones aritméticas
│   ├── hilbert.go          # HilbertSpace, QuantumState, DensityMatrix
│   ├── gates.go            # Gate, KronGate, BuiltinH/X/Y/Z/CNOT/I/SWAP
│   ├── backend.go          # Interfaz QuantumBackend / CircuitRunner
│   ├── ibm_backend.go      # SuperconductorBackend — cliente HTTP real (IBM)
│   ├── ionq_backend.go     # TrappedIonBackend — cliente HTTP real (IonQ)
│   ├── cuda_backend.go     # CUDASimBackend — bindings cgo a cuStateVec
│   ├── multi_gpu_backend.go# MultiGPUBackend — primitivo de distribución NCCL
│   ├── noise.go / noisy_backend.go  # Decoherencia T1/T2, error de lectura
│   ├── qir.go / optimizer.go / routing.go  # IR intermedia, optimización, SWAP
│   ├── channel.go / teleport.go / bell_pool.go  # QuantumChannel, Teleport
│   ├── hilbert_registry.go # Propiedad de espacios por nodo
│   ├── scheduler.go        # ScheduleCircuit / ClusterGraph
│   └── remote_backend.go   # Cliente gRPC hacia quantum-node
├── parser/
│   ├── parser.go           # Runtime + dispatch principal, for/try/func/include
│   ├── db.go                # persist / drop / query / query_one / filter / map
│   └── quantum.go          # hilbert / gate / ket / apply / teleport / ...
├── store/                  # Backend de persistencia (memoria, SQLite, Redis)
├── cmd/
│   ├── todo-server/        # Servidor HTTP 100% LinLang-driven
│   ├── quantum-node/       # Servidor gRPC: un contenedor = un QPU simulado
│   ├── operator/           # Operator de Kubernetes (manager de controller-runtime)
│   └── lsp/                # Language Server Protocol sobre stdio
├── api/v1alpha1/           # CRD HilbertSpace (tipos Go + DeepCopy generado)
├── internal/controller/    # Reconciler del operator de Kubernetes
├── proto/quantumnode/      # Definición gRPC del servicio QuantumNode
├── config/                 # CRD YAML + RBAC generados con controller-gen
├── docs/
│   ├── index.html          # Referencia del lenguaje (sitio de GitHub Pages)
│   ├── architecture.md     # Diagramas UML de la arquitectura
│   └── teleportation.md    # Formalización de la teleportación
├── examples/                # Programas .lin de ejemplo
├── Dockerfile               # Imagen del intérprete LinLang
├── Dockerfile.todo           # Imagen del servidor TODO (multi-stage)
├── Dockerfile.quantum-node   # Imagen del nodo cuántico gRPC
├── docker-compose.yml        # todo-server + Redis (+ perfil SQLite)
├── .github/workflows/ci.yml  # build, vet, gofmt, tests, Docker multi-arch
├── monografia.tex             # Tesis doctoral en LaTeX
├── LICENSE                    # GNU GPL v3
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

## Estado del backlog

Los 49 issues en [GitHub Issues](https://github.com/Bulldrill/Linglang/issues?q=is%3Aissue+is%3Aclosed) (todos cerrados) están organizados en 5 épicas:

- **⚛️ Quantum Simulator** — `QuantumBackend`/`CircuitRunner`, backends reales (IBM Quantum, IonQ), GPU/CUDA, QIR con optimizador y ruteo de SWAPs, simulación shot-based, decoherencia T1/T2 + error de lectura
- **🌐 Distributed Qubits** — `QuantumChannel`, teleportación como primitiva de infraestructura, `HilbertRegistry`, scheduler (`ScheduleCircuit`), nodo cuántico por gRPC, operator de Kubernetes (CRD `HilbertSpace`, verificado contra un clúster `kind` real)
- **🔧 LinLang Core** — tipo `String`, colecciones de primera clase, multi-condición en `query`, módulos (`include`), REPL, funciones de usuario (`func`), inferencia de tipos, manejo de errores (`try`/`catch`), Language Server Protocol
- **🐳 Infrastructure** — CI/CD en GitHub Actions, tests unitarios e integración, benchmarks, Docker multi-arquitectura (amd64+arm64)
- **📚 Thesis** — formalización del álgebra de LinLang/Q, especificación formal (gramática, tipos, semántica operacional), prueba de corrección del modelo distribuido, comparativa con Qiskit/Q#/Cirq/OpenQASM

Quedó deliberadamente fuera de alcance (y documentado por qué): un backend para QuEra/Aquila (modelo analógico, no de puertas digitales) y la composición genérica de puertas arbitrarias en `MultiGPUBackend` (sin hardware multi-GPU para verificarla). Ver `monografia.tex`, capítulo "Hoja de Ruta".

---

## Documentación

| Recurso | Contenido |
|---|---|
| 📖 [**Referencia del lenguaje**](https://bulldrill.github.io/Linglang/) | Sitio navegable: sintaxis completa, modelo de memoria y estructuras de datos, LinLang/Q, backends y distribución, herramientas |
| [`docs/architecture.md`](docs/architecture.md) | Diagramas UML (clases y secuencia) de la arquitectura actual |
| [`docs/teleportation.md`](docs/teleportation.md) | Formalización del protocolo de teleportación como primitiva de infraestructura |
| [`monografia.tex`](monografia.tex) | Monografía doctoral completa: fundamentos, especificación formal, pruebas de corrección |

---

## Dependencias

- **Go 1.22+**
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — SQLite puro Go (sin CGO)
- [github.com/redis/go-redis/v9](https://github.com/redis/go-redis) — cliente Redis
- [google.golang.org/grpc](https://grpc.io/) — servicio `quantum-node` y su cliente (`core.RemoteBackend`)
- [sigs.k8s.io/controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) + `k8s.io/client-go` — operator de Kubernetes (`cmd/operator`)
- Docker + Docker Compose (para el servidor TODO y el nodo cuántico)
- Opcional: NVIDIA cuQuantum/NCCL (`-tags cuda`, `-tags "cuda nccl"`) y un clúster Kubernetes (`kind` para desarrollo local) para los componentes que lo requieren

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

## Licencia

LinLang está publicado bajo la **GNU General Public License v3.0**. Ver [`LICENSE`](LICENSE) para el texto completo.

```
Copyright (C) 2026  Aaronsoria

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
```

---

<p align="center">
  <em>LinLang — Algebraic Programming · Vector Spaces · Quantum Extension · Transparent Persistence</em>
</p>
