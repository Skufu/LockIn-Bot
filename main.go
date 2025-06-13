package main

import (
	"fmt"
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

	// Clear any remaining prepared statement cache issues after migrations
	log.Println("Clearing prepared statement cache...")
	// Note: We'll rely on the connection.go clearing which is safer

	const maxBotInitAttempts = 10
	const initialBackoff = 5 * time.Second

	var discordBot *bot.Bot
	for attempt := 1; attempt <= maxBotInitAttempts; attempt++ {
		log.Printf("Initializing Discord bot (attempt %d/%d)...", attempt, maxBotInitAttempts)

		var createErr error
		// Wrap in func to enable defer recover per attempt
		func() {
			defer func() {
				if r := recover(); r != nil {
					createErr = fmt.Errorf("panic while creating bot: %v", r)
					log.Printf("Bot creation panic recovered: %v", r)
				}
			}()
			// Add small delay to let database settle after migrations
			if attempt > 1 {
				time.Sleep(1 * time.Second)
			}
			discordBot, createErr = bot.New(cfg.DiscordToken, db.Querier, cfg, cfg.AllowedVoiceChannelIDsMap)
		}()

		if createErr == nil {
			log.Printf("Discord bot initialized successfully on attempt %d", attempt)
			break
		}

		// If not last attempt, backoff and retry
		if attempt < maxBotInitAttempts {
			wait := time.Duration(attempt*attempt) * initialBackoff
			log.Printf("Failed to initialize bot (attempt %d/%d): %v. Retrying in %s...", attempt, maxBotInitAttempts, createErr, wait)
			time.Sleep(wait)
			continue
		}

		// Out of attempts -> fatal
		log.Fatalf("Failed to create bot after %d attempts: %v", maxBotInitAttempts, createErr)
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
