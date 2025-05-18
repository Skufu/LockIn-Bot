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

const (
	streakWindow       = 24 * time.Hour
	warningWindowStart = 22 * time.Hour // Warn if streak ends in < 2 hours (24-22)
	warningCooldown    = 23 * time.Hour // Don't re-warn if a warning was sent in the last 23 hours
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
		cronScheduler:             cron.New(cron.WithLocation(time.UTC)),
	}
}

func (s *StreakService) HandleVoiceActivity(ctx context.Context, userID, guildID, voiceChannelID string, activityTime time.Time) error {
	if voiceChannelID == "" {
		return nil
	}

	if _, tracked := s.trackedVoiceChannelIDs[voiceChannelID]; !tracked {
		return nil
	}

	fmt.Printf("StreakService: Handling voice activity for user %s in guild %s, channel %s at %v.\n", userID, guildID, voiceChannelID, activityTime)

	userStreak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{
		UserID:  userID,
		GuildID: guildID,
	})

	isNewStreakRecord := false
	if err != nil {
		if err == sql.ErrNoRows {
			isNewStreakRecord = true
			userStreak = database.GetUserStreakRow{
				UserID:             userID,
				GuildID:            guildID,
				CurrentStreakCount: 0,
				MaxStreakCount:     0,
			}
		} else {
			return fmt.Errorf("failed to get user streak for user %s, guild %s: %w", userID, guildID, err)
		}
	}

	var embedToSend *discordgo.MessageEmbed
	params := database.UpsertUserStreakParams{
		UserID:                      userID,
		GuildID:                     guildID,
		LastStreakActivityTimestamp: sql.NullTime{Time: activityTime, Valid: true},
		WarningNotifiedAt:           sql.NullTime{Valid: false}, // Reset warning on activity
	}

	if isNewStreakRecord || !userStreak.LastStreakActivityTimestamp.Valid { // Brand new user for streaks or no prior qualifying activity
		embedToSend = s.newStreakStartedEmbed(userID, 1)
		params.CurrentStreakCount = 1
		// If userStreak exists but TS was invalid, keep old MaxStreakCount if it was higher
		if isNewStreakRecord || userStreak.MaxStreakCount < 1 {
			params.MaxStreakCount = 1
		} else {
			params.MaxStreakCount = userStreak.MaxStreakCount
		}
		fmt.Printf("StreakService: User %s started a new streak of 1 (new record or invalid prior TS).\n", userID)
	} else {
		// Existing streak with a valid last activity timestamp
		previousActivityTime := userStreak.LastStreakActivityTimestamp.Time
		timeSinceLastActivity := activityTime.Sub(previousActivityTime)

		if timeSinceLastActivity > streakWindow { // Streak was broken
			if userStreak.CurrentStreakCount > 0 {
				s.sendStreakEmbed(guildID, s.streakEndedEmbed(userID, userStreak.CurrentStreakCount))
				fmt.Printf("StreakService: User %s broke streak of %d. Starting new streak of 1.\n", userID, userStreak.CurrentStreakCount)
			} else {
				fmt.Printf("StreakService: User %s had no active streak (or TS was old). Starting new streak of 1.\n", userID)
			}
			embedToSend = s.newStreakAfterBreakEmbed(userID, userStreak.CurrentStreakCount)
			params.CurrentStreakCount = 1
			params.MaxStreakCount = userStreak.MaxStreakCount // Preserve historical max
			if params.MaxStreakCount < 1 {                    // Ensure max is at least 1
				params.MaxStreakCount = 1
			}
		} else { // Activity is WITHIN 24 hours of the last activity - MAINTAIN streak
			params.CurrentStreakCount = userStreak.CurrentStreakCount // Keep existing count
			params.MaxStreakCount = userStreak.MaxStreakCount         // Keep existing max

			// If current streak count was 0 (e.g. reset by cron, then user joins again soon within 24h of that reset time event)
			// it should become 1, as this is the first activity to re-establish it.
			if params.CurrentStreakCount == 0 {
				params.CurrentStreakCount = 1
				if params.MaxStreakCount < 1 { // Ensure max streak is at least 1
					params.MaxStreakCount = 1
				}
				embedToSend = s.newStreakStartedEmbed(userID, 1)
				fmt.Printf("StreakService: User %s started new streak of 1 (was 0, now maintained within 24h of a reset/non-event).\n", userID)
			} else {
				// Streak is simply maintained, no increment, no specific embed to avoid spam.
				embedToSend = nil
				fmt.Printf("StreakService: User %s MAINTAINED streak of %d. Last activity updated. (Activity at %v, previous at %v)\n", userID, userStreak.CurrentStreakCount, activityTime, previousActivityTime)
			}
		}
	}

	// Final MaxStreakCount adjustment: ensure it's at least the current streak, and not less than a previously higher max.
	if !isNewStreakRecord && userStreak.MaxStreakCount > params.MaxStreakCount {
		params.MaxStreakCount = userStreak.MaxStreakCount
	}
	if params.CurrentStreakCount > params.MaxStreakCount {
		params.MaxStreakCount = params.CurrentStreakCount
	}
	// Ensure MaxStreakCount is at least 1 if CurrentStreakCount is 1.
	if params.CurrentStreakCount >= 1 && params.MaxStreakCount < 1 {
		params.MaxStreakCount = 1
	}

	updatedStreak, err := s.dbQueries.UpsertUserStreak(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to upsert user streak for user %s, guild %s: %w", userID, guildID, err)
	}

	fmt.Printf("StreakService: Upserted streak for user %s, guild %s. New count: %d, Max: %d, LastActivity: %v\n",
		updatedStreak.UserID, updatedStreak.GuildID, updatedStreak.CurrentStreakCount, updatedStreak.MaxStreakCount, updatedStreak.LastStreakActivityTimestamp.Time)

	if embedToSend != nil {
		s.sendStreakEmbed(guildID, embedToSend)
	}

	return nil
}

