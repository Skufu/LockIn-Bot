package bot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/bwmarrin/discordgo"
)

// SessionTimeoutChecker periodically checks for sessions that have been running too long
type SessionTimeoutChecker struct {
	bot             *Bot
	maxSessionHours int
	checkInterval   time.Duration
}

// NewSessionTimeoutChecker creates a new session timeout checker
func NewSessionTimeoutChecker(bot *Bot, maxSessionHours int, checkInterval time.Duration) *SessionTimeoutChecker {
	return &SessionTimeoutChecker{
		bot:             bot,
		maxSessionHours: maxSessionHours,
		checkInterval:   checkInterval,
	}
}

// Start begins the timeout checking routine
func (s *SessionTimeoutChecker) Start() {
	go s.checkTimeoutsLoop()
}

// checkTimeoutsLoop runs periodically to check for timed-out sessions
func (s *SessionTimeoutChecker) checkTimeoutsLoop() {
	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkAndEndTimeoutSessions()
		case <-s.bot.shutdownChan:
			return
		}
	}
}

// checkAndEndTimeoutSessions finds and ends sessions that have been running too long
func (s *SessionTimeoutChecker) checkAndEndTimeoutSessions() {
	ctx := context.Background()
	now := time.Now()
	maxDuration := time.Duration(s.maxSessionHours) * time.Hour

	s.bot.activeSessionMu.Lock()
	var timeoutUsers []string

	for userID, startTime := range s.bot.activeSessions {
		if now.Sub(startTime) > maxDuration {
			timeoutUsers = append(timeoutUsers, userID)
		}
	}
	s.bot.activeSessionMu.Unlock()

	if len(timeoutUsers) == 0 {
		return
	}

	log.Printf("Found %d sessions that have exceeded %d hours, ending them", len(timeoutUsers), s.maxSessionHours)

	for _, userID := range timeoutUsers {
		s.endTimeoutSession(ctx, userID, now)
	}
}

// endTimeoutSession ends a single timeout session
func (s *SessionTimeoutChecker) endTimeoutSession(ctx context.Context, userID string, now time.Time) {
	log.Printf("Ending timeout session for user %s", userID)

	// Get the user object for notifications (used later for logging)
	_, err := s.bot.session.User(userID)
	if err != nil {
		log.Printf("Could not fetch user %s for timeout session end: %v", userID, err)
	}

	// Check if user is actually still in a voice channel
	isStillInVoice := s.isUserInTrackedVoiceChannel(userID)
	if isStillInVoice {
		log.Printf("User %s is still in a voice channel, allowing session to continue", userID)
		return
	}

	// End the session
	s.bot.activeSessionMu.Lock()
	startTime, exists := s.bot.activeSessions[userID]
	if !exists {
		s.bot.activeSessionMu.Unlock()
		return
	}
	delete(s.bot.activeSessions, userID)
	s.bot.activeSessionMu.Unlock()

	// End the database session
	activeDBSession, err := s.bot.db.GetActiveStudySession(ctx, sql.NullString{String: userID, Valid: true})
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("Error getting active DB session for timeout user %s: %v", userID, err)
		}
		return
	}

	endedSession, err := s.bot.db.EndStudySession(ctx, database.EndStudySessionParams{
		SessionID: activeDBSession.SessionID,
		EndTime:   sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		log.Printf("Error ending timeout session %d for user %s: %v", activeDBSession.SessionID, userID, err)
		return
	}

	duration := now.Sub(startTime)
	log.Printf("Ended timeout session %d for user %s. Duration: %s (DB: %d ms)",
		endedSession.SessionID, userID, formatDuration(duration), endedSession.DurationMs.Int64)

	// Update user stats
	if endedSession.DurationMs.Valid && endedSession.DurationMs.Int64 > 0 {
		_, err = s.bot.db.CreateOrUpdateUserStats(ctx, database.CreateOrUpdateUserStatsParams{
			UserID:       userID,
			TotalStudyMs: sql.NullInt64{Int64: endedSession.DurationMs.Int64, Valid: true},
		})
		if err != nil {
			log.Printf("Error updating user stats for timeout user %s after session %d: %v", userID, endedSession.SessionID, err)
		}
	}

	// Send notification about the ended session
	if s.bot.LoggingChannelID != "" && endedSession.DurationMs.Valid && endedSession.DurationMs.Int64 > 0 {
		durationForMessage := time.Duration(endedSession.DurationMs.Int64) * time.Millisecond
		formattedDuration := formatDuration(durationForMessage)
		message := fmt.Sprintf("⏰ <@%s> session auto-ended after %s (session cleanup)", userID, formattedDuration)
		_, err = s.bot.session.ChannelMessageSend(s.bot.LoggingChannelID, message)
		if err != nil {
			log.Printf("Error sending timeout session message for user %s: %v", userID, err)
		}
	}
}

