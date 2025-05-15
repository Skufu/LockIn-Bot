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

	// Slash command specific fields
	commandHandlers map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)
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
		commandHandlers:        make(map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)),
	}

	// Register slash command handlers here
	bot.commandHandlers["stats"] = bot.handleSlashStatsCommand
	bot.commandHandlers["leaderboard"] = bot.handleSlashLeaderboardCommand
	bot.commandHandlers["help"] = bot.handleSlashHelpCommand
	bot.commandHandlers["streak"] = bot.handleSlashStreakCommand

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

	return bot, nil
}

// Session returns the underlying discordgo session
func (b *Bot) Session() *discordgo.Session {
	return b.session
}

// Close closes the Discord session
func (b *Bot) Close() {
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
		// durationMs := duration.Milliseconds() // This was calculated but not used in the original placeholder

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
		if handler, ok := b.commandHandlers[i.ApplicationCommandData().Name]; ok {
			handler(s, i)
		} else {
			log.Printf("Unknown command received: %s", i.ApplicationCommandData().Name)
			// Optionally send an ephemeral message back to the user
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Unknown command.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
		}
	}
}

// handleSlashStatsCommand is the new handler for the /stats slash command
func (b *Bot) handleSlashStatsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background() // Or derive from a base context if you have one

	// User ID from interaction
	userID := ""
	if i.Member != nil && i.Member.User != nil { // Interaction in a Guild
		userID = i.Member.User.ID
	} else if i.User != nil { // Interaction in DMs
		userID = i.User.ID
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

	// Username for display
	username := ""
	if i.Member != nil && i.Member.User != nil {
		username = i.Member.User.Username
	} else if i.User != nil {
		username = i.User.Username
	}

	// DEFER THE RESPONSE INITIALLY
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		// Optionally, make the deferred message ephemeral if you want the final stats to also be ephemeral.
		// Data: &discordgo.InteractionResponseData{
		// 	Flags: discordgo.MessageFlagsEphemeral,
		// },
	})
	if err != nil {
		log.Printf("Error sending deferred interaction response for /stats: %v", err)
		// If we can't even defer, we probably can't edit later either.
		// You might want to just return or try a simple error message if deferral fails.
		return
	}

	// Check if user exists
	_, err = b.db.GetUser(ctx, userID)
	if err != nil {
		// Create the user if they don't exist
		_, createErr := b.db.CreateUser(ctx, database.CreateUserParams{
			UserID:   userID,
			Username: sql.NullString{String: username, Valid: true},
		})
		if createErr != nil {
			log.Printf("Error creating user via slash command: %v", createErr)
			content := "Error creating your user profile. Please try again or join a voice channel first."
			_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &content,
			})
			if err != nil {
				log.Printf("Error editing interaction for user creation error: %v", err)
			}
			return
		}
	}

	// Get user stats
	stats, err := b.db.GetUserStats(ctx, userID)
	if err != nil {
		content := "You haven't studied yet! Join a voice channel to start tracking your study time."
		_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &content,
			// Note: Original did not have Ephemeral flag here, so message would be public.
		})
		if err != nil {
			log.Printf("Error editing interaction for no stats yet: %v", err)
		}
		return
	}

	// Format the study times
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

	total := time.Duration(totalMs) * time.Millisecond
	daily := time.Duration(dailyMs) * time.Millisecond
	weekly := time.Duration(weeklyMs) * time.Millisecond
	monthly := time.Duration(monthlyMs) * time.Millisecond

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

	// Send the embed as an edit to the original deferred response
	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
	if err != nil {
		log.Printf("Error sending slash command response edit for /stats: %v", err)
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
		// entry.Username is sql.NullString, entry.TotalStudyMs is sql.NullInt64
		// entry.UserID is string (not nullable in users table schema, assuming)
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
		log.Printf("Error sending slash command response for /leaderboard: %v", err)
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
			// Flags:   discordgo.MessageFlagsEphemeral, // Optional: make it visible only to the user
		},
	})
	if err != nil {
		log.Printf("Error sending slash command response for /help: %v", err)
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
		// Call StreakService's handler in a goroutine to prevent blocking event processing.
		// The StreakService will internally check if the channel is tracked and if the user is joining.
		go func() {
			ctx := context.Background()
			err := b.streakService.HandleVoiceActivity(ctx, v.UserID, v.GuildID, v.ChannelID) // v.ChannelID is the new channel, empty if leaving all
			if err != nil {
				log.Printf("Error in StreakService.HandleVoiceActivity for user %s: %v", v.UserID, err)
			}
		}()
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

// handleUserJoinedStudySession (ensure this and Left are methods on Bot or accessible)
// This is where you'd put the logic from your old bot.handleUserJoinedVoice
func (b *Bot) handleUserJoinedStudySession(s *discordgo.Session, v *discordgo.VoiceStateUpdate, user *discordgo.User) {
	ctx := context.Background()
	b.activeSessionMu.Lock()
	defer b.activeSessionMu.Unlock()

	if _, exists := b.activeSessions[v.UserID]; exists {
		log.Printf("User %s already has an active study session, not starting new one.", v.UserID)
		return
	}
	now := time.Now()
	b.activeSessions[v.UserID] = now

	dbUserParams := database.CreateUserParams{
		UserID: v.UserID,
	}
	if user != nil {
		dbUserParams.Username = sql.NullString{String: user.Username, Valid: true}
	} else {
		dbUserParams.Username = sql.NullString{String: "UnknownUser-" + v.UserID[:6], Valid: true} // Fallback username
	}

	_, err := b.db.CreateUser(ctx, dbUserParams)
	if err != nil {
		log.Printf("Error creating/updating user %s for study session: %v", v.UserID, err)
	}

	session, err := b.db.CreateStudySession(ctx, database.CreateStudySessionParams{
		UserID:    sql.NullString{String: v.UserID, Valid: true},
		StartTime: now,
	})
	if err != nil {
		log.Printf("Error creating study session for user %s: %v", v.UserID, err)
	} else {
		log.Printf("Started study session %d for user %s in VC %s", session.SessionID, v.UserID, v.ChannelID)
	}
}

// handleUserLeftStudySession (ensure this and Joined are methods on Bot or accessible)
// This is where you'd put the logic from your old bot.handleUserLeftVoice
func (b *Bot) handleUserLeftStudySession(s *discordgo.Session, v *discordgo.VoiceStateUpdate, user *discordgo.User) {
	ctx := context.Background()
	b.activeSessionMu.Lock()
	defer b.activeSessionMu.Unlock()

	if _, exists := b.activeSessions[v.UserID]; !exists {
		log.Printf("No active study session found for user %s to end.", v.UserID)
		return
	}
	now := time.Now()
	delete(b.activeSessions, v.UserID)

	// v.BeforeUpdate should be non-nil if user truly left a channel
	// However, if this is called due to moving to untracked, v.BeforeUpdate might be the state *before* moving to untracked.
	// We need the session associated with this user_id that is active.
	activeSession, err := b.db.GetActiveStudySession(ctx, sql.NullString{String: v.UserID, Valid: true})
	if err != nil {
		log.Printf("Error getting active study session for user %s to end: %v", v.UserID, err)
		return
	}

	endedSession, err := b.db.EndStudySession(ctx, database.EndStudySessionParams{
		SessionID: activeSession.SessionID,
		EndTime:   sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		log.Printf("Error ending study session %d for user %s: %v", activeSession.SessionID, v.UserID, err)
		return
	}
	log.Printf("Ended study session %d for user %s. Duration: %dms", endedSession.SessionID, v.UserID, endedSession.DurationMs.Int64)

	if endedSession.DurationMs.Valid && endedSession.DurationMs.Int64 > 0 {
		durationMs := endedSession.DurationMs.Int64
		_, err = b.db.CreateOrUpdateUserStats(ctx, database.CreateOrUpdateUserStatsParams{
			UserID:       v.UserID,
			TotalStudyMs: sql.NullInt64{Int64: durationMs, Valid: true},
			// Ensure other stats fields are handled as per your DB schema and logic for CreateOrUpdateUserStats
		})
		if err != nil {
			log.Printf("Error updating user stats for %s post-study: %v", v.UserID, err)
		}

		duration := time.Duration(durationMs) * time.Millisecond
		log.Printf("User %s studied for %s", v.UserID, formatDuration(duration))
		if b.LoggingChannelID != "" {
			message := fmt.Sprintf("<@%s> has spent %s studying!", v.UserID, formatDuration(duration))
			_, errC := s.ChannelMessageSend(b.LoggingChannelID, message)
			if errC != nil {
				log.Printf("Error sending study session end message to Discord: %v", errC)
			}
		}
	}
}

// registerCommands registers slash commands with Discord.
func (b *Bot) registerCommands() {
	log.Println("Registering commands...")
	// Add command registration logic here using b.Session.ApplicationCommandCreate
	// This should include the new /streak command.
	// Example for a /streak command:
	// _, err := b.Session.ApplicationCommandCreate(b.Session.State.User.ID, b.TestGuildID, &discordgo.ApplicationCommand{
	// Name: "streak",
	// Description: "Check your current study streak!",
	// })
	// if err != nil {
	// log.Printf("Cannot create slash command 'streak': %v", err)
	// }
	log.Println("Commands registered (placeholder - implement actual registration)")
}

// SetStreakService assigns the StreakService to the bot.
func (b *Bot) SetStreakService(ss *service.StreakService) {
	b.streakService = ss
}

// NEW Method for /streak command
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
	} else if i.User != nil { // Interaction in DMs, GuildID might be empty
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
	if guildID == "" && i.User != nil { // DM context, streaks are per-guild, so this command is less meaningful in DM
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "The /streak command is best used within a server.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	streakInfo, err := b.streakService.GetUserStreakInfoText(context.Background(), userID, guildID)
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

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: streakInfo,
			// Flags: discordgo.MessageFlagsEphemeral, // Uncomment if you want streak info to be ephemeral
		},
	})
}