func (s *StreakService) StartScheduledTasks() {
	logMsg := "StreakService: Scheduled task %s (%s) %s."
	var err error

	// Check and reset expired streaks (e.g., hourly)
	_, err = s.cronScheduler.AddFunc("@hourly", func() {
		fmt.Println("StreakService: Running hourly check for expired streaks...")
		ctx := context.Background()
		s.CheckAndHandleExpiredStreaks(ctx)
	})
	if err != nil {
		fmt.Printf(logMsg, "CheckAndHandleExpiredStreaks", "@hourly", fmt.Sprintf("failed to add: %v", err))
	} else {
		fmt.Printf(logMsg, "CheckAndHandleExpiredStreaks", "@hourly", "added successfully")
	}

	// Send streak warnings (e.g., hourly)
	_, err = s.cronScheduler.AddFunc("@hourly", func() { // Can adjust schedule
		fmt.Println("StreakService: Running hourly check for streak warnings...")
		ctx := context.Background()
		s.SendStreakWarningNotifications(ctx)
	})
	if err != nil {
		fmt.Printf(logMsg, "SendStreakWarningNotifications", "@hourly", fmt.Sprintf("failed to add: %v", err))
	} else {
		fmt.Printf(logMsg, "SendStreakWarningNotifications", "@hourly", "added successfully")
	}

	s.cronScheduler.Start()
	fmt.Println("StreakService: Cron scheduler started with UTC location.")
}

func (s *StreakService) StopScheduledTasks() {
	if s.cronScheduler != nil {
		fmt.Println("StreakService: Stopping cron scheduler...")
		ctx := s.cronScheduler.Stop()
		<-ctx.Done()
		fmt.Println("StreakService: Cron scheduler stopped.")
	}
}

