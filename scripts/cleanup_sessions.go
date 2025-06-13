package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/config"
	"github.com/Skufu/LockIn-Bot/internal/database"
)

func main() {
	fmt.Println("🧹 LockIn-Bot Study Session Cleanup Tool")
	fmt.Println("========================================")

	// Load configuration
	fmt.Println("Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	fmt.Printf("Connecting to database at %s...\n", cfg.DBHost)
	db, err := database.Connect(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Count current sessions
	fmt.Println("Counting current study sessions...")
	// Using DeleteOldStudySessions with future date to delete all
	futureDate := time.Now().AddDate(1, 0, 0) // 1 year in future

	fmt.Printf("Deleting all study sessions (using cutoff date: %s)...\n", futureDate.Format("2006-01-02"))

	err = db.Querier.DeleteOldStudySessions(ctx, futureDate)
	if err != nil {
		log.Fatalf("Failed to delete study sessions: %v", err)
	}

	fmt.Println("✅ Successfully deleted all study sessions!")
	fmt.Println("📊 User statistics remain intact in user_stats table")
	fmt.Println("🔄 New sessions will start tracking from now")

	// Ask user if they want to also reset weekly cleanup schedule
	fmt.Println("\n💡 Note: The bot is now configured to delete sessions older than 1 week automatically.")
	fmt.Println("   This will prevent future storage buildup.")
}
