package store

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // driver puro Go, sin CGO

	"linlang-go/core"
)

// SQLiteStore almacena cada espacio LinLang como una tabla SQLite.
// La primera dimensión del espacio se convierte en PRIMARY KEY.
// Las columnas se crean automáticamente la primera vez que se persiste
// un vector en ese espacio — el schema .lin define el schema SQL.
//
// DSN: sqlite://path/to/file.db
// Ejemplo: sqlite://linlang.db  (relativo al directorio de trabajo)
type SQLiteStore struct {
	db     *sql.DB
	tables map[string]bool // espacios cuya tabla ya fue creada
}

func OpenSQLite(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("sqlite: ping: %w", err)
	}
	// WAL mode — mejor concurrencia lectores/escritor
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("sqlite: WAL mode: %w", err)
	}
	return &SQLiteStore{db: db, tables: make(map[string]bool)}, nil
}

// ensureTable crea la tabla para el espacio si no existe todavía.
// Columnas: primera dimensión = REAL PRIMARY KEY, resto = REAL NOT NULL DEFAULT 0.
func (s *SQLiteStore) ensureTable(space *core.Space) error {
	if s.tables[space.Name] {
		return nil
	}
	cols := make([]string, len(space.Dimensions))
	for i, d := range space.Dimensions {
		if i == 0 {
			cols[i] = fmt.Sprintf(`"%s" REAL PRIMARY KEY`, d)
		} else {
			cols[i] = fmt.Sprintf(`"%s" REAL NOT NULL DEFAULT 0`, d)
		}
	}
	q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (%s)`,
		space.Name, strings.Join(cols, ", "))
	if _, err := s.db.Exec(q); err != nil {
		return fmt.Errorf("sqlite: create table %s: %w", space.Name, err)
	}
	s.tables[space.Name] = true
	return nil
}

func (s *SQLiteStore) Upsert(space *core.Space, values []float64) error {
	if err := s.ensureTable(space); err != nil {
		return err
	}

	// Construir: INSERT INTO "Space" (d0,d1,...) VALUES (?,?,...)
	//            ON CONFLICT(d0) DO UPDATE SET d1=excluded.d1, ...
	quotedCols := make([]string, len(space.Dimensions))
	placeholders := make([]string, len(space.Dimensions))
	args := make([]any, len(values))
	for i, d := range space.Dimensions {
		quotedCols[i] = fmt.Sprintf(`"%s"`, d)
		placeholders[i] = "?"
		args[i] = values[i]
	}

	var updateSets []string
	for _, d := range space.Dimensions[1:] {
		updateSets = append(updateSets, fmt.Sprintf(`"%s"=excluded."%s"`, d, d))
	}

	q := fmt.Sprintf(
		`INSERT INTO "%s" (%s) VALUES (%s) ON CONFLICT("%s") DO UPDATE SET %s`,
		space.Name,
		strings.Join(quotedCols, ","),
		strings.Join(placeholders, ","),
		space.Dimensions[0],
		strings.Join(updateSets, ","),
	)
	_, err := s.db.Exec(q, args...)
	return err
}

func (s *SQLiteStore) Query(space *core.Space, filter func([]float64) bool) ([]*core.Vector, error) {
	if err := s.ensureTable(space); err != nil {
		return nil, err
	}

	quotedCols := make([]string, len(space.Dimensions))
	for i, d := range space.Dimensions {
		quotedCols[i] = fmt.Sprintf(`"%s"`, d)
	}
	q := fmt.Sprintf(`SELECT %s FROM "%s"`, strings.Join(quotedCols, ","), space.Name)

	rows, err := s.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query %s: %w", space.Name, err)
	}
	defer rows.Close()

	scanDst := make([]any, len(space.Dimensions))
	rawVals := make([]float64, len(space.Dimensions))
	for i := range scanDst {
		scanDst[i] = &rawVals[i]
	}

	var result []*core.Vector
	for rows.Next() {
		if err := rows.Scan(scanDst...); err != nil {
			return nil, fmt.Errorf("sqlite: scan: %w", err)
		}
		vals := make([]float64, len(rawVals))
		copy(vals, rawVals)
		if filter == nil || filter(vals) {
			result = append(result, core.NewVector(space, vals))
		}
	}
	return result, rows.Err()
}

func (s *SQLiteStore) Delete(space *core.Space, id float64) error {
	if err := s.ensureTable(space); err != nil {
		return err
	}
	q := fmt.Sprintf(`DELETE FROM "%s" WHERE "%s" = ?`, space.Name, space.Dimensions[0])
	_, err := s.db.Exec(q, id)
	return err
}

func (s *SQLiteStore) Close() error { return s.db.Close() }
