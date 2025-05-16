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
	_, err = s.cronScheduler.AddFunc("@hourly", func() { // ORIGINAL SCHEDULE
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
				CurrentStreakCount: 0,
				MaxStreakCount:     0,
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

	var embedToSend *discordgo.MessageEmbed
	params := database.UpsertUserStreakParams{
		UserID:              userID,
		GuildID:             guildID,
		LastActivityDate:    sql.NullTime{Time: today, Valid: true},
		StreakExtendedToday: true,
		WarningNotifiedAt:   sql.NullTime{Valid: false}, // Reset warning on activity
	}

	if isNewStreakRecord { // Brand new user for streaks
		embedToSend = s.newStreakStartedEmbed(userID, 1)
		params.CurrentStreakCount = 1
		params.MaxStreakCount = 1
	} else if userStreak.LastActivityDate.Time.Before(yesterday) { // Streak was broken
		// Send ended message first, then the new streak message
		if userStreak.CurrentStreakCount > 0 {
			s.sendStreakEmbed(guildID, s.streakEndedEmbed(userID, userStreak.CurrentStreakCount))
		}
		embedToSend = s.newStreakAfterBreakEmbed(userID, userStreak.CurrentStreakCount) // Special message for immediate new streak
		params.CurrentStreakCount = 1
		params.MaxStreakCount = userStreak.MaxStreakCount // Preserve historical max streak
		if params.MaxStreakCount < 1 {                    // Ensure max streak is at least 1 if it was 0 for some reason
			params.MaxStreakCount = 1
		}
	} else if userStreak.LastActivityDate.Time.Equal(yesterday) { // Streak continues from yesterday
		params.CurrentStreakCount = userStreak.CurrentStreakCount + 1
		if params.CurrentStreakCount > userStreak.MaxStreakCount {
			params.MaxStreakCount = params.CurrentStreakCount
		} else {
			params.MaxStreakCount = userStreak.MaxStreakCount // Preserve historical if current is not greater
		}
		embedToSend = s.streakContinuedEmbed(userID, params.CurrentStreakCount)
	} else if userStreak.LastActivityDate.Time.Equal(today) && !userStreak.StreakExtendedToday { // Active earlier today or first activity of the day
		params.CurrentStreakCount = userStreak.CurrentStreakCount
		params.MaxStreakCount = userStreak.MaxStreakCount
		// No embed needed here if streak already existed and is just being marked as extended for the day
		// However, if it's the *first* action of the day that *starts* the count for today, it's handled by other conditions.
		// This condition primarily ensures StreakExtendedToday is set.
		// If current streak count is 0 (e.g. after a reset, and this is first activity), logic should have gone to newStreakRecord or broken streak path.
		// If it was 0 and somehow lands here, setting to 1.
		if params.CurrentStreakCount == 0 {
			params.CurrentStreakCount = 1
			if params.MaxStreakCount < 1 {
				params.MaxStreakCount = 1
			}
			// This could happen if a streak was reset, and then user joins on the *same day* of the reset.
			// It wouldn't be "yesterday" or "before yesterday".
			embedToSend = s.newStreakStartedEmbed(userID, 1)
		} else {
			// If an existing streak is simply being marked as extended for the day (already > 0), no new embed.
			// Embeds are for: new streak, continued from yesterday, broken then new.
			fmt.Printf("StreakService: User %s maintained streak for today (%d days), first qualifying action marking extended_today=true.\n", userID, params.CurrentStreakCount)
		}
	} else {
		fmt.Printf("StreakService: User %s in guild %s - unhandled streak state or already processed. LastActivity: %v, ExtendedToday: %v\n", userID, guildID, userStreak.LastActivityDate.Time, userStreak.StreakExtendedToday)
		return nil
	}

	// This final check ensures MaxStreakCount is always at least CurrentStreakCount.
	// This was simplified from previous logic as MaxStreakCount is now more carefully managed in the conditions above.
	if params.MaxStreakCount < params.CurrentStreakCount {
		params.MaxStreakCount = params.CurrentStreakCount
	}

	updatedStreak, err := s.dbQueries.UpsertUserStreak(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to upsert user streak for user %s, guild %s: %w", userID, guildID, err)
	}

	fmt.Printf("StreakService: Upserted streak for user %s, guild %s. New count: %d, Max: %d, LastActivity: %v, ExtendedToday: %v\n",
		updatedStreak.UserID, updatedStreak.GuildID, updatedStreak.CurrentStreakCount, updatedStreak.MaxStreakCount, updatedStreak.LastActivityDate.Time, updatedStreak.StreakExtendedToday)

	if embedToSend != nil {
		s.sendStreakEmbed(guildID, embedToSend)
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
// This now assumes GetStreaksToReset fetches users where streak_extended_today = FALSE
// and last_activity_date < today_start_date (passed as TodayStartDate param).
func (s *StreakService) CheckAndHandleExpiredStreaks(ctx context.Context) error {
	todayStartDate := time.Now().UTC().Truncate(24 * time.Hour)

	// Parameter name for GetStreaksToResetParams should match what sqlc generates
	// based on the `@today_start_date` in your SQL query. Assuming it's TodayStartDate.
	streaksToReset, err := s.dbQueries.GetStreaksToReset(ctx, sql.NullTime{Time: todayStartDate, Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // No streaks to reset
		}
		return fmt.Errorf("failed to get streaks to reset (where last_activity_date < %v and not extended): %w", todayStartDate, err)
	}

	fmt.Printf("StreakService: Found %d streaks to potentially reset (not extended today and last active before %v).\n", len(streaksToReset), todayStartDate)
	for _, streak := range streaksToReset {
		// Re-fetch the streak to get the absolute latest state, to avoid race conditions
		// where a user might have just joined a VC and extended their streak.
		currentDBStreak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{
			UserID:  streak.UserID,
			GuildID: streak.GuildID,
		})
		if err != nil {
			fmt.Printf("Error re-fetching streak for user %s, guild %s before reset: %v\n", streak.UserID, streak.GuildID, err)
			continue
		}

		// If streak_extended_today is true OR last_activity_date is not actually before today_start_date,
		// then this streak was updated after GetStreaksToReset was called. Skip.
		if currentDBStreak.StreakExtendedToday || !currentDBStreak.LastActivityDate.Time.Before(todayStartDate) {
			fmt.Printf("StreakService: Streak for user %s, guild %s was extended or activity is recent (%v, extended: %t). Skipping reset.\n", currentDBStreak.UserID, currentDBStreak.GuildID, currentDBStreak.LastActivityDate.Time, currentDBStreak.StreakExtendedToday)
			continue
		}

		// If we are here, the streak is genuinely expired.
		if currentDBStreak.CurrentStreakCount > 0 { // Only notify if there was an active streak
			s.sendStreakEmbed(streak.GuildID, s.streakEndedEmbed(streak.UserID, currentDBStreak.CurrentStreakCount))
		}

		err = s.dbQueries.ResetUserStreakCount(ctx, database.ResetUserStreakCountParams{
			UserID:  streak.UserID,
			GuildID: streak.GuildID,
		})
		if err != nil {
			fmt.Printf("Error resetting streak count for user %s, guild %s: %v\n", streak.UserID, streak.GuildID, err)
		} else {
			fmt.Printf("StreakService: Reset streak for user %s, guild %s. Previous count was %d.\n", streak.UserID, streak.GuildID, currentDBStreak.CurrentStreakCount)
		}
	}
	return nil
}

// SendStreakWarningNotifications checks and sends warnings for streaks about to end.
// Scheduled to run hourly, but only acts after a certain UTC hour.
func (s *StreakService) SendStreakWarningNotifications(ctx context.Context) error {
	now := time.Now().UTC()
	// TEMPORARY: Commenting out time restriction for quick testing
	if now.Hour() < 22 { // Only send warnings from 22:00 UTC onwards
		// fmt.Println("StreakService: Not time for streak warnings yet.")
		return nil
	}
	// fmt.Println("StreakService: Warning time check bypassed for testing.") // TEMPORARY LOG

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

		hoursRemaining := 24 - now.Hour()
		if hoursRemaining <= 0 {
			hoursRemaining = 1
		} // Ensure at least 1 hour for phrasing if very close to midnight
		s.sendStreakEmbed(streak.GuildID, s.streakWarningEmbed(streak.UserID, currentDBStreak.CurrentStreakCount, hoursRemaining))

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

// GetUserStreakInfoEmbed constructs an embed for the /streak command.
func (s *StreakService) GetUserStreakInfoEmbed(ctx context.Context, userID, guildID string) (*discordgo.MessageEmbed, error) {
	streak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{
		UserID:  userID,
		GuildID: guildID,
	})

	discordUser, errUser := s.discordSession.User(userID)
	username := userID // Fallback to ID
	if errUser == nil && discordUser != nil {
		username = discordUser.Username
	}

	title := fmt.Sprintf("🎯 Streak Status for %s", username)
	description := ""
	color := 0xAAAAAA // Grey by default

	if err != nil {
		if err == sql.ErrNoRows {
			// Case 1: User has no streak record at all.
			description = fmt.Sprintf("<@%s> hasn't started a study streak yet. Time to lock in!", userID)
		} else {
			// Other database error
			return nil, fmt.Errorf("failed to get user streak for user %s, guild %s: %w", userID, guildID, err)
		}
	} else {
		if streak.CurrentStreakCount > 0 {
			// Case 2: User has an active streak.
			title = fmt.Sprintf("🔥 Streak Status for %s 🔥", username)
			description = fmt.Sprintf("<@%s> is currently on a **%d day** study streak! 🎉", userID, streak.CurrentStreakCount)
			color = 0x00FF00 // Green
			return &discordgo.MessageEmbed{
				Title:       title,
				Description: description,
				Fields: []*discordgo.MessageEmbedField{
					{
						Name:   "Current Streak",
						Value:  fmt.Sprintf("%d days", streak.CurrentStreakCount),
						Inline: true,
					},
					{
						Name:   "Longest Streak",
						Value:  fmt.Sprintf("%d days", streak.MaxStreakCount),
						Inline: true,
					},
				},
				Color:     color,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Footer:    &discordgo.MessageEmbedFooter{Text: "Keep Locking In!"},
			}, nil
		} else {
			// Case 3: User has a record, but CurrentStreakCount is 0.
			if streak.MaxStreakCount > 0 {
				description = fmt.Sprintf("<@%s> has no active study streak currently. Their longest streak was **%d days**. Let's get back on track!", userID, streak.MaxStreakCount)
				color = 0xFFA500 // Orange for encouragement
			} else {
				description = fmt.Sprintf("<@%s> hasn't recorded a streak yet. Join a voice channel to start one!", userID)
			}
		}
	}

	// Fallback embed for no active streak / no record initially
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       color,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "LockIn to build your streak!"},
	}, nil
}

