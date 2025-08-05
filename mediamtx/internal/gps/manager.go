package gps

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// Config holds the GPS manager configuration.
type Config struct {
	DatabaseHost     string
	DatabasePort     int
	DatabaseUser     string
	DatabasePassword string
	DatabaseName     string
	UpdateInterval   time.Duration
	MaxConnections   int
}

// Manager handles PostgreSQL GPS data retrieval and provides it to overlay systems.
type Manager struct {
	db             *sql.DB
	config         *Config
	mu             sync.RWMutex
	lastUpdate     time.Time
	currentData    *Data
	updateTicker   *time.Ticker
	ctx            context.Context
	cancel         context.CancelFunc
	updateInterval time.Duration
}

// NewManager creates a new GPS manager.
func NewManager(config *Config) (*Manager, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.DatabaseHost,
		config.DatabasePort,
		config.DatabaseUser,
		config.DatabasePassword,
		config.DatabaseName,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(config.MaxConnections)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &Manager{
		db:             db,
		config:         config,
		ctx:            ctx,
		cancel:         cancel,
		updateInterval: config.UpdateInterval,
	}

	// Start background GPS data updates
	go manager.startUpdates()

	return manager, nil
}

// Close closes the GPS manager and its database connection.
func (gm *Manager) Close() error {
	if gm.cancel != nil {
		gm.cancel()
	}
	if gm.updateTicker != nil {
		gm.updateTicker.Stop()
	}
	if gm.db != nil {
		return gm.db.Close()
	}
	return nil
}

// GetCurrentGPS returns the most recent GPS data.
func (gm *Manager) GetCurrentGPS() *Data {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	
	if gm.currentData == nil {
		return &Data{
			Timestamp: time.Now(),
			Latitude:  0.0,
			Longitude: 0.0,
			Status:    "V", // Void (no data)
		}
	}
	
	// Return a copy to avoid race conditions
	dataCopy := *gm.currentData
	return &dataCopy
}

// startUpdates starts the background GPS data update routine.
func (gm *Manager) startUpdates() {
	gm.updateTicker = time.NewTicker(gm.updateInterval)
	defer gm.updateTicker.Stop()

	// Get initial data immediately
	gm.updateGPSData()

	for {
		select {
		case <-gm.ctx.Done():
			return
		case <-gm.updateTicker.C:
			gm.updateGPSData()
		}
	}
}

// updateGPSData fetches the latest GPS data from the database.
func (gm *Manager) updateGPSData() {
	ctx, cancel := context.WithTimeout(gm.ctx, 5*time.Second)
	defer cancel()

	query := `
		SELECT timestamp, latitude, longitude, status 
		FROM gps_data 
		WHERE latitude IS NOT NULL AND longitude IS NOT NULL
		ORDER BY timestamp DESC 
		LIMIT 1
	`

	var data Data
	err := gm.db.QueryRowContext(ctx, query).Scan(
		&data.Timestamp,
		&data.Latitude,
		&data.Longitude,
		&data.Status,
	)

	if err != nil {
		if err != sql.ErrNoRows {
			// Log error but don't stop the update loop
			// TODO: Add proper logging
			return
		}
		// No data available, keep current data or use default
		return
	}

	gm.mu.Lock()
	gm.currentData = &data
	gm.lastUpdate = time.Now()
	gm.mu.Unlock()
}