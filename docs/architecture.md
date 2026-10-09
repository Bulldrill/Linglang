# Arquitectura de LinLang — diagramas UML

> Generado a partir del estado del código en el commit `e9e36fd` (2026-10-08).
> Diagramas en sintaxis [Mermaid](https://mermaid.js.org/) (notación UML de clases y
> secuencia) — GitHub los renderiza automáticamente al ver este archivo.

## 1. Vista de componentes

Cuatro paquetes Go. `core` no depende de nada del proyecto; `store` solo depende de
`core`; `parser` depende de ambos; `cmd/todo-server` y `main.go` son los dos binarios,
ambos thin-shells sobre `parser.Runtime`.

```mermaid
flowchart TB
    main["main.go<br/>(REPL de archivo .lin)"]
    todo["cmd/todo-server<br/>(backend REST TODO)"]
    parser["parser<br/>Runtime: dispatch de línea .lin"]
    core["core<br/>álgebra clásica + cuántica + distribución"]
    store["store<br/>persistencia transparente"]

    main --> parser
    todo --> parser
    todo --> store
    parser --> core
    parser --> store
    store --> core

    style core fill:#2b4c7e,color:#fff
    style parser fill:#3a6b8a,color:#fff
    style store fill:#3a6b8a,color:#fff
    style todo fill:#5a8296,color:#fff
    style main fill:#5a8296,color:#fff
```

## 2. `core` — álgebra clásica

