package bot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/config"
	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/Skufu/LockIn-Bot/internal/service"
	"github.com/bwmarrin/discordgo"
)

// Bot represents the Discord bot
type Bot struct {
	session                *discordgo.Session
	db                     *database.Queries
	activeSessions         map[string]time.Time // Maps user_id to session start time
	activeSessionMu        sync.Mutex
	LoggingChannelID       string                 // Added to store the logging channel ID
	testGuildID            string                 // Added to store the test guild ID for command registration
	allowedVoiceChannelIDs map[string]struct{}    // For storing allowed voice channel IDs
	cfg                    *config.Config         // Store the full config
	streakService          *service.StreakService // Added streak service

	// Worker pool for handling voice events to prevent goroutine explosion
	voiceEventChan chan func()
	shutdownChan   chan struct{}
}

// New creates a new Discord bot instance
func New(token string, db *database.Queries, appConfig *config.Config, allowedVCs map[string]struct{}) (*Bot, error) {
	// Create a new Discord session
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	// Make a copy of the allowed VCs map from config
	currentAllowedVCs := make(map[string]struct{})
	if appConfig.AllowedVoiceChannelIDsMap != nil {
		for id := range appConfig.AllowedVoiceChannelIDsMap {
			currentAllowedVCs[id] = struct{}{}
		}
	}

	bot := &Bot{
		session:                dg,
		db:                     db,
		activeSessions:         make(map[string]time.Time),
		LoggingChannelID:       appConfig.LoggingChannelID,
		testGuildID:            appConfig.TestGuildID,
		allowedVoiceChannelIDs: currentAllowedVCs,
		cfg:                    appConfig,
		streakService:          nil,
		voiceEventChan:         make(chan func()),
		shutdownChan:           make(chan struct{}),
	}

	// Register handlers
	dg.AddHandler(bot.handleReady)
	dg.AddHandler(bot.handleVoiceStateUpdate)
	dg.AddHandler(bot.handleInteractionCreate)

	// We only care about voice and guild messages
	dg.Identify.Intents = discordgo.IntentsGuildVoiceStates | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	// Open the websocket and begin listening
	err = dg.Open()
	if err != nil {
		return nil, err
	}

	// Start worker pool for voice events (prevents goroutine explosion)
	go bot.voiceEventWorker()

	return bot, nil
}

// Session returns the underlying discordgo session
func (b *Bot) Session() *discordgo.Session {
	return b.session
}

// Close closes the Discord session
func (b *Bot) Close() {
	// Signal shutdown to worker
	close(b.shutdownChan)

	// End all active sessions before shutting down
	b.endAllActiveSessions()
	b.session.Close()
}

// endAllActiveSessions ends all active study sessions when the bot shuts down
func (b *Bot) endAllActiveSessions() {
	b.activeSessionMu.Lock()
	defer b.activeSessionMu.Unlock()

	ctx := context.Background() // Create a context for database operations
	now := time.Now()

	if len(b.activeSessions) == 0 {
		log.Println("No active study sessions to end on shutdown.")
		return
	}

	log.Printf("Attempting to end %d active study session(s) on shutdown...", len(b.activeSessions))

	for userID, startTime := range b.activeSessions {
		log.Printf("Processing shutdown for user %s (session started at %v)", userID, startTime)

		duration := now.Sub(startTime)

		// 1. Find the active session ID for this user
		activeDBSession, err := b.db.GetActiveStudySession(ctx, sql.NullString{String: userID, Valid: true})
		if err != nil {
			log.Printf("Error getting active DB session for user %s during shutdown: %v. Session may not be ended correctly.", userID, err)
			continue // Skip to the next user
		}

		// 2. End that specific session in the DB
		endedSession, err := b.db.EndStudySession(ctx, database.EndStudySessionParams{
			SessionID: activeDBSession.SessionID,
			EndTime:   sql.NullTime{Time: now, Valid: true},
		})
		if err != nil {
			log.Printf("Error ending DB study session %d for user %s during shutdown: %v. Stats may not be updated.", activeDBSession.SessionID, userID, err)
			continue // Skip to the next user
		}

		log.Printf("Successfully ended DB session %d for user %s on shutdown. Duration: %d ms.", endedSession.SessionID, userID, endedSession.DurationMs.Int64)

		// 3. Update user stats
		if endedSession.DurationMs.Valid && endedSession.DurationMs.Int64 > 0 {
			_, err = b.db.CreateOrUpdateUserStats(ctx, database.CreateOrUpdateUserStatsParams{
				UserID:       userID,
				TotalStudyMs: sql.NullInt64{Int64: endedSession.DurationMs.Int64, Valid: true},
			})
			if err != nil {
				log.Printf("Error updating user stats for user %s during shutdown after session %d: %v", userID, endedSession.SessionID, err)
			}
		}

		// If LoggingChannelID is set, also send a message about the shutdown-ended session
		if b.LoggingChannelID != "" {
			username := userID                             // Default to UserID
			discordUser, userErr := b.session.User(userID) // Attempt to get full user info
			if userErr == nil && discordUser != nil {
				username = discordUser.Username
			}

			// Use the duration from the ended DB session for consistency if available, otherwise fallback to in-memory calculation
			finalDuration := duration // Fallback
			if endedSession.DurationMs.Valid {
				finalDuration = time.Duration(endedSession.DurationMs.Int64) * time.Millisecond
			}

			message := fmt.Sprintf("<@%s> (%s) session ended due to bot shutdown after %s.", userID, username, formatDuration(finalDuration))
			_, sendErr := b.session.ChannelMessageSend(b.LoggingChannelID, message)
			if sendErr != nil {
				log.Printf("Error sending shutdown session message to Discord channel %s for user %s: %v", b.LoggingChannelID, userID, sendErr)
			}
		}
		delete(b.activeSessions, userID) // Remove from in-memory map after processing
	}
	log.Println("Finished processing active sessions on shutdown.")
}

