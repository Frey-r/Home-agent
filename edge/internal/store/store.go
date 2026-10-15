// Package store gestiona la base SQLite local de edge-core (ADR-0006):
// apertura con pragmas (WAL, busy_timeout, foreign keys) y migraciones
// embebidas.
package store

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
)

// ErrNotFound indica que la clave o registro solicitado no existe.
var ErrNotFound = errors.New("store: no encontrado")

// Store es la base de datos local de edge-core.
type Store struct {
	db *sql.DB
}

// Open abre (o crea) la base en path y aplica las migraciones pendientes.
func Open(ctx context.Context, path string) (*Store, error) {
	return nil, errors.New("store: no implementado")
}

// Close cierra la base.
func (s *Store) Close() error {
	return errors.New("store: no implementado")
}

// SetMeta guarda un par clave/valor de metadatos.
func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	return errors.New("store: no implementado")
}

// GetMeta lee un metadato; devuelve ErrNotFound si no existe.
func (s *Store) GetMeta(ctx context.Context, key string) (string, error) {
	return "", errors.New("store: no implementado")
}

func openDB(path string) (*sql.DB, error) {
	return nil, errors.New("store: no implementado")
}

func migrate(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	return errors.New("store: no implementado")
}