func (s *StreakService) CheckAndHandleExpiredStreaks(ctx context.Context) {
	streaksToReset, err := s.dbQueries.GetStreaksToReset(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Println("StreakService: No streaks found to reset.")
			return
		}
		fmt.Printf("Error in CheckAndHandleExpiredStreaks getting streaks to reset: %v\n", err)
		return
	}

	fmt.Printf("StreakService: Found %d streaks to potentially reset.\n", len(streaksToReset))
	for _, streak := range streaksToReset {
		// Additional check: ensure the streak wasn't updated since GetStreaksToReset was called.
		// This is a safeguard against race conditions, though less likely with NOW() in query.
		currentDBStreak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{
			UserID:  streak.UserID,
			GuildID: streak.GuildID,
		})
		if err != nil {
			fmt.Printf("Error re-fetching streak for user %s, guild %s before reset: %v\n", streak.UserID, streak.GuildID, err)
			continue
		}

		if currentDBStreak.LastStreakActivityTimestamp.Valid &&
			time.Now().Sub(currentDBStreak.LastStreakActivityTimestamp.Time) <= streakWindow {
			fmt.Printf("StreakService: Streak for user %s, guild %s was updated recently (%v). Skipping reset.\n", currentDBStreak.UserID, currentDBStreak.GuildID, currentDBStreak.LastStreakActivityTimestamp.Time)
			continue
		}

		if currentDBStreak.CurrentStreakCount > 0 {
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
}

func (s *StreakService) SendStreakWarningNotifications(ctx context.Context) {
	now := time.Now().UTC()

	streaksToWarn, err := s.dbQueries.GetStreaksToWarn(ctx) // Query now uses NOW() for its window
	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Println("StreakService: No streaks found to warn.")
			return
		}
		fmt.Printf("Error in SendStreakWarningNotifications getting streaks: %v\n", err)
		return
	}

	fmt.Printf("StreakService: Found %d streaks to potentially warn.\n", len(streaksToWarn))
	for _, streak := range streaksToWarn {
		currentDBStreak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{UserID: streak.UserID, GuildID: streak.GuildID})
		if err != nil {
			fmt.Printf("Error re-fetching streak for user %s, guild %s before warning: %v\n", streak.UserID, streak.GuildID, err)
			continue
		}

		// Ensure streak is still active and warning is still relevant
		if !currentDBStreak.LastStreakActivityTimestamp.Valid ||
			now.Sub(currentDBStreak.LastStreakActivityTimestamp.Time) >= streakWindow { // Streak already ended or activity is too new
			fmt.Printf("StreakService: Streak for user %s, guild %s already ended or was updated. Skipping warning.\n", currentDBStreak.UserID, currentDBStreak.GuildID)
			continue
		}

		// Check if already warned recently
		if currentDBStreak.WarningNotifiedAt.Valid && now.Sub(currentDBStreak.WarningNotifiedAt.Time) < warningCooldown {
			fmt.Printf("StreakService: User %s, guild %s was warned recently at %v. Skipping warning.\n", currentDBStreak.UserID, currentDBStreak.GuildID, currentDBStreak.WarningNotifiedAt.Time)
			continue
		}

		hoursRemaining := int(streakWindow.Hours() - now.Sub(currentDBStreak.LastStreakActivityTimestamp.Time).Hours())
		if hoursRemaining <= 0 {
			hoursRemaining = 1
		} // for phrasing

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
}

func (s *StreakService) GetUserStreakInfoEmbed(ctx context.Context, userID, guildID string) (*discordgo.MessageEmbed, error) {
	streak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{
		UserID:  userID,
		GuildID: guildID,
	})

	discordUser, errUser := s.discordSession.User(userID)
	username := userID
	if errUser == nil && discordUser != nil {
		username = discordUser.Username
	}

	title := fmt.Sprintf("🎯 Streak Status for %s", username)
	description := ""
	color := 0xAAAAAA

	if err != nil {
		if err == sql.ErrNoRows {
			description = fmt.Sprintf("<@%s> hasn't started a study streak yet. Time to lock in!", userID)
		} else {
			return nil, fmt.Errorf("failed to get user streak info for %s, guild %s: %w", userID, guildID, err)
		}
	} else {
		if streak.CurrentStreakCount > 0 {
			if streak.LastStreakActivityTimestamp.Valid && time.Now().UTC().Sub(streak.LastStreakActivityTimestamp.Time) <= streakWindow {
				// Active streak
				title = fmt.Sprintf("🔥 Streak Status for %s 🔥", username)
				description = fmt.Sprintf("<@%s> is currently on a **%d day** study streak! 🎉", userID, streak.CurrentStreakCount)
				timeLeft := streakWindow - time.Now().UTC().Sub(streak.LastStreakActivityTimestamp.Time)
				description += fmt.Sprintf("\nTime left to continue: **%s**", formatDurationSimple(timeLeft))
				color = 0x00FF00 // Green
			} else {
				// Streak record exists, count > 0, but last activity too old (edge case, should be reset by cron)
				description = fmt.Sprintf("<@%s> had a streak of **%d days**. It seems to have ended. Start a new one!", userID, streak.CurrentStreakCount)
				if streak.MaxStreakCount > 0 {
					description += fmt.Sprintf(" Their longest was **%d days**.", streak.MaxStreakCount)
				}
				color = 0xFFA500 // Orange
			}

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
			if streak.MaxStreakCount > 0 {
				description = fmt.Sprintf("<@%s> has no active study streak currently. Their longest streak was **%d days**. Let's get back on track!", userID, streak.MaxStreakCount)
				color = 0xFFA500
			} else {
				description = fmt.Sprintf("<@%s> hasn't recorded a streak yet. Join a voice channel to start one!", userID)
			}
		}
	}

	return &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       color,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "LockIn to build your streak!"},
	}, nil
}