func (b *Bot) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)

	guildID := b.testGuildID // Use the configured testGuildID

	if guildID == "" {
		log.Println("Registering GLOBAL slash commands...")
	} else {
		log.Printf("Registering slash commands for TEST guild ID: %s", guildID)
	}

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "stats",
			Description: "Shows your study/voice channel time statistics.",
		},
		{
			Name:        "leaderboard",
			Description: "Shows the study time leaderboard.",
		},
		{
			Name:        "help",
			Description: "Shows available commands and information about the bot.",
		},
		{
			Name:        "streak",
			Description: "Check your current study streak!",
		},
	}

	// Iterate and register commands
	// Note: For global commands, it can take up to an hour for them to propagate.
	// For guild-specific commands (faster registration for testing), you use:
	// s.ApplicationCommandCreate(s.State.User.ID, "YOUR_GUILD_ID", cmd)
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, cmd := range commands {
		regCmd, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, cmd) // Using configured guildID
		if err != nil {
			log.Printf("Cannot create '%v' command in guild %s: %v", cmd.Name, guildID, err)
		} else {
			registeredCommands[i] = regCmd
			log.Printf("Registered command: %s", regCmd.Name)
		}
	}
}

func (b *Bot) handleInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type == discordgo.InteractionApplicationCommand {
		commandName := i.ApplicationCommandData().Name
		switch commandName {
		case "stats":
			b.handleSlashStatsCommand(s, i)
		case "leaderboard":
			b.handleSlashLeaderboardCommand(s, i)
		case "help":
			b.handleSlashHelpCommand(s, i)
		case "streak":
			b.handleSlashStreakCommand(s, i)
		default:
			log.Printf("Unknown command received: %s", commandName)
			// Direct error response - no retry needed for user errors
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Unknown command.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				log.Printf("Error responding to unknown command: %v", err)
			}
		}
	}
}

