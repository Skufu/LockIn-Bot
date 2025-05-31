package database

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"time"
)

func ConnectToDatabase(databaseURL string) (*sql.DB, error) {
	// Parse and validate the connection string
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	log.Printf("Connecting to Neon PostgreSQL database at %s...", parsedURL.Host)

	startTime := time.Now()

	// Open database connection
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Clear any cached prepared statements to prevent parameter binding issues
	_, err = db.Exec("DEALLOCATE ALL")
	if err != nil {
		log.Printf("Warning: Failed to clear prepared statements (this is usually safe to ignore): %v", err)
	}

	duration := time.Since(startTime)
	log.Printf("Successfully connected to database in %v", duration)

	return db, nil
}