// sendStreakEmbed sends a pre-built embed to the configured streak notification channel.
func (s *StreakService) sendStreakEmbed(guildID string, embed *discordgo.MessageEmbed) {
	if s.streakNotificationChannel == "" {
		fmt.Println("StreakService: STREAK_NOTIFICATION_CHANNEL_ID not set. Cannot send streak embed.")
		return
	}
	if embed == nil {
		fmt.Println("StreakService: Attempted to send a nil embed.")
		return
	}

	_, err := s.discordSession.ChannelMessageSendEmbed(s.streakNotificationChannel, embed)
	if err != nil {
		fmt.Printf("Error sending streak embed to channel %s: %v\nEmbed Title: %s\n", s.streakNotificationChannel, err, embed.Title)
	}
}

// --- Embed Helper Functions ---

func (s *StreakService) newStreakStartedEmbed(userID string, streakCount int32) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "✨ New Streak Started! ✨",
		Description: fmt.Sprintf("<@%s> has embarked on a new study journey, starting a **%d day** streak! Let's go!", userID, streakCount),
		Color:       0x00FF00, // Green
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *StreakService) newStreakAfterBreakEmbed(userID string, oldStreakCount int32) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "🚀 Back on Track! 🚀",
		Description: fmt.Sprintf("After a previous streak of %d days, <@%s> is starting fresh with a new **1 day** streak! Welcome back!", oldStreakCount, userID),
		Color:       0x00FF00, // Green
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *StreakService) streakContinuedEmbed(userID string, streakCount int32) *discordgo.MessageEmbed {
	// Milestone checks
	milestoneEmoji := "🔥"
	milestoneMsg := ""
	switch streakCount {
	case 7:
		milestoneEmoji = "🌟"
		milestoneMsg = " Awesome! One week streak!"
	case 14:
		milestoneEmoji = "💫"
		milestoneMsg = " Two weeks strong!"
	case 30:
		milestoneEmoji = "🏆"
		milestoneMsg = " Incredible! One month streak!"
	case 50:
		milestoneEmoji = "💯"
		milestoneMsg = " Halfway to 100! Amazing!"
	case 100:
		milestoneEmoji = "👑"
		milestoneMsg = " LEGENDARY! 100 Day Streak!"
	}

	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s Streak Continued! %s", milestoneEmoji, milestoneEmoji),
		Description: fmt.Sprintf("<@%s> is now on a **%d day** streak!%s Keep the momentum!", userID, streakCount, milestoneMsg),
		Color:       0x00AAFF, // Blue
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *StreakService) streakWarningEmbed(userID string, streakCount int32, hoursRemaining int) *discordgo.MessageEmbed {
	pluralS := "s"
	if hoursRemaining == 1 {
		pluralS = ""
	}
	return &discordgo.MessageEmbed{
		Title:       "⏳ Streak Warning! ⏳",
		Description: fmt.Sprintf("<@%s>, your **%d day** study streak is in danger! You have about **%d hour%s** left (until midnight UTC) to join a tracked voice channel and keep it alive!", userID, streakCount, hoursRemaining, pluralS),
		Color:       0xFFA500, // Orange
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *StreakService) streakEndedEmbed(userID string, lastStreakCount int32) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "💔 Streak Ended 💔",
		Description: fmt.Sprintf("Oh no! <@%s>'s impressive study streak of **%d days** has come to an end. Don't give up, start a new one soon!", userID, lastStreakCount),
		Color:       0xFF0000, // Red
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}
