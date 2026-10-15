package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"testing/fstest"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close() error: %v", err)
		}
	})
	return s
}

func appliedVersions(t *testing.T, db *sql.DB) []int {
	t.Helper()
	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("consultando schema_migrations: %v", err)
	}
	defer rows.Close()
	var vs []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		vs = append(vs, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return vs
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestOpen_AppliesPragmas(t *testing.T) {
	s := openTemp(t)

	tests := []struct {
		pragma string
		want   string
	}{
		{"journal_mode", "wal"},
		{"foreign_keys", "1"},
		{"busy_timeout", "5000"},
		{"synchronous", "1"}, // NORMAL
	}
	for _, tt := range tests {
		t.Run(tt.pragma, func(t *testing.T) {
			var got string
			if err := s.db.QueryRow("PRAGMA " + tt.pragma).Scan(&got); err != nil {
				t.Fatalf("PRAGMA %s: %v", tt.pragma, err)
			}
			if got != tt.want {
				t.Fatalf("PRAGMA %s = %q, want %q", tt.pragma, got, tt.want)
			}
		})
	}
}

func TestMigrate(t *testing.T) {
	m1 := &fstest.MapFile{Data: []byte(`CREATE TABLE a (id INTEGER PRIMARY KEY);`)}
	m2 := &fstest.MapFile{Data: []byte(`ALTER TABLE a ADD COLUMN x TEXT;`)} // falla si se aplica antes que 0001
	bad := &fstest.MapFile{Data: []byte(`CREATE TABLE c (id INTEGER); INSERT INTO tabla_inexistente VALUES (1);`)}

	tests := []struct {
		name         string
		runs         []fstest.MapFS // cada elemento es una ejecución sucesiva de migrate
		wantErrOnRun int            // índice de la ejecución que debe fallar; -1 = ninguna
		wantVersions []int
		wantTables   []string
		absentTables []string
	}{
		{
			name:         "aplica en orden numérico",
			runs:         []fstest.MapFS{{"0002_add_x.sql": m2, "0001_a.sql": m1}},
			wantErrOnRun: -1,
			wantVersions: []int{1, 2},
			wantTables:   []string{"a"},
		},
		{
			name:         "idempotente",
			runs:         []fstest.MapFS{{"0001_a.sql": m1, "0002_add_x.sql": m2}, {"0001_a.sql": m1, "0002_add_x.sql": m2}},
			wantErrOnRun: -1,
			wantVersions: []int{1, 2},
			wantTables:   []string{"a"},
		},
		{
			name:         "aplica solo pendientes",
			runs:         []fstest.MapFS{{"0001_a.sql": m1}, {"0001_a.sql": m1, "0002_add_x.sql": m2}},
			wantErrOnRun: -1,
			wantVersions: []int{1, 2},
			wantTables:   []string{"a"},
		},
		{
			name:         "migración fallida hace rollback",
			runs:         []fstest.MapFS{{"0001_a.sql": m1, "0002_bad.sql": bad}},
			wantErrOnRun: 0,
			wantVersions: []int{1},
			wantTables:   []string{"a"},
			absentTables: []string{"c"},
		},
		{
			name:         "nombre de archivo inválido",
			runs:         []fstest.MapFS{{"init.sql": m1}},
			wantErrOnRun: 0,
			wantVersions: nil,
		},
		{
			name:         "versión duplicada",
			runs:         []fstest.MapFS{{"0001_a.sql": m1, "0001_b.sql": m2}},
			wantErrOnRun: 0,
			wantVersions: nil,
		},
		{
			name:         "ignora archivos que no son .sql",
			runs:         []fstest.MapFS{{"0001_a.sql": m1, "README.md": &fstest.MapFile{Data: []byte("x")}}},
			wantErrOnRun: -1,
			wantVersions: []int{1},
			wantTables:   []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := openDB(filepath.Join(t.TempDir(), "m.db"))
			if err != nil {
				t.Fatalf("openDB: %v", err)
			}
			t.Cleanup(func() { db.Close() })

			for i, fsys := range tt.runs {
				err := migrate(context.Background(), db, fsys)
				if i == tt.wantErrOnRun {
					if err == nil {
						t.Fatalf("ejecución %d: se esperaba error", i)
					}
					continue
				}
				if err != nil {
					t.Fatalf("ejecución %d: error inesperado: %v", i, err)
				}
			}

			if got := appliedVersions(t, db); !reflect.DeepEqual(got, tt.wantVersions) {
				t.Fatalf("versiones = %v, want %v", got, tt.wantVersions)
			}
			for _, tbl := range tt.wantTables {
				if !tableExists(t, db, tbl) {
					t.Errorf("tabla %q debería existir", tbl)
				}
			}
			for _, tbl := range tt.absentTables {
				if tableExists(t, db, tbl) {
					t.Errorf("tabla %q no debería existir (rollback)", tbl)
				}
			}
		})
	}
}

func TestOpen_EmbeddedMigrations(t *testing.T) {
	s := openTemp(t)
	if vs := appliedVersions(t, s.db); len(vs) == 0 || vs[0] != 1 {
		t.Fatalf("versiones aplicadas = %v, se esperaba al menos la 0001", vs)
	}
	if !tableExists(t, s.db, "meta") {
		t.Fatal("la tabla meta debería existir tras Open")
	}
}

func TestMeta_SetGet(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		sets    [][2]string
		get     string
		want    string
		wantErr error
	}{
		{name: "ida y vuelta", sets: [][2]string{{"device_id", "abc"}}, get: "device_id", want: "abc"},
		{name: "sobrescribe", sets: [][2]string{{"k", "v1"}, {"k", "v2"}}, get: "k", want: "v2"},
		{name: "clave inexistente", get: "nope", wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openTemp(t)
			for _, kv := range tt.sets {
				if err := s.SetMeta(ctx, kv[0], kv[1]); err != nil {
					t.Fatalf("SetMeta(%q): %v", kv[0], err)
				}
			}
			got, err := s.GetMeta(ctx, tt.get)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("GetMeta() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetMeta() error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("GetMeta() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStore_ConcurrentWrites(t *testing.T) {
	const workers, perWorker = 20, 25
	ctx := context.Background()
	s := openTemp(t)

	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perWorker {
				if err := s.SetMeta(ctx, fmt.Sprintf("w%d-%d", w, i), "v"); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("escritura concurrente falló: %v", err)
	}

	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM meta WHERE key LIKE 'w%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != workers*perWorker {
		t.Fatalf("filas = %d, want %d", n, workers*perWorker)
	}
}
