package handler

import (
	"database/sql"
	"encoding/json"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type fsCache struct {
	db *sql.DB
}

func newFsCache(dataDir string) (*fsCache, error) {
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "cache.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS folder_mtimes (
			path  TEXT PRIMARY KEY,
			mtime INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS file_cache (
			path       TEXT PRIMARY KEY,
			parent_dir TEXT NOT NULL,
			data       TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_file_cache_parent ON file_cache(parent_dir);
	`); err != nil {
		db.Close()
		return nil, err
	}

	return &fsCache{db: db}, nil
}

func (c *fsCache) close() error { return c.db.Close() }

func (c *fsCache) getFolderMtime(path string) (int64, bool) {
	var mtime int64
	err := c.db.QueryRow("SELECT mtime FROM folder_mtimes WHERE path = ?", path).Scan(&mtime)
	return mtime, err == nil
}

func (c *fsCache) setFolderMtime(path string, mtime int64) {
	c.db.Exec("INSERT OR REPLACE INTO folder_mtimes (path, mtime) VALUES (?, ?)", path, mtime)
}

func (c *fsCache) invalidateDir(dirPath string) {
	c.db.Exec("DELETE FROM file_cache WHERE parent_dir = ?", dirPath)
}

// getFilesInDir returns all cached files whose immediate parent is dirPath, keyed by full path.
func (c *fsCache) getFilesInDir(dirPath string) map[string]FileData {
	rows, err := c.db.Query("SELECT path, data FROM file_cache WHERE parent_dir = ?", dirPath)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := make(map[string]FileData)
	for rows.Next() {
		var path, dataJSON string
		if err := rows.Scan(&path, &dataJSON); err != nil {
			return nil
		}
		var fd FileData
		if err := json.Unmarshal([]byte(dataJSON), &fd); err != nil {
			return nil
		}
		result[path] = fd
	}
	if rows.Err() != nil {
		return nil
	}
	return result
}

func (c *fsCache) setFile(path, parentDir string, fd FileData) {
	data, err := json.Marshal(fd)
	if err != nil {
		return
	}
	c.db.Exec(
		"INSERT OR REPLACE INTO file_cache (path, parent_dir, data) VALUES (?, ?, ?)",
		path, parentDir, string(data),
	)
}
