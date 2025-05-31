package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/bot"
	"github.com/Skufu/LockIn-Bot/internal/config"
	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/Skufu/LockIn-Bot/internal/service"
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
	discordBot, err := bot.New(cfg.DiscordToken, db.Querier, cfg, cfg.AllowedVoiceChannelIDsMap)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Initialize StreakService
	log.Println("Initializing Streak Service...")
	streakService := service.NewStreakService(db.Querier, discordBot.Session(), cfg)

	// SET the StreakService on the Bot instance
	discordBot.SetStreakService(streakService)

	// SET the Bot reference on StreakService to access session timing
	streakService.SetBot(discordBot)

	// Start StreakService scheduled tasks (can be after setting it on the bot)
	streakService.StartScheduledTasks()

	// Start a simple HTTP server for health checks in a goroutine
	go func() {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8000" // Default port if not set by Render (Render usually sets PORT)
			log.Printf("Defaulting to port %s for health check server (PORT env var not set)", port)
		} else {
			log.Printf("Attempting to start health check server on port %s (from PORT env var)", port)
		}

		// Add multiple health check endpoints
		http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			log.Printf("Health check request received: %s %s", r.Method, r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy","service":"lockin-bot"}`))
		})

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			log.Printf("Root request received: %s %s", r.Method, r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy","service":"lockin-bot","message":"LockIn Bot is running"}`))
		})

		log.Printf("Health check server attempting to listen on :%s", port)
		log.Printf("Health check endpoints available: /healthz, /health, /")

		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatalf("Error starting health check server: %v", err)
		}
	}()

	// Create and start the scheduler for existing bot tasks (e.g., study session resets)
	scheduler := bot.NewScheduler(discordBot)
	scheduler.Start()

	// Wait for a CTRL-C
	log.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Stop the schedulers and close Discord session
	log.Println("Shutting down...")
	scheduler.Stop()
	streakService.StopScheduledTasks()
	discordBot.Close()
	log.Println("Shutdown complete. Goodbye!")
}
