package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"k8/internal/api"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS nodes (
	id   TEXT PRIMARY KEY,
	data BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS services (
	name TEXT PRIMARY KEY,
	data BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS jobs (
	name TEXT PRIMARY KEY,
	data BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS assignments (
	id         TEXT PRIMARY KEY,
	node_id    TEXT NOT NULL,
	owner_kind TEXT NOT NULL,
	owner_name TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	data       BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS assignments_by_owner ON assignments (owner_kind, owner_name);
CREATE INDEX IF NOT EXISTS assignments_by_node ON assignments (node_id);
`

// SQLiteStore persists state in a single SQLite file, the same trade k3s makes
// with kine: one server, no etcd. Objects are stored as JSON with the few
// columns we query on pulled out and indexed.
//
// It fails stop. Store methods have no error returns, so a database error
// crashes the controller rather than letting it carry on believing a write
// happened. The supervisor restarts it and state is reloaded from the last
// committed transaction.
type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLite(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dsn := "file:" + path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(FULL)" +
		"&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection serializes every read-modify-write, so transactions
	// never interleave. The controller's write rate is a few per second.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	var check string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		db.Close()
		return nil, fmt.Errorf("database %s failed integrity check: %v %s", path, err, check)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Nodes

func (s *SQLiteStore) UpsertNode(node api.Node) {
	s.exec("upsert node", `INSERT INTO nodes (id, data) VALUES (?, ?)
		ON CONFLICT (id) DO UPDATE SET data = excluded.data`, node.ID, encode(node))
}

func (s *SQLiteStore) TouchNode(id string, now time.Time) bool {
	return update(s, "touch node", "nodes", "id", id, func(node *api.Node) {
		node.LastHeartbeat = now
	})
}

func (s *SQLiteStore) ListNodes() []api.Node {
	return list[api.Node](s, "list nodes", `SELECT data FROM nodes ORDER BY id`)
}

// Services

func (s *SQLiteStore) UpsertService(service api.Service) {
	s.exec("upsert service", `INSERT INTO services (name, data) VALUES (?, ?)
		ON CONFLICT (name) DO UPDATE SET data = excluded.data`, service.Name, encode(service))
}

func (s *SQLiteStore) UpdateServiceStatus(name string, status api.ServiceStatus) bool {
	return update(s, "update service status", "services", "name", name, func(service *api.Service) {
		service.Status = status
	})
}

func (s *SQLiteStore) ListServices() []api.Service {
	return list[api.Service](s, "list services", `SELECT data FROM services ORDER BY name`)
}

func (s *SQLiteStore) DeleteService(name string) bool {
	return s.deleteOwner("delete service", "services", api.WorkloadKindService, name)
}

// Jobs

func (s *SQLiteStore) UpsertJob(job api.Job) {
	s.exec("upsert job", `INSERT INTO jobs (name, data) VALUES (?, ?)
		ON CONFLICT (name) DO UPDATE SET data = excluded.data`, job.Name, encode(job))
}

func (s *SQLiteStore) UpdateJobStatus(name string, status api.JobStatus) bool {
	return update(s, "update job status", "jobs", "name", name, func(job *api.Job) {
		job.Status = status
	})
}

func (s *SQLiteStore) ListJobs() []api.Job {
	return list[api.Job](s, "list jobs", `SELECT data FROM jobs ORDER BY name`)
}

func (s *SQLiteStore) DeleteJob(name string) bool {
	return s.deleteOwner("delete job", "jobs", api.WorkloadKindJob, name)
}

// Assignments

func (s *SQLiteStore) SaveAssignment(assignment api.Assignment) {
	s.exec("save assignment", `INSERT INTO assignments (id, node_id, owner_kind, owner_name, created_at, data)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			node_id = excluded.node_id,
			owner_kind = excluded.owner_kind,
			owner_name = excluded.owner_name,
			created_at = excluded.created_at,
			data = excluded.data`,
		assignment.ID, assignment.NodeID, string(assignment.OwnerKind), assignment.OwnerName,
		assignment.CreatedAt.UnixNano(), encode(assignment))
}

func (s *SQLiteStore) DeleteAssignment(id string) bool {
	result, err := s.db.Exec(`DELETE FROM assignments WHERE id = ?`, id)
	failOn("delete assignment", err)
	return rowsAffected("delete assignment", result) > 0
}

func (s *SQLiteStore) UpdateAssignmentStatus(id string, phase api.AssignmentPhase, message string, now time.Time) (api.Assignment, bool) {
	var updated api.Assignment
	found := update(s, "update assignment status", "assignments", "id", id, func(assignment *api.Assignment) {
		applyStatus(assignment, phase, message, now)
		updated = *assignment
	})
	return updated, found
}

// Assignments are ordered by creation time then ID, like MemoryStore.
func (s *SQLiteStore) ListAssignments() []api.Assignment {
	return list[api.Assignment](s, "list assignments",
		`SELECT data FROM assignments ORDER BY created_at, id`)
}

func (s *SQLiteStore) ListNodeAssignments(nodeID string) []api.Assignment {
	return list[api.Assignment](s, "list node assignments",
		`SELECT data FROM assignments WHERE node_id = ? ORDER BY created_at, id`, nodeID)
}

func (s *SQLiteStore) ListAssignmentsForOwner(kind api.WorkloadKind, name string) []api.Assignment {
	return list[api.Assignment](s, "list owner assignments",
		`SELECT data FROM assignments WHERE owner_kind = ? AND owner_name = ? ORDER BY created_at, id`,
		string(kind), name)
}

func (s *SQLiteStore) HasAssignment(id string) bool {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM assignments WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	failOn("has assignment", err)
	return true
}

// Snapshot reads everything in one transaction so the view is consistent.
func (s *SQLiteStore) Snapshot() api.StateSnapshot {
	tx, err := s.db.Begin()
	failOn("begin snapshot", err)
	defer tx.Rollback()

	snapshot := api.StateSnapshot{
		Nodes:       list[api.Node](tx, "snapshot nodes", `SELECT data FROM nodes ORDER BY id`),
		Services:    list[api.Service](tx, "snapshot services", `SELECT data FROM services ORDER BY name`),
		Jobs:        list[api.Job](tx, "snapshot jobs", `SELECT data FROM jobs ORDER BY name`),
		Assignments: list[api.Assignment](tx, "snapshot assignments", `SELECT data FROM assignments ORDER BY created_at, id`),
	}
	failOn("commit snapshot", tx.Commit())
	return snapshot
}

// helpers

type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func (s *SQLiteStore) Query(query string, args ...any) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}

func list[T any](q querier, op, query string, args ...any) []T {
	rows, err := q.Query(query, args...)
	failOn(op, err)
	defer rows.Close()

	out := make([]T, 0)
	for rows.Next() {
		var data []byte
		failOn(op, rows.Scan(&data))
		var item T
		failOn(op, json.Unmarshal(data, &item))
		out = append(out, item)
	}
	failOn(op, rows.Err())
	return out
}

// update loads one JSON row, applies mutate and writes it back in a single
// transaction. It reports whether the row existed.
func update[T any](s *SQLiteStore, op, table, keyColumn, key string, mutate func(*T)) bool {
	tx, err := s.db.Begin()
	failOn(op, err)
	defer tx.Rollback()

	var data []byte
	err = tx.QueryRow(`SELECT data FROM `+table+` WHERE `+keyColumn+` = ?`, key).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	failOn(op, err)

	var item T
	failOn(op, json.Unmarshal(data, &item))
	mutate(&item)
	_, err = tx.Exec(`UPDATE `+table+` SET data = ? WHERE `+keyColumn+` = ?`, encode(item), key)
	failOn(op, err)
	failOn(op, tx.Commit())
	return true
}

func (s *SQLiteStore) deleteOwner(op, table string, kind api.WorkloadKind, name string) bool {
	tx, err := s.db.Begin()
	failOn(op, err)
	defer tx.Rollback()

	result, err := tx.Exec(`DELETE FROM `+table+` WHERE name = ?`, name)
	failOn(op, err)
	if rowsAffected(op, result) == 0 {
		return false
	}
	_, err = tx.Exec(`DELETE FROM assignments WHERE owner_kind = ? AND owner_name = ?`, string(kind), name)
	failOn(op, err)
	failOn(op, tx.Commit())
	return true
}

func (s *SQLiteStore) exec(op, query string, args ...any) {
	_, err := s.db.Exec(query, args...)
	failOn(op, err)
}

func rowsAffected(op string, result sql.Result) int64 {
	n, err := result.RowsAffected()
	failOn(op, err)
	return n
}

func encode(v any) []byte {
	data, err := json.Marshal(v)
	failOn("encode", err)
	return data
}

func failOn(op string, err error) {
	if err != nil {
		log.Fatalf("store: %s: %v", op, err)
	}
}