`Space`/`Vector` son el núcleo vectorial sobre ℝ; `DimType` (issue #24) distingue
dimensiones que participan en la aritmética (`Real`) de las que son metadata (`String`).
`Transform` son los morfismos entre espacios; `Expr` es el parser recursivo-descendente
de expresiones (`p.dim * m.dim / 10`) que alimenta el cuerpo de un `transform`.

```mermaid
classDiagram
    class DimType {
        <<enumeration>>
        Real
        String
    }

    class Space {
        +Name string
        +Dimensions []string
        +Types []DimType
        +DimType(dim) DimType
    }

    class Vector {
        +ID int64
        +Space *Space
        +Values []float64
        +Strings map[string]string
        +Add(o *Vector) *Vector
        +Dot(o *Vector) float64
        +Scale(factor float64) *Vector
        +Norm() float64
        +Project(target *Space) *Vector
        +Get(dim string) float64
        +GetString(dim string) string
        +SetString(dim, value string)
        +Display() []any
    }

    class Transform {
        +Name string
        +Domain1 *Space
        +Domain2 *Space
        +Codomain *Space
        +Function func(*Vector, *Vector) *Vector
        +Apply(p, m *Vector) *Vector
    }

    class Conditional {
        +Label string
        +GetValue func() float64
        +Condition func(float64) bool
        +Action func()
        +Evaluate()
    }

    class Expr {
        <<interface>>
        +Eval(p, m *Vector) float64
    }
    class NumExpr {
        +V float64
    }
    class FieldExpr {
        +Src string
        +Dim string
    }
    class BinExpr {
        +Op string
        +Left Expr
        +Right Expr
    }

    Expr <|.. NumExpr
    Expr <|.. FieldExpr
    Expr <|.. BinExpr
    BinExpr --> Expr : Left / Right
    Vector --> Space
    Transform --> Space : Domain1 / Domain2 / Codomain
    Conditional ..> Vector : NewVectorConditional
```

## 3. `core` — simulación cuántica (LinLang/Q)

`HilbertSpace` es la especialización de `Space` sobre ℂ. `Gate`/`QuantumState`/
`DensityMatrix` implementan el álgebra de la Sección "Formalización Algebraica" de la
tesis. `QuantumBackend` (issue #1) desacopla el runtime del motor de ejecución;
`CircuitRunner` (issue #3/#4) es el camino para hardware real, que no puede aceptar un
vector de amplitudes arbitrario. `QIRCircuit` (issue #7) es el DAG intermedio que
`OptimizeCircuit` (#8) y `RouteCircuit` (#9) transforman antes de la ejecución.

```mermaid
classDiagram
    class HilbertSpace {
        +Name string
        +Dim int
    }

    class QuantumState {
        +Space *HilbertSpace
        +Amplitudes []complex128
        +Norm() float64
        +Normalize() *QuantumState
        +Braket(other) complex128
        +Tensor(other, space) *QuantumState
        +Probabilities() []float64
        +Measure() (int, []float64)
        +Shot(rng) int
        +Shots(n, rng) map[int]int
        +ToDensityMatrix() *DensityMatrix
    }

    class DensityMatrix {
        +Space *HilbertSpace
        +Matrix [][]complex128
        +Trace() float64
        +Purity() float64
        +PartialTrace(subDim) *DensityMatrix
        +PartialTraceA(keepDim) *DensityMatrix
    }

    class Gate {
        +Name string
        +Domain *HilbertSpace
        +Codomain *HilbertSpace
        +Matrix [][]complex128
        +Apply(state) *QuantumState
        +IsUnitary() bool
    }

    class Topology {
        +Qubits int
        +Edges [][2]int
        +Connected(a, b int) bool
    }
    class NoiseModel {
        +T1 float64
        +T2 float64
        +ReadoutError float64
    }

    class QuantumBackend {
        <<interface>>
        +Name() string
        +Apply(gate, state) *QuantumState
        +Measure(state) (int, []float64)
        +SupportedGates() []string
        +Topology() Topology
        +NoiseModel() NoiseModel
    }
    class CircuitRunner {
        <<interface>>
        +Run(circuit, shots) map[int]int
    }
    class SimulatorBackend
    class CUDASimBackend
    class SuperconductorBackend {
        -apiKey string
        -crn string
        -backend string
    }
    class TrappedIonBackend {
        -apiKey string
        -target string
    }

    QuantumBackend <|.. SimulatorBackend
    QuantumBackend <|.. CUDASimBackend
    CircuitRunner --|> QuantumBackend
    CircuitRunner <|.. SuperconductorBackend
    CircuitRunner <|.. TrappedIonBackend

    class QIRNode {
        +ID int
        +Gate *Gate
        +Qubits []int
        +Deps []*QIRNode
    }
    class QIRCircuit {
        +NumQubits int
        +Nodes []*QIRNode
        +AddGate(gate, qubits) *QIRNode
        +TopoOrder() []*QIRNode
        +Depth() int
        +Execute(backend, initial) *QuantumState
    }

    QIRCircuit "1" --> "*" QIRNode
    QIRNode --> Gate
    QIRNode --> QIRNode : Deps
    QIRCircuit ..> QuantumBackend : Execute
    QuantumState --> HilbertSpace
    Gate --> HilbertSpace : Domain / Codomain
    DensityMatrix --> HilbertSpace
    QuantumState ..> DensityMatrix : ToDensityMatrix
```

## 4. `core` — distribución y teleportación

`QuantumChannel` (issue #15) es la abstracción de un enlace cuántico entre dos nodos.
`BellPool` (#17) decora un `QuantumChannel` con un buffer pre-provisionado.
`HilbertRegistry` (#16) registra qué nodo posee cada espacio. `ScheduleCircuit`/
`ClusterGraph` (#18/#19) particionan un circuito entre nodos insertando `Teleport` (#14)
donde haga falta — nunca `SWAP`, porque el no-clonación lo prohíbe (ver
`docs/teleportation.md` y la Sección "Hilbert Spaces Distribuidos" de la tesis).

```mermaid
classDiagram
    class BellPair {
        +Space *HilbertSpace
        +State *QuantumState
        +QubitA int
        +QubitB int
    }

    class QuantumChannel {
        <<interface>>
        +NodeA() string
        +NodeB() string
        +ShareBellPair() *BellPair
        +SendClassical(from, bits)
        +RecvClassical(to) []int
    }

    class LocalChannel {
        -nodeA string
        -nodeB string
        -qubitSpace *HilbertSpace
        -inboxes map
    }

    class BellPool {
        -channel QuantumChannel
        -target int
        -buffer []*BellPair
        +Refill() error
        +Available() int
    }

    QuantumChannel <|.. LocalChannel
    QuantumChannel <|.. BellPool
    BellPool --> QuantumChannel : wraps
    LocalChannel ..> BellPair : ShareBellPair()

    class NodeID {
        <<string>>
    }
    class DistributedHilbertSpace {
        +HilbertSpace
        +Owner NodeID
    }
    class HilbertRegistry {
        -spaces map[string]*DistributedHilbertSpace
        +Register(space, owner)
        +Lookup(name) DistributedHilbertSpace
        +Owner(name) NodeID
        +Transfer(name, newOwner)
        +Local(name, node) bool
    }
    HilbertRegistry --> DistributedHilbertSpace
    DistributedHilbertSpace --> HilbertSpace

    class ClusterGraph {
        -edges map[NodeID]map[NodeID]bool
        +AddChannel(ch QuantumChannel)
        +Connected(a, b NodeID) bool
    }
    class TeleportStep {
        +Qubit int
        +From NodeID
        +To NodeID
    }
    class TeleportResult {
        +State *QuantumState
        +M0 int
        +M1 int
    }

    class TeleportFn {
        <<function>>
        Teleport(psi, ch) TeleportResult
    }
    class ScheduleCircuitFn {
        <<function>>
        ScheduleCircuit(circuit, ownerOf, cluster) TeleportStep[]
    }

    TeleportFn ..> QuantumChannel : usa
    TeleportFn ..> TeleportResult : produce
    ScheduleCircuitFn ..> ClusterGraph : usa
    ScheduleCircuitFn ..> TeleportStep : produce
    ScheduleCircuitFn ..> TeleportFn : implica en runtime
    ClusterGraph --> QuantumChannel
```

## 5. `parser` — el intérprete `Runtime`

`Runtime` es el intérprete completo: un diccionario de namespaces (uno por tipo de
valor) más el estado de los bloques multi-línea (`transform { }`, `gate { }`,
`for { }`). No hay AST persistente — cada línea se interpreta y descarta
(`ParseLine`), excepto dentro de esos tres bloques, que acumulan su cuerpo hasta `}`.

```mermaid
classDiagram
    class Runtime {
        +Spaces map[string]*core.Space
        +Vectors map[string]*core.Vector
        +Scalars map[string]float64
        +Transforms map[string]*core.Transform
        +LastCond *core.Conditional
        +HilbertSpaces map[string]*core.HilbertSpace
        +QuantumStates map[string]*core.QuantumState
        +DensityMatrices map[string]*core.DensityMatrix
        +Gates map[string]*core.Gate
        +Measurements map[string]int
        +Backend core.QuantumBackend
        +Histograms map[string]map[int]int
        +Store store.Backend
        +Collections map[string][]*core.Vector
        -inTransform bool
        -inGate bool
        -inFor bool
        +ParseLine(line string)
        +Parse(code string)
        +quantumBackend() core.QuantumBackend
    }

    class pendingTransform {
        -name, dom1, dom2, codomain string
        -mappings map[string]core.Expr
    }
    class pendingGateDecl {
        -name, dom, cod string
        -rows [][]complex128
    }
    class pendingForLoop {
        -varName, collName string
        -body []string
    }

    Runtime --> pendingTransform : mientras parsea transform
    Runtime --> pendingGateDecl : mientras parsea gate
    Runtime --> pendingForLoop : mientras parsea for
    Runtime --> StoreBackend : persist / drop / query
    Runtime --> CoreTypes : Spaces, Vectors, HilbertSpaces, Gates, ...

    class StoreBackend["store.Backend"]
    class CoreTypes["core: Space, Vector, HilbertSpace, Gate, ..."]
```

## 6. `store` — persistencia transparente

Un único `Backend` con tres implementaciones intercambiables vía `LINLANG_DB`
(`memory://`, `sqlite://`, `redis://`). Desde #24, `Upsert`/`Query` también llevan las
dimensiones `String` (antes se perdían al pasar por el store).

```mermaid
classDiagram
    class Backend {
        <<interface>>
        +Upsert(space, values, strs) error
        +Query(space, filter) []*core.Vector
        +Delete(space, id) error
        +Close() error
    }

    class MemoryStore {
        -data map[string]map[float64]*storedRow
    }
    class storedRow {
        -values []float64
        -strs map[string]string
    }
    class SQLiteStore {
        -db *sql.DB
        -tables map[string]bool
    }
    class RedisStore {
        -client *redis.Client
        -ctx context.Context
    }

    Backend <|.. MemoryStore
    Backend <|.. SQLiteStore
    Backend <|.. RedisStore
    MemoryStore --> storedRow
```

## 7. `cmd/todo-server` — thin shell HTTP

Cada endpoint genera un fragmento `.lin`, lo corre en un `Runtime` fresco (con el
schema precargado) y traduce el `Vector` resultante a JSON. `titles` vive fuera de
LinLang porque el schema `examples/todo.lin` no declara `titulo` como dimensión
`String` — podría migrar a eso ahora que #24 existe, pero no se tocó en esta sesión.

```mermaid
classDiagram
    class Server {
        -schemaRT *parser.Runtime
        -db store.Backend
        -titles map[int64]string
        -nextID int64
        +ServeHTTP(w, r)
        +createTask(w, r)
        +listTasks(w, r)
        +getTask(w, r)
        +stateHandler(txName, suffix) HandlerFunc
        +updatePriority(w, r)
        +updateCategory(w, r)
        +deleteTask(w, r)
        +urgentTasks(w, r)
        +stats(w, r)
    }

    Server --> ParserRuntime : newExecRT() por request
    Server --> StoreBackend2

    class ParserRuntime["parser.Runtime"]
    class StoreBackend2["store.Backend"]
```

## 8. Secuencia: protocolo de teleportación

`core.Teleport` (issue #14), construido sobre `QuantumChannel` (#15) — ver
`docs/teleportation.md` para la formalización completa.

```mermaid
sequenceDiagram
    participant Caller as Runtime (teleport(psi))
    participant Ch as QuantumChannel
    participant T as core.Teleport

    Caller->>T: Teleport(psi, ch)
    T->>Ch: ShareBellPair()
    Ch-->>T: BellPair{State, QubitA, QubitB}
    T->>T: state = psi ⊗ pair.State
    T->>T: aplica CNOT, luego H (Alice)
    T->>T: outcome = state.Measure() → (m0, m1)
    T->>Ch: SendClassical(NodeA, [m0, m1])
    T->>Ch: RecvClassical(NodeB)
    Ch-->>T: [m0, m1]
    T->>T: colapsa amplitudes de Bob, normaliza
    T->>T: aplica X^m1 · Z^m0 (corrección de Bob)
    T-->>Caller: TeleportResult{State, M0, M1}
    Note over T: State == psi exacto, para los 4 resultados posibles de medición
```

## 9. Secuencia: request HTTP → LinLang → store

Ejemplo con `PUT /tasks/{id}/complete` — el mismo patrón (generar código `.lin`,
ejecutarlo en un `Runtime` fresco) se repite en los 10 endpoints.

```mermaid
sequenceDiagram
    participant C as Cliente HTTP
    participant S as Server
    participant RT as parser.Runtime (fresco)
    participant DB as store.Backend

    C->>S: PUT /tasks/42/complete
    S->>S: genera código .lin:<br/>let t = query_one Tarea where id == 42<br/>let t_new = completar(t, t)<br/>persist t_new
    S->>RT: newExecRT() + Parse(código)
    RT->>DB: Query(Tarea, filter id==42)
    DB-->>RT: Vector t
    RT->>RT: t_new = completar.Apply(t, t)
    RT->>DB: Upsert(Tarea, t_new.Values, t_new.Strings)
    DB-->>RT: ok
    RT-->>S: rt.Vectors["t_new"]
    S-->>C: 200 {id, status:"completada", ...}
```
