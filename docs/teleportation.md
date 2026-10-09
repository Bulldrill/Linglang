# Teleportación como primitiva de infraestructura

> Formaliza `core.Teleport` / `teleport(ψ)` (issue #14). Implementación:
> [`core/teleport.go`](../core/teleport.go), [`core/channel.go`](../core/channel.go).
> Ejemplo manual (gate a gate): [`examples/teleportacion.lin`](../examples/teleportacion.lin).
> Ejemplo con la primitiva: [`examples/teleport_builtin.lin`](../examples/teleport_builtin.lin).

## 1. Motivación

En un sistema distribuido clásico, "mover" un dato de un nodo A a un nodo B
es trivial: se copia el dato y se borra el original. El **teorema de
no-clonación** prohíbe esa estrategia para un qubit arbitrario |ψ⟩ — no
existe una operación unitaria `U` tal que `U(|ψ⟩⊗|0⟩) = |ψ⟩⊗|ψ⟩` para todo
|ψ⟩. Por tanto, **migrar** un qubit entre nodos requiere un protocolo
distinto: teleportación cuántica. Es la única forma de lograr "transparencia
de distribución" (ver `docs/teleportation.md` §4 y el capítulo de tesis
asociado, issue #43) sin violar la mecánica cuántica.

## 2. El protocolo, formalmente

Sean Alice (nodo A) y Bob (nodo B), conectados por un `QuantumChannel`.
Alice posee un qubit en el estado desconocido

```
|ψ⟩ = α|0⟩ + β|1⟩        (α,β ∈ ℂ, |α|²+|β|²=1)
```

que quiere migrar a Bob, sin transmitir el qubit físico.

**Paso 0 — recurso compartido.** El canal pre-provisiona un par de Bell

```
|Φ⁺⟩ = 1/√2 (|00⟩ + |11⟩)  ∈  H_A ⊗ H_B
```

y entrega a Alice el primer qubit, a Bob el segundo (`QuantumChannel.ShareBellPair`,
`core/channel.go`).

**Paso 1 — estado conjunto.** El sistema completo, de 3 qubits (ψ, mitad-A
de Bell, mitad-B de Bell), es

```
|ψ⟩ ⊗ |Φ⁺⟩ = 1/√2 [ α|0⟩(|00⟩+|11⟩) + β|1⟩(|00⟩+|11⟩) ]
```

**Paso 2 — Alice entrelaza su qubit con su mitad del par de Bell.**
Aplica `CNOT` (control=ψ, target=mitad-A) y luego `H` sobre ψ. Expandiendo
en la base de Bell de los dos primeros qubits, el estado se reescribe como:

```
1/2 Σ_{m₀,m₁∈{0,1}}  |m₀ m₁⟩ ⊗ (X^m₁ Z^m₀) |ψ⟩
```

Esta identidad es el corazón del protocolo: **para cada uno de los 4
resultados posibles de la medición de Alice, el qubit de Bob queda, salvo
una corrección Pauli conocida, exactamente en el estado |ψ⟩** — la
información de ψ se ha "esparcido" en la correlación entre los qubits
medidos y el qubit de Bob, sin que ψ haya viajado físicamente.

**Paso 3 — medición clásica.** Alice mide sus dos qubits, obtiene bits
clásicos `(m₀, m₁)`, y los envía a Bob por el canal clásico del
`QuantumChannel` (`SendClassical` / `RecvClassical`). Esta es la única
información que atraviesa el canal: dos bits clásicos, nunca el qubit.

**Paso 4 — corrección de Bob.** Bob aplica `X^m₁ · Z^m₀` a su qubit,
recuperando exactamente

```
(X^m₁ Z^m₀)⁻¹ (X^m₁ Z^m₀) |ψ⟩ = |ψ⟩
```

## 3. De circuito manual a primitiva de infraestructura

`examples/teleportacion.lin` implementa los pasos 1–4 manualmente con
`kron`, `apply`, `tensor`, `measure` — útil para enseñar el protocolo, pero
inadecuado como mecanismo de migración real: cada llamada expone qubits
internos de Alice (q₀,q₁,q₂) que un runtime distribuido no debería
necesitar conocer.

`core.Teleport(psi *QuantumState, ch QuantumChannel) (*TeleportResult, error)`
eleva el protocolo a primitiva: el llamador solo ve *qué* qubit migra y
*sobre qué canal*, no la mecánica interna de puertas y mediciones. El
builtin `teleport(ψ)` del lenguaje (`parser/quantum.go`) lo expone a nivel
de sintaxis LinLang:

```linlang
hilbert Qubit: dim 2
let psi = ket(Qubit, 0.6, 0.8)
let bob = teleport(psi)   # bob ahora contiene |ψ⟩, migrado sin clonarlo
```

## 4. Qué significa "transparencia de distribución" aquí

En microservicios clásicos, transparencia de distribución significa que el
código no necesita saber en qué máquina vive un dato. Para qubits, no puede
significar "copiar el dato a donde se necesite" (no-cloning lo prohíbe).
Significa, en cambio: **el programador invoca una operación de migración
(`teleport`) y el runtime decide cómo satisfacerla** — vía un par de Bell
pre-provisionado localmente (`LocalChannel`, hoy) o, en un despliegue real
(issues #16–#19), vía un par de Bell distribuido entre nodos físicos
separados y un canal clásico de red. La interfaz `QuantumChannel` es
justamente ese punto de abstracción: el protocolo de la sección 2 no
cambia, solo cambia cómo se implementa `ShareBellPair`/`SendClassical`.

## 5. Verificación

`core/teleport_test.go::TestTeleportRecoversState` verifica, para varios
|ψ⟩ (incluyendo superposiciones asimétricas), que
`Teleport(ψ, ch).State == ψ` exactamente (tolerancia 1e-9), para cualquiera
de los 4 resultados de medición que el canal produzca — es la propiedad
central del protocolo, no un caso particular.