// isUserInTrackedVoiceChannel checks if a user is currently in any tracked voice channel
func (s *SessionTimeoutChecker) isUserInTrackedVoiceChannel(userID string) bool {
	// Get all guilds the bot is in
	for _, guild := range s.bot.session.State.Guilds {
		// Check voice states in this guild
		for _, voiceState := range guild.VoiceStates {
			if voiceState.UserID == userID && voiceState.ChannelID != "" {
				// Check if this channel is tracked
				if _, tracked := s.bot.allowedVoiceChannelIDs[voiceState.ChannelID]; tracked {
					return true
				}
			}
		}
	}
	return false
}

// ImprovedVoiceStateHandler contains enhanced voice state update logic
type ImprovedVoiceStateHandler struct {
	bot *Bot
}

// NewImprovedVoiceStateHandler creates a new improved voice state handler
func NewImprovedVoiceStateHandler(bot *Bot) *ImprovedVoiceStateHandler {
	return &ImprovedVoiceStateHandler{bot: bot}
}

// HandleVoiceStateUpdate processes voice state updates with better session management
func (h *ImprovedVoiceStateHandler) HandleVoiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	// Enhanced logging for debugging
	beforeChannel := "none"
	if v.BeforeUpdate != nil && v.BeforeUpdate.ChannelID != "" {
		beforeChannel = v.BeforeUpdate.ChannelID
	}
	currentChannel := "none"
	if v.ChannelID != "" {
		currentChannel = v.ChannelID
	}

	log.Printf("Voice state update: User %s moved from %s to %s", v.UserID, beforeChannel, currentChannel)

	// Check for potential session inconsistencies FIRST
	h.validateSessionConsistency(v.UserID)

	// Deduplication with more detailed logging
	if h.bot.isDuplicateVoiceEvent(v) {
		log.Printf("Skipping duplicate voice event for user %s (from %s to %s)", v.UserID, beforeChannel, currentChannel)
		return
	}

	// Process using the original logic but with validation
	h.bot.handleVoiceStateUpdate(s, v)
}

// validateSessionConsistency checks if the user's session state matches reality
func (h *ImprovedVoiceStateHandler) validateSessionConsistency(userID string) {
	h.bot.activeSessionMu.Lock()
	_, hasActiveSession := h.bot.activeSessions[userID]
	h.bot.activeSessionMu.Unlock()

	if hasActiveSession {
		// User has active session - verify they're actually in a tracked channel
		isInTrackedChannel := h.isUserInAnyTrackedChannel(userID)
		if !isInTrackedChannel {
			log.Printf("INCONSISTENCY: User %s has active session but not in any tracked channel. Will end session on next leave event.", userID)
		}
	}
}

// isUserInAnyTrackedChannel checks if user is in any tracked voice channel
func (h *ImprovedVoiceStateHandler) isUserInAnyTrackedChannel(userID string) bool {
	for _, guild := range h.bot.session.State.Guilds {
		for _, voiceState := range guild.VoiceStates {
			if voiceState.UserID == userID && voiceState.ChannelID != "" {
				if _, tracked := h.bot.allowedVoiceChannelIDs[voiceState.ChannelID]; tracked {
					return true
				}
			}
		}
	}
	return false
}

// Enhanced session management functions
func (b *Bot) StartSessionTimeoutChecker() {
	checker := NewSessionTimeoutChecker(b, 4, 10*time.Minute) // Max 4 hours, check every 10 minutes
	checker.Start()
	log.Println("Started session timeout checker (max 4 hours, check every 10 minutes)")
}
