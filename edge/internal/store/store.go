// Package store gestiona la base SQLite local de edge-core (ADR-0006):
// apertura con pragmas (WAL, busy_timeout, foreign keys) y migraciones
// embebidas.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	_ "modernc.org/sqlite" // driver "sqlite" (Go puro, sin CGO)
)

// ErrNotFound indica que la clave o registro solicitado no existe.
var ErrNotFound = errors.New("store: no encontrado")

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// dsnParams se aplican en cada conexión del pool (ADR-0006).
const dsnParams = "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1&_synchronous=NORMAL&_txlock=immediate"

// Store es la base de datos local de edge-core.
type Store struct {
	db *sql.DB
}

// Open abre (o crea) la base en path y aplica las migraciones pendientes.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	migrations, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migraciones embebidas: %w", err)
	}
	if err := migrate(ctx, db, migrations); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close cierra la base.
func (s *Store) Close() error {
	return s.db.Close()
}

// SetMeta guarda un par clave/valor de metadatos.
func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`, key, value)
	if err != nil {
		return fmt.Errorf("store: set meta %q: %w", key, err)
	}
	return nil
}

// GetMeta lee un metadato; devuelve ErrNotFound si no existe.
func (s *Store) GetMeta(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("store: meta %q: %w", key, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("store: get meta %q: %w", key, err)
	}
	return v, nil
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+dsnParams)
	if err != nil {
		return nil, fmt.Errorf("store: abrir %s: %w", path, err)
	}
	// sql.Open es perezoso: Ping fuerza la primera conexión y valida pragmas.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: abrir %s: %w", path, err)
	}
	return db, nil
}