func (s *StreakService) sendStreakEmbed(guildID string, embed *discordgo.MessageEmbed) {
	if s.streakNotificationChannel == "" {
		fmt.Println("StreakService: Streak notification channel ID is not configured. Cannot send embed.")
		return
	}
	// Try to find the configured channel by ID. If not found, log and potentially try a default channel.
	_, err := s.discordSession.ChannelMessageSendEmbed(s.streakNotificationChannel, embed)
	if err != nil {
		fmt.Printf("StreakService: Failed to send streak embed to channel %s: %v. Attempting to find a general channel.\n", s.streakNotificationChannel, err)
		// Fallback: Try to find the first available text channel in the guild to send the message
		channels, _ := s.discordSession.GuildChannels(guildID)
		for _, ch := range channels {
			if ch.Type == discordgo.ChannelTypeGuildText {
				_, errAlt := s.discordSession.ChannelMessageSendEmbed(ch.ID, embed)
				if errAlt == nil {
					fmt.Printf("StreakService: Successfully sent streak embed to fallback channel %s (%s).\n", ch.Name, ch.ID)
					return
				}
			}
		}
		fmt.Printf("StreakService: Could not find any suitable channel in guild %s to send streak embed.\n", guildID)
	}
}

func (s *StreakService) newStreakStartedEmbed(userID string, streakCount int32) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "🚀 New Streak Started! 🚀",
		Description: fmt.Sprintf("<@%s> has just started a new study streak! Currently **%d day(s)** strong. Let's go!", userID, streakCount),
		Color:       0x7CFC00, // Lawngreen
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *StreakService) newStreakAfterBreakEmbed(userID string, previousStreakCount int32) *discordgo.MessageEmbed {
	msg := fmt.Sprintf("<@%s> is starting a fresh streak of **1 day**!", userID)
	if previousStreakCount > 0 {
		msg = fmt.Sprintf("<@%s> broke their streak of %d days, but is back on track starting a new one of **1 day**!", userID, previousStreakCount)
	}
	return &discordgo.MessageEmbed{
		Title:       "🚀 Back At It! New Streak! 🚀",
		Description: msg,
		Color:       0x1E90FF, // DodgerBlue
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *StreakService) streakContinuedEmbed(userID string, streakCount int32) *discordgo.MessageEmbed {
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
		// Add more milestones if desired
	}

	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s Streak Continued! %s", milestoneEmoji, milestoneEmoji),
		Description: fmt.Sprintf("<@%s> is now on a **%d day** streak!%s Keep the momentum!", userID, streakCount, milestoneMsg),
		Color:       0x00AAFF,
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
		Description: fmt.Sprintf("<@%s>, your **%d day** study streak is in danger! You have about **%d hour%s** left to join a tracked voice channel and keep it alive!", userID, streakCount, hoursRemaining, pluralS),
		Color:       0xFFA500,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *StreakService) streakEndedEmbed(userID string, lastStreakCount int32) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "💔 Streak Ended 💔",
		Description: fmt.Sprintf("Oh no! <@%s>'s impressive study streak of **%d days** has come to an end. Don't give up, start a new one soon!", userID, lastStreakCount),
		Color:       0xFF0000,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// formatDurationSimple formats duration to Hh Mm Ss
func formatDurationSimple(d time.Duration) string {
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