// handleSlashStatsCommand is the handler for the /stats slash command
func (b *Bot) handleSlashStatsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()

	// Get user ID from interaction
	userID := ""
	username := ""
	if i.Member != nil && i.Member.User != nil { // Guild interaction
		userID = i.Member.User.ID
		username = i.Member.User.Username
	} else if i.User != nil { // DM interaction
		userID = i.User.ID
		username = i.User.Username
	}

	if userID == "" {
		log.Println("Error: could not determine UserID from interaction for /stats command")
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Error: Could not identify user.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Check if user exists, create if needed
	_, err := b.db.GetUser(ctx, userID)
	if err != nil {
		// Create the user if they don't exist
		_, createErr := b.db.CreateUser(ctx, database.CreateUserParams{
			UserID:   userID,
			Username: sql.NullString{String: username, Valid: true},
		})
		if createErr != nil {
			log.Printf("Error creating user via /stats command: %v", createErr)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Error creating your user profile. Please try again or join a voice channel first.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}
	}

	// Get user stats
	stats, err := b.db.GetUserStats(ctx, userID)
	if err != nil {
		log.Printf("Error getting user stats for %s: %v", userID, err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You haven't studied yet! Join a voice channel to start tracking your study time.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Extract study times safely
	var totalMs, dailyMs, weeklyMs, monthlyMs int64
	if stats.TotalStudyMs.Valid {
		totalMs = stats.TotalStudyMs.Int64
	}
	if stats.DailyStudyMs.Valid {
		dailyMs = stats.DailyStudyMs.Int64
	}
	if stats.WeeklyStudyMs.Valid {
		weeklyMs = stats.WeeklyStudyMs.Int64
	}
	if stats.MonthlyStudyMs.Valid {
		monthlyMs = stats.MonthlyStudyMs.Int64
	}

	// Convert to durations
	total := time.Duration(totalMs) * time.Millisecond
	daily := time.Duration(dailyMs) * time.Millisecond
	weekly := time.Duration(weeklyMs) * time.Millisecond
	monthly := time.Duration(monthlyMs) * time.Millisecond

	// Create stats embed
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("Study Stats for %s", username),
		Description: "Your study time statistics from voice channels.",
		Color:       0x00AAFF, // Blue color
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Total Study Time", Value: formatDuration(total), Inline: true},
			{Name: "Today", Value: formatDuration(daily), Inline: true},
			{Name: "This Week", Value: formatDuration(weekly), Inline: true},
			{Name: "This Month", Value: formatDuration(monthly), Inline: true},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer:    &discordgo.MessageEmbedFooter{Text: "Keep up the good work!"},
	}

	// Send response directly (no deferred response needed for simple stats)
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})

	if err != nil {
		log.Printf("Error sending /stats response: %v", err)
		// If the direct response fails, we can't send a followup since we didn't defer
		// This is actually better - it fails fast and clearly
	}
}

// handleSlashLeaderboardCommand handles the /leaderboard slash command
func (b *Bot) handleSlashLeaderboardCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()

	leaderboardData, err := b.db.GetLeaderboard(ctx)
	if err != nil {
		log.Printf("Error fetching leaderboard data: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Error: Could not fetch leaderboard data at this time. Please try again later.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if len(leaderboardData) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No one is on the leaderboard yet! Start studying to get your name up here.",
			},
		})
		return
	}

	embedFields := []*discordgo.MessageEmbedField{}
	for rank, entry := range leaderboardData {
		username := "Unknown User"
		if entry.Username.Valid {
			username = entry.Username.String
		}

		durationMs := int64(0)
		if entry.TotalStudyMs.Valid {
			durationMs = entry.TotalStudyMs.Int64
		}
		duration := time.Duration(durationMs) * time.Millisecond

		embedFields = append(embedFields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%d. %s", rank+1, username),
			Value:  fmt.Sprintf("Time Studied: %s (<@%s>)", formatDuration(duration), entry.UserID),
			Inline: false,
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🏆 Study Time Leaderboard - Top 10",
		Description: "See who has been putting in the hours!",
		Color:       0xFFD700, // Gold color
		Fields:      embedFields,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "LockIn Bot Leaderboard"},
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
	if err != nil {
		log.Printf("Error sending /leaderboard response: %v", err)
	}
}

// handleSlashHelpCommand handles the /help slash command
func (b *Bot) handleSlashHelpCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "LockIn Bot Help",
		Description: "Hi there! I'm LockIn Bot. I track time spent in voice channels and help you stay focused.",
		Color:       0x00AAFF, // Blue color
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "`/stats`",
				Value: "Shows your personal voice channel time statistics (total, today, this week, this month).",
			},
			{
				Name:  "`/leaderboard`",
				Value: "Displays the top users by voice channel time.",
			},
			{
				Name:  "`/help`",
				Value: "Shows this help message.",
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer:    &discordgo.MessageEmbedFooter{Text: "LockIn Bot"},
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
	if err != nil {
		log.Printf("Error sending /help response: %v", err)
	}
}

