package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/config"
	"github.com/Skufu/LockIn-Bot/internal/database" // Path to your SQLC generated code
	"github.com/bwmarrin/discordgo"
	"github.com/robfig/cron/v3" // For scheduling tasks
)

type StreakService struct {
	dbQueries                 *database.Queries
	discordSession            *discordgo.Session
	cfg                       *config.Config
	trackedVoiceChannelIDs    map[string]struct{} // Parsed from cfg.AllowedVoiceChannelIDsMap
	streakNotificationChannel string              // From cfg.StreakNotificationChannelID
	cronScheduler             *cron.Cron
}

func NewStreakService(
	queries *database.Queries,
	session *discordgo.Session,
	appConfig *config.Config,
) *StreakService {
	// Make a copy of the map to ensure the service has its own instance
	// and to handle the case where appConfig.AllowedVoiceChannelIDsMap might be nil.
	trackedIDs := make(map[string]struct{})
	if appConfig.AllowedVoiceChannelIDsMap != nil {
		for id := range appConfig.AllowedVoiceChannelIDsMap {
			trackedIDs[id] = struct{}{}
		}
	}

	return &StreakService{
		dbQueries:                 queries,
		discordSession:            session,
		cfg:                       appConfig,
		trackedVoiceChannelIDs:    trackedIDs,
		streakNotificationChannel: appConfig.StreakNotificationChannelID,
		cronScheduler:             cron.New(cron.WithLocation(time.UTC)), // Use UTC for cron
	}
}

// StartScheduledTasks initializes and starts the cron jobs for streak management.
func (s *StreakService) StartScheduledTasks() {
	// Reset daily "streak_extended_today" flags shortly after midnight UTC
	_, err := s.cronScheduler.AddFunc("1 0 * * *", func() { // 00:01 UTC daily
		fmt.Println("StreakService: Running ResetDailyFlags task...")
		if err := s.ResetDailyFlags(context.Background()); err != nil {
			fmt.Printf("Error in ResetDailyFlags cron job: %v\n", err)
		}
	})
	if err != nil {
		fmt.Printf("Error scheduling ResetDailyFlags: %v\n", err)
	}

	// Check for and handle expired streaks hourly
	_, err = s.cronScheduler.AddFunc("@hourly", func() { // Runs at the beginning of every hour
		fmt.Println("StreakService: Running CheckAndHandleExpiredStreaks task...")
		if err := s.CheckAndHandleExpiredStreaks(context.Background()); err != nil {
			fmt.Printf("Error in CheckAndHandleExpiredStreaks cron job: %v\n", err)
		}
	})
	if err != nil {
		fmt.Printf("Error scheduling CheckAndHandleExpiredStreaks: %v\n", err)
	}

	// Send streak warning notifications hourly (internal logic limits to specific times)
	_, err = s.cronScheduler.AddFunc("@hourly", func() {
		fmt.Println("StreakService: Running SendStreakWarningNotifications task...")
		if err := s.SendStreakWarningNotifications(context.Background()); err != nil {
			fmt.Printf("Error in SendStreakWarningNotifications cron job: %v\n", err)
		}
	})
	if err != nil {
		fmt.Printf("Error scheduling SendStreakWarningNotifications: %v\n", err)
	}

	s.cronScheduler.Start()
	fmt.Println("StreakService: Scheduled tasks started.")
}

// StopScheduledTasks stops the cron scheduler.
func (s *StreakService) StopScheduledTasks() {
	if s.cronScheduler != nil {
		s.cronScheduler.Stop()
		fmt.Println("StreakService: Scheduled tasks stopped.")
	}
}

