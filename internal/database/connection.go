package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq" // Import PostgreSQL driver
	"github.com/pressly/goose/v3"
)

// Connection contains the database connection and querier
type Connection struct {
	db      *sql.DB
	Querier *Queries
}

// Connect establishes a connection to the database
func Connect(host, port, user, password, dbname string) (*Connection, error) {
	// For Neon PostgreSQL, SSL should be enabled
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Verify connection works
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Clear any cached prepared statements to prevent parameter binding issues
	_, err = db.Exec("DEALLOCATE ALL")
	if err != nil {
		// This is usually safe to ignore on new connections
		fmt.Printf("Warning: Failed to clear prepared statements (this is usually safe to ignore on new connections): %v\n", err)
	}

	// Force a fresh connection to clear any PostgreSQL statement cache issues
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed to verify database connection after cleanup: %w", err)
	}

	// Configure connection pool settings for Neon
	db.SetMaxOpenConns(25) // Limit open connections for serverless
	db.SetMaxIdleConns(5)  // Keep some connections ready to go

	return &Connection{
		db:      db,
		Querier: New(db),
	}, nil
}

// MigrateUp runs all migrations that haven't been applied yet
func (c *Connection) MigrateUp(migrationsDir string) error {
	goose.SetBaseFS(nil)
	err := goose.SetDialect("postgres")
	if err != nil {
		return fmt.Errorf("failed to set dialect: %w", err)
	}

	err = goose.Up(c.db, migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// Close closes the database connection
func (c *Connection) Close() error {
	return c.db.Close()
}