// formatDuration converts a time.Duration to a human-readable string
// e.g., "2h 15m 30s" or "45m 20s"
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// handleVoiceStateUpdate is called when a user's voice state changes
func (b *Bot) handleVoiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	// --- Streak Service Integration --- START
	if b.streakService != nil {
		// Queue the task instead of spawning a new goroutine every time
		b.voiceEventChan <- func() {
			ctx := context.Background()

			// Check if user joined a tracked voice channel
			userJoinedTrackedChannel := v.ChannelID != "" &&
				(v.BeforeUpdate == nil || v.BeforeUpdate.ChannelID != v.ChannelID)

			// Check if user left a tracked voice channel
			userLeftTrackedChannel := (v.BeforeUpdate != nil && v.BeforeUpdate.ChannelID != "") &&
				(v.ChannelID == "" || v.ChannelID != v.BeforeUpdate.ChannelID)

			if userJoinedTrackedChannel {
				err := b.streakService.HandleVoiceJoin(ctx, v.UserID, v.GuildID, v.ChannelID)
				if err != nil {
					log.Printf("Error in StreakService.HandleVoiceJoin for user %s: %v", v.UserID, err)
				}
			}

			if userLeftTrackedChannel {
				err := b.streakService.HandleVoiceLeave(ctx, v.UserID, v.GuildID)
				if err != nil {
					log.Printf("Error in StreakService.HandleVoiceLeave for user %s: %v", v.UserID, err)
				}
			}
		}
	} else {
		log.Println("Warning: StreakService is not initialized in Bot, skipping streak handling for VoiceStateUpdate.")
	}
	// --- Streak Service Integration --- END

	// Existing Study Session Logic (adapted from your previous handlers.go content)
	user, err := s.User(v.UserID)
	if err != nil {
		log.Printf("Error getting user %s for study session logic: %v", v.UserID, err)
		// Depending on how critical user object is, you might return or proceed cautiously
	}

	b.activeSessionMu.Lock()
	_, userWasInTrackedSession := b.activeSessions[v.UserID]
	b.activeSessionMu.Unlock()

	// Determine current state
	userJoinedNewChannel := v.ChannelID != "" && (v.BeforeUpdate == nil || v.BeforeUpdate.ChannelID != v.ChannelID)
	completelyLeftVoice := v.ChannelID == "" && (v.BeforeUpdate != nil && v.BeforeUpdate.ChannelID != "")

	// Check if the new channel (if any) is tracked for study sessions
	newChannelIsTracked := false
	if v.ChannelID != "" {
		if _, ok := b.allowedVoiceChannelIDs[v.ChannelID]; ok {
			newChannelIsTracked = true
		}
	}

	// Check if the old channel (if any) was tracked for study sessions
	oldChannelWasTracked := false
	if v.BeforeUpdate != nil && v.BeforeUpdate.ChannelID != "" {
		if _, ok := b.allowedVoiceChannelIDs[v.BeforeUpdate.ChannelID]; ok {
			oldChannelWasTracked = true
		}
	}

	// Logic for Study Sessions:
	if userJoinedNewChannel {
		if newChannelIsTracked {
			if !userWasInTrackedSession { // Started new session in a tracked channel
				log.Printf("User %s joined tracked VC %s. Starting study session.", v.UserID, v.ChannelID)
				b.handleUserJoinedStudySession(s, v, user)
			} else if oldChannelWasTracked && v.BeforeUpdate.ChannelID != v.ChannelID {
				// Moved between two tracked VCs - current study session logic might implicitly handle this by not ending/restarting.
				log.Printf("User %s moved between tracked VCs (%s -> %s). Study session continues.", v.UserID, v.BeforeUpdate.ChannelID, v.ChannelID)
			} else if !oldChannelWasTracked { // Moved from untracked to tracked
				log.Printf("User %s moved from untracked to tracked VC %s. Starting study session.", v.UserID, v.ChannelID)
				b.handleUserJoinedStudySession(s, v, user)
			}
		} else { // Joined/moved to an untracked channel
			if userWasInTrackedSession { // Was in a tracked channel, now in untracked: end session
				log.Printf("User %s moved from tracked to untracked VC %s. Ending study session.", v.UserID, v.ChannelID)
				b.handleUserLeftStudySession(s, v, user) // Pass v, it has BeforeUpdate for context
			}
		}
	} else if completelyLeftVoice {
		if userWasInTrackedSession && oldChannelWasTracked { // Left from a tracked channel
			log.Printf("User %s left tracked VC %s. Ending study session.", v.UserID, v.BeforeUpdate.ChannelID)
			b.handleUserLeftStudySession(s, v, user)
		}
	}
}