// HandleVoiceActivity is called when a user's voice state updates.
// It processes streak logic if the user joins a tracked voice channel.
func (s *StreakService) HandleVoiceActivity(ctx context.Context, userID, guildID, voiceChannelID string) error {
	if voiceChannelID == "" { // User left a channel, or it's an update we don't care about for starting streaks.
		return nil
	}

	if _, tracked := s.trackedVoiceChannelIDs[voiceChannelID]; !tracked {
		// fmt.Printf("StreakService: Voice channel %s is not tracked for streaks.\n", voiceChannelID)
		return nil // Not a tracked channel
	}

	fmt.Printf("StreakService: Handling voice activity for user %s in guild %s, channel %s (tracked).\n", userID, guildID, voiceChannelID)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)

	userStreak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{
		UserID:  userID,
		GuildID: guildID,
	})
	isNewStreakRecord := false
	if err != nil {
		if err == sql.ErrNoRows {
			isNewStreakRecord = true
			// Initialize a new streak object for internal logic, will be upserted
			userStreak = database.UserStreak{
				UserID:             userID,
				GuildID:            guildID,
				CurrentStreakCount: 0, // Will be set to 1
				MaxStreakCount:     0, // Will be set to 1
				// LastActivityDate will be set
				// StreakExtendedToday will be set
			}
		} else {
			return fmt.Errorf("failed to get user streak for user %s, guild %s: %w", userID, guildID, err)
		}
	}

	// If already extended today, nothing more to do.
	if userStreak.StreakExtendedToday && userStreak.LastActivityDate.Time.Equal(today) && !isNewStreakRecord {
		fmt.Printf("StreakService: Streak for user %s already extended today.\n", userID)
		return nil
	}

	var notificationMsg string
	params := database.UpsertUserStreakParams{
		UserID:              userID,
		GuildID:             guildID,
		LastActivityDate:    sql.NullTime{Time: today, Valid: true},
		StreakExtendedToday: true,
		WarningNotifiedAt:   sql.NullTime{Valid: false}, // Reset warning on activity
	}

	if isNewStreakRecord || userStreak.LastActivityDate.Time.Before(yesterday) {
		// Condition for new streak or streak was broken
		if !isNewStreakRecord && userStreak.CurrentStreakCount > 0 {
			s.sendStreakNotification(guildID, fmt.Sprintf("<@%s>'s previous streak of %d days has ended. They've started a new 1-day streak!", userID, userStreak.CurrentStreakCount))
		} else {
			notificationMsg = fmt.Sprintf("<@%s> has started a new study streak of 1 day!", userID)
		}
		params.CurrentStreakCount = 1
		params.MaxStreakCount = 1 // For upsert, GREATEST will handle existing max
	} else if userStreak.LastActivityDate.Time.Equal(yesterday) {
		// Streak continues from yesterday
		params.CurrentStreakCount = userStreak.CurrentStreakCount + 1
		params.MaxStreakCount = userStreak.MaxStreakCount // Upsert will use GREATEST
		if params.CurrentStreakCount > userStreak.MaxStreakCount {
			params.MaxStreakCount = params.CurrentStreakCount
		}
		notificationMsg = fmt.Sprintf("<@%s> is now on a %d day streak!", userID, params.CurrentStreakCount)
	} else if userStreak.LastActivityDate.Time.Equal(today) && !userStreak.StreakExtendedToday {
		// Active earlier today before flag reset, or first activity of the day. Maintain current streak.
		params.CurrentStreakCount = userStreak.CurrentStreakCount
		params.MaxStreakCount = userStreak.MaxStreakCount
		// No notification, just maintaining the streak for today.
		fmt.Printf("StreakService: User %s maintained streak for today, first qualifying action.\n", userID)
	} else {
		// Should not happen if logic above is correct, or already handled (e.g., already extended today)
		fmt.Printf("StreakService: User %s in guild %s - unhandled streak state or already processed. LastActivity: %v, ExtendedToday: %v\n", userID, guildID, userStreak.LastActivityDate.Time, userStreak.StreakExtendedToday)
		return nil
	}

	// Ensure MaxStreakCount is at least CurrentStreakCount during upsert logic.
	// The `GREATEST(user_streaks.max_streak_count, EXCLUDED.max_streak_count)` in SQLC handles this well for existing records.
	// For new records, we set MaxStreakCount = CurrentStreakCount initially.
	if isNewStreakRecord || params.MaxStreakCount < params.CurrentStreakCount {
		params.MaxStreakCount = params.CurrentStreakCount
	}

	updatedStreak, err := s.dbQueries.UpsertUserStreak(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to upsert user streak for user %s, guild %s: %w", userID, guildID, err)
	}

	fmt.Printf("StreakService: Upserted streak for user %s, guild %s. New count: %d, Max: %d, LastActivity: %v, ExtendedToday: %v\n",
		updatedStreak.UserID, updatedStreak.GuildID, updatedStreak.CurrentStreakCount, updatedStreak.MaxStreakCount, updatedStreak.LastActivityDate.Time, updatedStreak.StreakExtendedToday)

	if notificationMsg != "" {
		s.sendStreakNotification(guildID, notificationMsg)
	}

	return nil
}

