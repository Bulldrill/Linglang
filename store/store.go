// store/store.go
//
// Capa de persistencia de LinLang.
//
// El usuario del lenguaje escribe:
//
//	persist t1           → upsert del vector t1 en su espacio
//	drop t1              → eliminar por id
//	let all = query Tarea                      → todos los vectores
//	let pending = query Tarea where estado == 0 → filtrado
//
// El driver real (SQLite, Redis, memoria) es completamente transparente
// al programa .lin. Se selecciona con la variable de entorno LINLANG_DB:
//
//	LINLANG_DB=sqlite://linlang.db
//	LINLANG_DB=redis://localhost:6379/0
//	LINLANG_DB=memory://
package store

import (
	"fmt"
	"os"
	"strings"

	"linlang-go/core"
)

// Backend es la interfaz que deben implementar todos los drivers de
// persistencia.  Las operaciones trabajan sobre vectores LinLang, no
// sobre tipos SQL ni Redis.
type Backend interface {
	// Upsert inserta o actualiza un vector.
	// La primera dimensión del espacio se trata como clave primaria.
	// strs lleva los valores de las dimensiones String (issue #24); una
	// dimensión Real no tiene entrada en strs.
	Upsert(space *core.Space, values []float64, strs map[string]string) error

	// Query devuelve todos los vectores del espacio que satisfacen filter.
	// Si filter es nil se devuelven todos.
	Query(space *core.Space, filter func(values []float64) bool) ([]*core.Vector, error)

	// Delete elimina el vector cuya primera dimensión coincide con id.
	Delete(space *core.Space, id float64) error

	// Close libera los recursos del driver.
	Close() error
}

// Open abre un Backend a partir de un DSN.
// Si dsn está vacío lee la variable de entorno LINLANG_DB.
// Si tampoco está definida, usa memoria.
func Open(dsn string) (Backend, error) {
	if dsn == "" {
		dsn = os.Getenv("LINLANG_DB")
	}
	if dsn == "" {
		dsn = "memory://"
	}

	switch {
	case strings.HasPrefix(dsn, "sqlite://"):
		path := strings.TrimPrefix(dsn, "sqlite://")
		return OpenSQLite(path)

	case strings.HasPrefix(dsn, "redis://"):
		return OpenRedis(dsn)

	case strings.HasPrefix(dsn, "memory://"):
		return NewMemoryStore(), nil

	default:
		return nil, fmt.Errorf("linlang/store: driver no reconocido en DSN '%s'", dsn)
	}
}