// handleUserJoinedStudySession handles when a user joins a tracked voice channel
func (b *Bot) handleUserJoinedStudySession(s *discordgo.Session, v *discordgo.VoiceStateUpdate, user *discordgo.User) {
	// Add user to active sessions map
	b.activeSessionMu.Lock()
	startTime := time.Now()
	b.activeSessions[user.ID] = startTime
	b.activeSessionMu.Unlock()

	ctx := context.Background()
	now := time.Now() // Define 'now' for consistent timing

	b.activeSessionMu.Lock()
	defer b.activeSessionMu.Unlock()

	// Check for and end any pre-existing active session for this user in the DB
	existingDBSession, err := b.db.GetActiveStudySession(ctx, sql.NullString{String: v.UserID, Valid: true})
	if err == nil { // An active session exists in the DB
		log.Printf("User %s has an existing active DB session %d started at %v. Ending it with current time %v before starting new one.", v.UserID, existingDBSession.SessionID, existingDBSession.StartTime, now)
		_, endErr := b.db.EndStudySession(ctx, database.EndStudySessionParams{
			SessionID: existingDBSession.SessionID,
			EndTime:   sql.NullTime{Time: now, Valid: true}, // End it with current time
		})
		if endErr != nil {
			log.Printf("Error auto-ending pre-existing DB session %d for user %s: %v", existingDBSession.SessionID, v.UserID, endErr)
		}
	} else if err != sql.ErrNoRows { // Log unexpected errors from GetActiveStudySession
		log.Printf("Error checking for existing active DB session for user %s: %v", v.UserID, err)
		// For now, we'll proceed to attempt creating a new session.
	}

	// 2. Regardless of in-memory state, we are starting a new session because the user just joined a tracked VC.
	// The previous active DB session (if any) has been ended.
	// Update/set the in-memory tracker.
	if _, existsInMemory := b.activeSessions[v.UserID]; existsInMemory {
		log.Printf("User %s was already in local activeSessions map. Overwriting with new session start time %v.", v.UserID, now)
	}
	b.activeSessions[v.UserID] = now

	// 3. Create DB user if they don't exist
	dbUserParams := database.CreateUserParams{UserID: v.UserID}
	if user != nil {
		dbUserParams.Username = sql.NullString{String: user.Username, Valid: true}
	} else {
		// Attempt to fetch username again if primary 'user' object is nil
		fetchedUser, fetchErr := s.User(v.UserID)
		if fetchErr == nil && fetchedUser != nil {
			dbUserParams.Username = sql.NullString{String: fetchedUser.Username, Valid: true}
		} else {
			log.Printf("Could not fetch user object for %s in handleUserJoinedStudySession (UserID: %s), using placeholder username. Fetch error: %v", v.UserID, v.UserID, fetchErr)
			// Use a placeholder to satisfy NOT NULL constraints if username is mandatory, or ensure DB schema allows NULL
			dbUserParams.Username = sql.NullString{String: "UnknownUser-" + v.UserID, Valid: true} // Ensure length is not an issue
			if len(v.UserID) > 6 {                                                                 // make sure UserID has at least 6 chars to slice
				dbUserParams.Username = sql.NullString{String: "UnknownUser-" + v.UserID[:6], Valid: true}
			}
		}
	}
	_, createErr := b.db.CreateUser(ctx, dbUserParams)
	if createErr != nil {
		// Log error, but don't necessarily block session creation if user already exists and this is just an update failing
		log.Printf("Error creating/updating user %s for study session (params: %+v): %v", v.UserID, dbUserParams, createErr)
	}

	// 4. Create the new study session in the DB
	session, err := b.db.CreateStudySession(ctx, database.CreateStudySessionParams{
		UserID:    sql.NullString{String: v.UserID, Valid: true},
		StartTime: now, // Use the 'now' from the beginning of this function call
	})
	if err != nil {
		log.Printf("Error creating new study session for user %s: %v", v.UserID, err)
		// If DB creation fails, consider removing from b.activeSessions or other cleanup
	} else {
		log.Printf("Started study session %d for user %s in VC %s at %v", session.SessionID, v.UserID, v.ChannelID, now)
	}
}

