package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/bot"
	"github.com/Skufu/LockIn-Bot/internal/config"
	"github.com/Skufu/LockIn-Bot/internal/database"
)

func main() {
	// Load configuration
	log.Println("Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	log.Printf("Connecting to Neon PostgreSQL database at %s...", cfg.DBHost)
	startTime := time.Now()

	db, err := database.Connect(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	log.Printf("Successfully connected to database in %v", time.Since(startTime))

	// Run migrations
	log.Println("Running database migrations...")
	err = db.MigrateUp("db/migrations")
	if err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations completed successfully")

	// Create and start the bot
	log.Println("Initializing Discord bot...")
	discordBot, err := bot.New(cfg.DiscordToken, db.Querier, cfg.LoggingChannelID, cfg.TestGuildID, cfg.AllowedVoiceChannelIDsMap)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Create and start the scheduler
	scheduler := bot.NewScheduler(discordBot)
	scheduler.Start()

	// Wait for a CTRL-C
	log.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Stop the scheduler and close Discord session
	log.Println("Shutting down...")
	scheduler.Stop()
	discordBot.Close()
	log.Println("Shutdown complete. Goodbye!")
}