// ResetDailyFlags resets the StreakExtendedToday flag for all users.
// Scheduled to run daily just after midnight UTC.
func (s *StreakService) ResetDailyFlags(ctx context.Context) error {
	fmt.Println("StreakService: Resetting all streak daily flags.")
	err := s.dbQueries.ResetAllStreakDailyFlags(ctx)
	if err != nil {
		return fmt.Errorf("failed to reset all streak daily flags: %w", err)
	}
	fmt.Println("StreakService: All streak daily flags reset.")
	return nil
}

// CheckAndHandleExpiredStreaks checks for streaks that were not continued.
// Scheduled to run hourly.
func (s *StreakService) CheckAndHandleExpiredStreaks(ctx context.Context) error {
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)

	streaksToReset, err := s.dbQueries.GetStreaksToReset(ctx, sql.NullTime{Time: yesterday, Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // No streaks to reset
		}
		return fmt.Errorf("failed to get streaks to reset (older than %v): %w", yesterday, err)
	}

	fmt.Printf("StreakService: Found %d streaks to potentially reset.\n", len(streaksToReset))
	for _, streak := range streaksToReset {
		// Double check: user might have just joined a VC and extended their streak
		// between the GetStreaksToReset query and now.
		// So, re-fetch the streak to get the absolute latest state.
		currentDBStreak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{
			UserID:  streak.UserID,
			GuildID: streak.GuildID,
		})
		if err != nil {
			fmt.Printf("Error re-fetching streak for user %s, guild %s before reset: %v\n", streak.UserID, streak.GuildID, err)
			continue
		}

		// If streak was extended today or last activity is not actually before yesterday, skip.
		if currentDBStreak.StreakExtendedToday && currentDBStreak.LastActivityDate.Time.Equal(time.Now().UTC().Truncate(24*time.Hour)) {
			fmt.Printf("StreakService: Streak for user %s, guild %s was extended recently. Skipping reset.\n", currentDBStreak.UserID, currentDBStreak.GuildID)
			continue
		}
		if !currentDBStreak.LastActivityDate.Time.Before(yesterday) {
			fmt.Printf("StreakService: Streak for user %s, guild %s LastActivityDate (%v) is not before yesterday (%v). Skipping reset.\n", currentDBStreak.UserID, currentDBStreak.GuildID, currentDBStreak.LastActivityDate.Time, yesterday)
			continue
		}

		if currentDBStreak.CurrentStreakCount > 0 { // Only notify if there was an active streak
			s.sendStreakNotification(streak.GuildID, fmt.Sprintf("<@%s>'s study streak of %d days has ended.", streak.UserID, currentDBStreak.CurrentStreakCount))
		}

		err = s.dbQueries.ResetUserStreakCount(ctx, database.ResetUserStreakCountParams{
			UserID:  streak.UserID,
			GuildID: streak.GuildID,
		})
		if err != nil {
			fmt.Printf("Error resetting streak count for user %s, guild %s: %v\n", streak.UserID, streak.GuildID, err)
			// Continue to next user
		} else {
			fmt.Printf("StreakService: Reset streak for user %s, guild %s.\n", streak.UserID, streak.GuildID)
		}
	}
	return nil
}