// handleUserLeftStudySession handles when a user leaves a tracked voice channel
func (b *Bot) handleUserLeftStudySession(s *discordgo.Session, v *discordgo.VoiceStateUpdate, user *discordgo.User) {
	b.activeSessionMu.Lock()
	defer b.activeSessionMu.Unlock()

	// Check if the user has an active session in memory
	startTime, ok := b.activeSessions[user.ID]
	if !ok {
		// log.Printf("User %s left voice channel %s but had no active session in memory.", user.Username, v.BeforeUpdate.ChannelID)
		return // No active session for this user in memory
	}

	duration := time.Since(startTime)
	log.Printf("User %s (%s) left voice channel. Study session ended. Duration: %s", user.Username, user.ID, formatDuration(duration))

	// Get the active study session from the database
	ctx := context.Background()
	activeDBSession, err := b.db.GetActiveStudySession(ctx, sql.NullString{String: user.ID, Valid: true})
	if err != nil {
		log.Printf("Error getting active DB session for user %s: %v", user.ID, err)
		// Still attempt to remove from in-memory map
		delete(b.activeSessions, user.ID)
		return
	}

	// End the study session in the database
	// Ensure we are passing sql.NullTime for EndTime
	endedSession, err := b.db.EndStudySession(ctx, database.EndStudySessionParams{
		SessionID: activeDBSession.SessionID,
		EndTime:   sql.NullTime{Time: time.Now(), Valid: true},
		// DurationMs is now calculated by the query
	})
	if err != nil {
		log.Printf("Error ending study session %d for user %s in DB: %v", activeDBSession.SessionID, user.ID, err)
		// Still attempt to remove from in-memory map
		delete(b.activeSessions, user.ID)
		return
	}

	log.Printf("Ended DB session %d for user %s. DB Duration: %d ms.", endedSession.SessionID, user.ID, endedSession.DurationMs.Int64)

	// Update user stats
	if endedSession.DurationMs.Valid && endedSession.DurationMs.Int64 > 0 {
		_, err = b.db.CreateOrUpdateUserStats(ctx, database.CreateOrUpdateUserStatsParams{
			UserID:       user.ID,
			TotalStudyMs: sql.NullInt64{Int64: endedSession.DurationMs.Int64, Valid: true}, // Pass as sql.NullInt64
			// Daily, weekly, monthly are also updated by this query based on the same amount
		})
		if err != nil {
			log.Printf("Error updating user stats for user %s after session %d: %v", user.ID, endedSession.SessionID, err)
		} else {
			log.Printf("Successfully updated stats for user %s after session %d.", user.ID, endedSession.SessionID)
		}
	}

	// Send study time announcement to logging channel if configured
	if b.LoggingChannelID != "" && endedSession.DurationMs.Valid && endedSession.DurationMs.Int64 > 0 {
		durationForMessage := time.Duration(endedSession.DurationMs.Int64) * time.Millisecond
		formattedDuration := formatDuration(durationForMessage)
		message := fmt.Sprintf("<@%s> has spent %s studying!", user.ID, formattedDuration)
		_, err = b.session.ChannelMessageSend(b.LoggingChannelID, message)
		if err != nil {
			log.Printf("Error sending study time announcement to Discord channel %s for user %s: %v", b.LoggingChannelID, user.ID, err)
		}
	}

	// Remove user from active sessions map
	delete(b.activeSessions, user.ID)
	log.Printf("User %s removed from active session map.", user.ID)
}

func (b *Bot) SetStreakService(ss *service.StreakService) {
	b.streakService = ss
}

// handleSlashStreakCommand handles the /streak slash command
func (b *Bot) handleSlashStreakCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if b.streakService == nil {
		log.Println("Error: StreakService not available for /streak command")
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Streak service is currently unavailable.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	userID := ""
	if i.Member != nil && i.Member.User != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	if userID == "" {
		log.Println("Error: could not determine UserID from interaction for /streak command")
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Error: Could not identify user.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	guildID := i.GuildID
	if guildID == "" && i.User != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "The /streak command is best used within a server.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed, err := b.streakService.GetUserStreakInfoEmbed(context.Background(), userID, guildID)
	if err != nil {
		log.Printf("Error getting streak info for user %s in guild %s: %v", userID, guildID, err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Could not retrieve your streak information at this time.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
	if err != nil {
		log.Printf("Error sending /streak response: %v", err)
	}
}

// voiceEventWorker processes voice events in a single goroutine to prevent memory leaks
func (b *Bot) voiceEventWorker() {
	for {
		select {
		case task := <-b.voiceEventChan:
			task()
		case <-b.shutdownChan:
			return
		}
	}
}

// GetSessionStartTime returns the start time for a user's session (for StreakService)
func (b *Bot) GetSessionStartTime(userID string) (time.Time, bool) {
	b.activeSessionMu.Lock()
	defer b.activeSessionMu.Unlock()
	startTime, exists := b.activeSessions[userID]
	return startTime, exists
}
