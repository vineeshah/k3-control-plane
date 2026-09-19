package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const backupPattern = "state-*.db"

// Backup writes a consistent snapshot of the live database into dir (the NAS
// mount) and keeps only the newest keep snapshots.
//
// The database itself stays on local disk: SQLite's locking is not reliable
// over NFS/SMB. VACUUM INTO produces a transactionally consistent copy
// without stopping writers; it is written under a temp name, fsynced and
// renamed so a half-written file is never mistaken for a snapshot.
func (s *SQLiteStore) Backup(dir string, keep int, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	final := filepath.Join(dir, "state-"+now.UTC().Format("20060102T150405Z")+".db")
	tmp := final + ".tmp"
	_ = os.Remove(tmp)

	if _, err := s.db.Exec(`VACUUM INTO ?`, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("vacuum into %s: %w", tmp, err)
	}
	if err := syncFile(tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("publish backup: %w", err)
	}
	if err := pruneBackups(dir, keep); err != nil {
		return final, err
	}
	return final, nil
}

// LatestBackup returns the newest snapshot in dir, or "" if there is none.
func LatestBackup(dir string) (string, error) {
	backups, err := listBackups(dir)
	if err != nil || len(backups) == 0 {
		return "", err
	}
	return backups[len(backups)-1], nil
}

// RestoreIfMissing copies the newest snapshot from backupDir to dbPath when
// dbPath does not exist, so a controller whose disk was lost comes back with
// the last backed-up state instead of an empty cluster.
func RestoreIfMissing(dbPath, backupDir string) (string, error) {
	if _, err := os.Stat(dbPath); err == nil || !os.IsNotExist(err) {
		return "", err
	}
	latest, err := LatestBackup(backupDir)
	if err != nil || latest == "" {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return "", err
	}
	data, err := os.ReadFile(latest)
	if err != nil {
		return "", fmt.Errorf("read backup: %w", err)
	}
	tmp := dbPath + ".restore"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", fmt.Errorf("write restored db: %w", err)
	}
	if err := syncFile(tmp); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dbPath); err != nil {
		return "", fmt.Errorf("publish restored db: %w", err)
	}
	return latest, nil
}

func listBackups(dir string) ([]string, error) {
	backups, err := filepath.Glob(filepath.Join(dir, backupPattern))
	if err != nil {
		return nil, err
	}
	// Timestamped names sort chronologically.
	sort.Strings(backups)
	return backups, nil
}

func pruneBackups(dir string, keep int) error {
	if keep <= 0 {
		return nil
	}
	backups, err := listBackups(dir)
	if err != nil {
		return err
	}
	for len(backups) > keep {
		if err := os.Remove(backups[0]); err != nil {
			return fmt.Errorf("prune %s: %w", backups[0], err)
		}
		backups = backups[1:]
	}
	return nil
}

func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync %s: %w", path, err)
	}
	return nil
}