// SendStreakWarningNotifications checks and sends warnings for streaks about to end.
// Scheduled to run hourly, but only acts after a certain UTC hour.
func (s *StreakService) SendStreakWarningNotifications(ctx context.Context) error {
	now := time.Now().UTC()
	if now.Hour() < 22 { // Only send warnings from 22:00 UTC onwards
		// fmt.Println("StreakService: Not time for streak warnings yet.")
		return nil
	}

	// Warn if not notified in the last 23 hours (to avoid spam if task runs multiple times in warning window)
	notificationCutoff := now.Add(-23 * time.Hour)

	streaksToWarn, err := s.dbQueries.GetStreaksToWarn(ctx, sql.NullTime{Time: notificationCutoff, Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // No streaks to warn
		}
		return fmt.Errorf("failed to get streaks to warn (notified before %v): %w", notificationCutoff, err)
	}

	fmt.Printf("StreakService: Found %d streaks to potentially warn.\n", len(streaksToWarn))
	for _, streak := range streaksToWarn {
		// Additional check: ensure streak_extended_today is still false for today.
		// This handles the case where a user might have become active *after* GetStreaksToWarn ran
		// but *before* this specific user is processed in the loop.
		currentDBStreak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{UserID: streak.UserID, GuildID: streak.GuildID})
		if err != nil {
			fmt.Printf("Error re-fetching streak for user %s, guild %s before warning: %v\n", streak.UserID, streak.GuildID, err)
			continue
		}

		if currentDBStreak.StreakExtendedToday && currentDBStreak.LastActivityDate.Time.Equal(now.Truncate(24*time.Hour)) {
			fmt.Printf("StreakService: Streak for user %s, guild %s was extended today recently. Skipping warning.\n", currentDBStreak.UserID, currentDBStreak.GuildID)
			continue
		}

		warningMsg := fmt.Sprintf("<@%s>, your study streak of %d days is about to end! Join a tracked voice channel in the next ~%d hours (before midnight UTC) to keep it going!",
			streak.UserID, streak.CurrentStreakCount, 24-now.Hour())
		s.sendStreakNotification(streak.GuildID, warningMsg)

		err = s.dbQueries.UpdateStreakWarningNotifiedAt(ctx, database.UpdateStreakWarningNotifiedAtParams{
			WarningNotifiedAt: sql.NullTime{Time: now, Valid: true},
			UserID:            streak.UserID,
			GuildID:           streak.GuildID,
		})
		if err != nil {
			fmt.Printf("Error updating warning_notified_at for user %s, guild %s: %v\n", streak.UserID, streak.GuildID, err)
		} else {
			fmt.Printf("StreakService: Sent warning to user %s, guild %s.\n", streak.UserID, streak.GuildID)
		}
	}
	return nil
}

// GetUserStreakInfoText returns a string describing the user's current streak.
func (s *StreakService) GetUserStreakInfoText(ctx context.Context, userID, guildID string) (string, error) {
	streak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{
		UserID:  userID,
		GuildID: guildID,
	})
	if err != nil {
		if err == sql.ErrNoRows || (err != nil && streak.CurrentStreakCount == 0) { // Check CurrentStreakCount too from a potentially non-nil but zeroed streak
			return fmt.Sprintf("<@%s> has no active study streak.", userID), nil
		}
		return "", fmt.Errorf("failed to get user streak for user %s, guild %s: %w", userID, guildID, err)
	}

	if streak.CurrentStreakCount == 0 {
		return fmt.Sprintf("<@%s> has no active study streak, but their longest was %d days.", userID, streak.MaxStreakCount), nil
	}

	return fmt.Sprintf("<@%s> is currently on a %d day study streak! Their longest streak is %d days.", userID, streak.CurrentStreakCount, streak.MaxStreakCount), nil
}

// sendStreakNotification sends a message to the configured streak notification channel.
func (s *StreakService) sendStreakNotification(guildID, message string) {
	if s.streakNotificationChannel == "" {
		fmt.Println("StreakService: STREAK_NOTIFICATION_CHANNEL_ID not set. Cannot send streak notification.")
		return
	}

	// Future enhancement: Check if the s.streakNotificationChannel is part of the given guildID
	// For now, it's a global channel. If the bot is in many guilds, this global channel might not be ideal.
	// However, per user's current setup (single LOGGING_CHANNEL_ID), this global approach is consistent.

	_, err := s.discordSession.ChannelMessageSend(s.streakNotificationChannel, message)
	if err != nil {
		fmt.Printf("Error sending streak notification to channel %s: %v\nMessage: %s\n", s.streakNotificationChannel, err, message)
	}
}
