package conf

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Database handles SQLite operations for configuration storage
type Database struct {
	db *sql.DB
}

// NewDatabase creates a new database connection
func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// Create paths table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS paths (
			name TEXT PRIMARY KEY,
			config TEXT NOT NULL
		)
	`)
	if err != nil {
		return nil, err
	}

	return &Database{db: db}, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// SavePath saves or updates a path configuration
func (d *Database) SavePath(name string, path *Path) error {
	configJSON, err := json.Marshal(path)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(`
		INSERT OR REPLACE INTO paths (name, config)
		VALUES (?, ?)
	`, name, string(configJSON))
	return err
}

// LoadPath loads a single path configuration
func (d *Database) LoadPath(name string) (*Path, error) {
	var configJSON string
	err := d.db.QueryRow("SELECT config FROM paths WHERE name = ?", name).Scan(&configJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrPathNotFound
		}
		return nil, err
	}

	var path Path
	err = json.Unmarshal([]byte(configJSON), &path)
	if err != nil {
		return nil, err
	}

	return &path, nil
}

// LoadAllPaths loads all path configurations
func (d *Database) LoadAllPaths() (map[string]*Path, error) {
	rows, err := d.db.Query("SELECT name, config FROM paths")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := make(map[string]*Path)
	for rows.Next() {
		var name, configJSON string
		err := rows.Scan(&name, &configJSON)
		if err != nil {
			return nil, err
		}

		var path Path
		err = json.Unmarshal([]byte(configJSON), &path)
		if err != nil {
			return nil, err
		}

		paths[name] = &path
	}

	return paths, nil
}

// DeletePath deletes a path configuration
func (d *Database) DeletePath(name string) error {
	result, err := d.db.Exec("DELETE FROM paths WHERE name = ?", name)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrPathNotFound
	}

	return nil
}

// LoadFromDatabase loads configuration from database
func (conf *Conf) LoadFromDatabase(dbPath string) error {
	db, err := NewDatabase(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	paths, err := db.LoadAllPaths()
	if err != nil {
		return err
	}

	conf.Paths = paths
	return nil
}

// loadPathsFromDatabase loads only paths from database without replacing entire config
func loadPathsFromDatabase(dbPath string) (map[string]*Path, error) {
	db, err := NewDatabase(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	return db.LoadAllPaths()
}

// SaveToDatabase saves configuration to database
func (conf *Conf) SaveToDatabase(dbPath string) error {
	db, err := NewDatabase(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	// Save all paths
	for name, path := range conf.Paths {
		err = db.SavePath(name, path)
		if err != nil {
			return fmt.Errorf("failed to save path %s: %w", name, err)
		}
	}

	return nil
}