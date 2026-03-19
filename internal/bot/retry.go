package bot

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/config"
	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/bwmarrin/discordgo"
)

// Initialize randomness once during package initialization
func init() {
	rand.Seed(time.Now().UnixNano())
}

// connectWithRetry creates a new Bot instance with retry logic and exponential backoff
func connectWithRetry(token string, db *database.Queries, cfg *config.Config, allowedVCs map[string]struct{}, maxRetries int) (*Bot, error) {
	// Initial validation of token format before attempting any connections
	if err := validateToken(token); err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	var lastErr error
	baseDelay := time.Second
	maxDelay := 60 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("Attempt %d/%d to connect to Discord", attempt, maxRetries)

		// Create a new Discord session
		dg, err := discordgo.New("Bot " + token)
		if err != nil {
			classifiedErr := classifyStartupError(err)

			if classifiedErr.Type == ErrorTypePermanent {
				log.Printf("[PERMANENT ERROR] Failed to initialize Discord session on attempt %d: %v", attempt, classifiedErr)
				return nil, classifiedErr
			}

			lastErr = err
			log.Printf("[RETRYABLE ERROR] Session creation failed on attempt %d: %v (classification: %s)",
				attempt, classifiedErr, classifiedErr.getTypeString())

			if attempt == maxRetries {
				break
			}

			nextDelay := calculateBackoffWithJitter(baseDelay, attempt, maxDelay)
			log.Printf("Waiting %v before retry attempt %d/%d due to error: %v",
				nextDelay, attempt+1, maxRetries, classifiedErr.Message)

			time.Sleep(nextDelay)
			continue
		}

		// Attempt to open the session connection
		err = dg.Open()
		if err != nil {
			// Clean up the session since opening failed
			dg.Close()

			classifiedErr := classifyStartupError(err)

			if classifiedErr.Type == ErrorTypePermanent {
				log.Printf("[PERMANENT ERROR] Failed to open Discord connection on attempt %d: %v", attempt, classifiedErr)
				return nil, classifiedErr
			}

			lastErr = err
			log.Printf("[RETRYABLE ERROR] Connection failed on attempt %d: %v (classification: %s)",
				attempt, classifiedErr, classifiedErr.getTypeString())

			if attempt == maxRetries {
				break
			}

			nextDelay := calculateBackoffWithJitter(baseDelay, attempt, maxDelay)
			log.Printf("Waiting %v before retry attempt %d/%d due to error: %v",
				nextDelay, attempt+1, maxRetries, classifiedErr.Message)

			time.Sleep(nextDelay)
			continue
		}

		// Successfully connected - create the bot instance with all handlers registered
		bot := &Bot{
			session:                dg,
			db:                     db,
			activeSessions:         make(map[string]time.Time),
			LoggingChannelID:       cfg.LoggingChannelID,
			testGuildID:            cfg.TestGuildID,
			allowedVoiceChannelIDs: allowedVCs,
			cfg:                    cfg,
			streakService:          nil, // Will be set later by the main application
			achievementService:     nil, // Will be set later by the main application
			voiceEventChan:         make(chan func()),
			shutdownChan:           make(chan struct{}),
			lastVoiceEvent:         make(map[string]time.Time),
			voiceEventMu:           sync.Mutex{},
		}

		// Register handlers just like in New constructor
		dg.AddHandler(bot.handleReady)
		dg.AddHandler(bot.handleVoiceStateUpdate)
		dg.AddHandler(bot.handleInteractionCreate)

		// Set intents
		dg.Identify.Intents = discordgo.IntentsGuildVoiceStates | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

		// Start worker pool for voice events (prevents goroutine explosion)
		go bot.voiceEventWorker()

		// Start session timeout checker to prevent phantom sessions
		bot.StartSessionTimeoutChecker()

		// At this point the bot is fully operational with registered handlers
		log.Printf("Successfully connected to Discord on attempt %d", attempt)
		return bot, nil
	}

	// If we reach here, all retry attempts have failed
	finalClassifiedErr := classifyStartupError(lastErr)
	log.Printf("Final failure after %d attempts. Last error (classification: %s): %v",
		maxRetries, finalClassifiedErr.getTypeString(), finalClassifiedErr)

	return nil, fmt.Errorf("failed to connect to Discord after %d attempts: %w", maxRetries, finalClassifiedErr)
}

// calculateBackoffWithJitter calculates the next delay using exponential backoff with jitter
func calculateBackoffWithJitter(baseDelay time.Duration, attempt int, maxDelay time.Duration) time.Duration {
	// Calculate exponential backoff: baseDelay * 2^(attempt-1)
	expMultiplier := 1 << uint(attempt-1)
	expDelay := baseDelay * time.Duration(expMultiplier)

	// Cap at maximum delay
	if expDelay > maxDelay {
		expDelay = maxDelay
	}

	// Add jitter: 0-25% of the current delay
	jitter := time.Duration(rand.Intn(int(expDelay / 4)))
	return expDelay + jitter
}

// ConnectWithRetry is an exported version of connectWithRetry for use in main application
func ConnectWithRetry(token string, db *database.Queries, cfg *config.Config, allowedVCs map[string]struct{}, maxRetries int) (*Bot, error) {
	return connectWithRetry(token, db, cfg, allowedVCs, maxRetries)
}
