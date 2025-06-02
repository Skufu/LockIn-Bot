package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/config"
	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/bwmarrin/discordgo"
	"github.com/robfig/cron/v3"
)

const (
	minimumActivityMinutes = 1
)

type StreakService struct {
	dbQueries                 *database.Queries
	discordSession            *discordgo.Session
	cfg                       *config.Config
	trackedVoiceChannelIDs    map[string]struct{}
	streakNotificationChannel string
	cronScheduler             *cron.Cron

	bot interface { // Interface to access Bot's session timing
		GetSessionStartTime(userID string) (time.Time, bool)
	}
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
		cronScheduler:             cron.New(cron.WithLocation(GetManilaLocation())),
		bot:                       nil, // Set later with SetBot
	}
}

// SetBot sets the bot reference to access session timing
func (s *StreakService) SetBot(bot interface {
	GetSessionStartTime(userID string) (time.Time, bool)
}) {
	s.bot = bot
}

// HandleVoiceJoin is called when a user joins a tracked voice channel
func (s *StreakService) HandleVoiceJoin(ctx context.Context, userID, guildID, voiceChannelID string) error {
	if voiceChannelID == "" {
		return nil
	}

	if _, tracked := s.trackedVoiceChannelIDs[voiceChannelID]; !tracked {
		return nil
	}

	todayDate := GetTodayManilaDate()
	now := GetManilaTimeNow()

	fmt.Printf("StreakService: User %s joined voice channel %s in guild %s at %v Manila time\n",
		userID, voiceChannelID, guildID, now.Format("2006-01-02 15:04:05"))

	// Check if user already has sufficient activity for today
	hasActivity, err := s.dbQueries.HasActivityForDate(ctx, database.HasActivityForDateParams{
		UserID:               userID,
		GuildID:              guildID,
		LastActivityDate:     sql.NullTime{Time: todayDate, Valid: true},
		DailyActivityMinutes: sql.NullInt32{Int32: int32(minimumActivityMinutes), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to check activity for date: %w", err)
	}

	if hasActivity {
		fmt.Printf("StreakService: User %s already has sufficient activity (%d+ minutes) for today. No tracking needed.\n",
			userID, minimumActivityMinutes)
		return nil
	}

	// Initialize or update the daily activity record
	_, err = s.dbQueries.StartDailyActivity(ctx, database.StartDailyActivityParams{
		UserID:            userID,
		GuildID:           guildID,
		LastActivityDate:  sql.NullTime{Time: todayDate, Valid: true},
		ActivityStartTime: sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to start daily activity tracking: %w", err)
	}

	fmt.Printf("StreakService: Started activity tracking for user %s in guild %s\n", userID, guildID)
	return nil
}

// HandleVoiceLeave is called when a user leaves a tracked voice channel
func (s *StreakService) HandleVoiceLeave(ctx context.Context, userID, guildID string) error {
	fmt.Printf("StreakService: User %s left voice channel in guild %s\n", userID, guildID)

	// Get session start time from Bot's tracking
	if s.bot == nil {
		return fmt.Errorf("bot reference not set in StreakService")
	}

	startTime, hasSession := s.bot.GetSessionStartTime(userID)
	if !hasSession {
		return nil // User wasn't in a tracked session
	}

	now := GetManilaTimeNow()
	sessionDuration := now.Sub(startTime)
	sessionMinutes := int(sessionDuration.Minutes())

	fmt.Printf("StreakService: Session duration: %d minutes\n", sessionMinutes)

	if sessionMinutes < 1 {
		return nil // Too short to count
	}

	// Get current activity for today to determine if we need to process anything
	streak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{
		UserID:  userID,
		GuildID: guildID,
	})
	if err != nil {
		// If user doesn't exist in streaks table, that's fine - they haven't started streaking yet
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("failed to get user streak: %w", err)
	}

	todayDate := GetTodayManilaDate()
	currentMinutes := int(streak.DailyActivityMinutes.Int32)

	// Only process if this is today's activity date
	if !streak.LastActivityDate.Valid ||
		!IsSameManilaDate(streak.LastActivityDate.Time, todayDate) {
		return nil
	}

	newTotalMinutes := currentMinutes + sessionMinutes

	// Update the daily activity minutes
	err = s.dbQueries.UpdateDailyActivityMinutes(ctx, database.UpdateDailyActivityMinutesParams{
		UserID:               userID,
		GuildID:              guildID,
		DailyActivityMinutes: sql.NullInt32{Int32: int32(newTotalMinutes), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to update daily activity minutes: %w", err)
	}

	fmt.Printf("StreakService: Updated daily activity for user %s in guild %s: %d total minutes\n",
		userID, guildID, newTotalMinutes)

	// If they just reached the minimum, send a notification and update streak immediately
	if currentMinutes < minimumActivityMinutes && newTotalMinutes >= minimumActivityMinutes {
		// Check if streak was already incremented today
		if !streak.StreakIncrementedToday {
			err = s.processImmediateStreakUpdate(ctx, userID, guildID, newTotalMinutes)
			if err != nil {
				fmt.Printf("StreakService: Error processing immediate streak update for user %s: %v\n", userID, err)
				// Still send basic completion message if streak update fails
				embed := s.basicDailyActivityCompletedEmbed(userID, newTotalMinutes)
				s.sendStreakEmbed(guildID, embed)
			}
		}
	}

	return nil
}

// StartScheduledTasks starts the cron jobs for daily evaluation and warnings
func (s *StreakService) StartScheduledTasks() {
	// Daily streak evaluation at 11:59 PM Manila time (end of day)
	_, err := s.cronScheduler.AddFunc("59 23 * * *", func() {
		fmt.Println("StreakService: Running daily streak evaluation at 11:59 PM Manila time...")
		ctx := context.Background()
		s.EvaluateAllUserStreaks(ctx)
	})
	if err != nil {
		fmt.Printf("StreakService: Failed to schedule daily evaluation: %v\n", err)
	} else {
		fmt.Println("StreakService: Scheduled daily evaluation at 11:59 PM Manila time")
	}

	// Evening warnings at 8:00 PM Manila time
	_, err = s.cronScheduler.AddFunc("0 20 * * *", func() {
		fmt.Println("StreakService: Running evening warning check at 8:00 PM Manila time...")
		ctx := context.Background()
		s.SendEveningWarnings(ctx)
	})
	if err != nil {
		fmt.Printf("StreakService: Failed to schedule evening warnings: %v\n", err)
	} else {
		fmt.Println("StreakService: Scheduled evening warnings at 8:00 PM Manila time")
	}

	s.cronScheduler.Start()
	fmt.Println("StreakService: Cron scheduler started with Manila timezone")
}

func (s *StreakService) StopScheduledTasks() {
	if s.cronScheduler != nil {
		fmt.Println("StreakService: Stopping cron scheduler...")
		ctx := s.cronScheduler.Stop()
		<-ctx.Done()
		fmt.Println("StreakService: Cron scheduler stopped")
	}
}

// EvaluateAllUserStreaks evaluates streaks for all users based on today's activity
func (s *StreakService) EvaluateAllUserStreaks(ctx context.Context) {
	todayDate := GetTodayManilaDate()

	fmt.Printf("StreakService: Running daily evaluation for %s Manila time\n", FormatManilaDate(todayDate))

	// Reset all daily flags at start of evaluation
	err := s.dbQueries.ResetAllStreakDailyFlags(ctx)
	if err != nil {
		fmt.Printf("StreakService: Error resetting daily flags: %v\n", err)
		return
	}
	fmt.Println("StreakService: Daily flags reset successfully")

	// Get ALL users who need evaluation for today (haven't been evaluated yet)
	users, err := s.dbQueries.GetUsersForDailyEvaluation(ctx, sql.NullTime{Time: todayDate, Valid: true})
	if err != nil {
		fmt.Printf("StreakService: Error getting users for daily evaluation: %v\n", err)
		return
	}

	fmt.Printf("StreakService: Found %d users to evaluate for today\n", len(users))

	// Process each user's streak based on TODAY's activity
	for _, user := range users {
		err = s.evaluateUserStreakForToday(ctx, user, todayDate)
		if err != nil {
			fmt.Printf("StreakService: Error evaluating streak for user %s: %v\n", user.UserID, err)
			continue
		}
	}

	fmt.Printf("StreakService: Daily evaluation completed for %s\n", FormatManilaDate(todayDate))
}

// evaluateUserStreakForToday evaluates a single user's streak based on today's activity
func (s *StreakService) evaluateUserStreakForToday(ctx context.Context, user database.GetUsersForDailyEvaluationRow, todayDate time.Time) error {
	userID := user.UserID
	guildID := user.GuildID

	// Check if user has sufficient activity for TODAY
	hasActivityToday := false
	if user.LastActivityDate.Valid &&
		IsSameManilaDate(user.LastActivityDate.Time, todayDate) &&
		user.DailyActivityMinutes.Valid &&
		user.DailyActivityMinutes.Int32 >= int32(minimumActivityMinutes) {
		hasActivityToday = true
	}

	var newStreakCount int32
	var notificationEmbed *discordgo.MessageEmbed

	if hasActivityToday {
		// User was active today - continue or increment streak
		if user.CurrentStreakCount == 0 {
			// Starting a new streak
			newStreakCount = 1
			notificationEmbed = s.newStreakStartedEmbed(userID, newStreakCount)
		} else {
			// Continuing existing streak
			newStreakCount = user.CurrentStreakCount + 1
			notificationEmbed = s.streakContinuedEmbed(userID, newStreakCount)
		}

		fmt.Printf("StreakService: User %s was active today (%d mins), streak: %d -> %d\n",
			userID, user.DailyActivityMinutes.Int32, user.CurrentStreakCount, newStreakCount)
	} else {
		// User was NOT active today - reset streak if they had one
		if user.CurrentStreakCount > 0 {
			newStreakCount = 0
			notificationEmbed = s.streakEndedEmbed(userID, user.CurrentStreakCount)
			fmt.Printf("StreakService: User %s was inactive today, streak reset from %d to 0\n",
				userID, user.CurrentStreakCount)
		} else {
			// User had no streak and was inactive - no change needed
			fmt.Printf("StreakService: User %s remains inactive (no streak to reset)\n", userID)
			return s.markUserEvaluated(ctx, userID, guildID, todayDate)
		}
	}

	// Update the user's streak in database
	newMaxStreak := user.MaxStreakCount
	if newStreakCount > newMaxStreak {
		newMaxStreak = newStreakCount
	}

	_, err := s.dbQueries.UpdateUserStreakAfterEvaluation(ctx, database.UpdateUserStreakAfterEvaluationParams{
		UserID:              userID,
		GuildID:             guildID,
		CurrentStreakCount:  newStreakCount,
		MaxStreakCount:      newMaxStreak,
		StreakEvaluatedDate: sql.NullTime{Time: todayDate, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to update streak after evaluation: %w", err)
	}

	// Send notification if we have one
	if notificationEmbed != nil {
		s.sendStreakEmbed(guildID, notificationEmbed)
	}

	return nil
}

// markUserEvaluated marks a user as evaluated for today without changing their streak
func (s *StreakService) markUserEvaluated(ctx context.Context, userID, guildID string, todayDate time.Time) error {
	// Get current streak to preserve it
	streak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{
		UserID:  userID,
		GuildID: guildID,
	})
	if err != nil {
		return fmt.Errorf("failed to get current streak: %w", err)
	}

	_, err = s.dbQueries.UpdateUserStreakAfterEvaluation(ctx, database.UpdateUserStreakAfterEvaluationParams{
		UserID:              userID,
		GuildID:             guildID,
		CurrentStreakCount:  streak.CurrentStreakCount,
		MaxStreakCount:      streak.MaxStreakCount,
		StreakEvaluatedDate: sql.NullTime{Time: todayDate, Valid: true},
	})
	return err
}

// SendEveningWarnings sends warnings to users who haven't been active today
func (s *StreakService) SendEveningWarnings(ctx context.Context) {
	todayDate := GetTodayManilaDate()

	users, err := s.dbQueries.GetUsersNeedingWarnings(ctx, sql.NullTime{Time: todayDate, Valid: true})
	if err != nil {
		fmt.Printf("StreakService: Error getting users needing warnings: %v\n", err)
		return
	}

	fmt.Printf("StreakService: Found %d users needing warnings\n", len(users))

	for _, user := range users {
		embed := s.streakWarningEmbed(user.UserID, user.CurrentStreakCount)
		s.sendStreakEmbed(user.GuildID, embed)

		// Mark as warned
		err = s.dbQueries.UpdateWarningNotifiedAt(ctx, database.UpdateWarningNotifiedAtParams{
			UserID:            user.UserID,
			GuildID:           user.GuildID,
			WarningNotifiedAt: sql.NullTime{Time: GetManilaTimeNow(), Valid: true},
		})
		if err != nil {
			fmt.Printf("StreakService: Error updating warning timestamp for user %s: %v\n", user.UserID, err)
		}

		fmt.Printf("StreakService: Sent warning to user %s (streak: %d days)\n", user.UserID, user.CurrentStreakCount)
	}
}

// processImmediateStreakUpdate handles immediate streak increment when user completes daily activity
func (s *StreakService) processImmediateStreakUpdate(ctx context.Context, userID, guildID string, minutes int) error {
	// Get current streak info
	streak, err := s.dbQueries.GetUserStreak(ctx, database.GetUserStreakParams{
		UserID:  userID,
		GuildID: guildID,
	})
	if err != nil {
		return fmt.Errorf("failed to get current streak: %w", err)
	}

	// Simple logic: just increment the streak from current value
	// Don't check yesterday - that's handled by daily evaluation
	var newStreak int32
	if streak.CurrentStreakCount == 0 {
		// Starting first day of streak
		newStreak = 1
	} else {
		// Continuing streak (increment from current)
		newStreak = streak.CurrentStreakCount + 1
	}

	// Update max streak if needed
	newMaxStreak := streak.MaxStreakCount
	if newStreak > newMaxStreak {
		newMaxStreak = newStreak
	}

	// Update streak immediately
	err = s.dbQueries.UpdateStreakImmediately(ctx, database.UpdateStreakImmediatelyParams{
		UserID:             userID,
		GuildID:            guildID,
		CurrentStreakCount: newStreak,
		MaxStreakCount:     newMaxStreak,
	})
	if err != nil {
		return fmt.Errorf("failed to update streak immediately: %w", err)
	}

	// Send completion message with streak count
	embed := s.dailyActivityCompletedWithStreakEmbed(userID, minutes, newStreak)
	s.sendStreakEmbed(guildID, embed)

	fmt.Printf("StreakService: User %s completed daily activity and streak updated to %d days\n", userID, newStreak)
	return nil
}

// GetUserStreakInfoEmbed returns an embed with the user's current streak information
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
	fields := []*discordgo.MessageEmbedField{}

	if err != nil {
		if err == sql.ErrNoRows {
			description = fmt.Sprintf("<@%s> hasn't started a study streak yet. Join a tracked voice channel for %d+ minutes to begin!", userID, minimumActivityMinutes)
		} else {
			return nil, fmt.Errorf("failed to get user streak info: %w", err)
		}
	} else {
		// Add current streak info
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Current Streak",
			Value:  fmt.Sprintf("%d days", streak.CurrentStreakCount),
			Inline: true,
		})

		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Longest Streak",
			Value:  fmt.Sprintf("%d days", streak.MaxStreakCount),
			Inline: true,
		})

		// Add today's activity info
		todayDate := GetTodayManilaDate()
		todayMinutes := 0
		if streak.LastActivityDate.Valid && IsSameManilaDate(streak.LastActivityDate.Time, todayDate) {
			todayMinutes = int(streak.DailyActivityMinutes.Int32)
		}

		activityStatus := fmt.Sprintf("%d/%d minutes", todayMinutes, minimumActivityMinutes)
		if todayMinutes >= minimumActivityMinutes {
			activityStatus += " ✅"
		}

		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Today's Activity",
			Value:  activityStatus,
			Inline: true,
		})

		if streak.CurrentStreakCount > 0 {
			title = fmt.Sprintf("🔥 Streak Status for %s 🔥", username)
			description = fmt.Sprintf("<@%s> is currently on a **%d day** study streak! 🎉", userID, streak.CurrentStreakCount)
			color = 0x00FF00

			if todayMinutes < minimumActivityMinutes {
				description += fmt.Sprintf("\n⚠️ You need **%d more minutes** of voice activity today to maintain your streak!", minimumActivityMinutes-todayMinutes)
				color = 0xFFA500
			}
		} else {
			if streak.MaxStreakCount > 0 {
				description = fmt.Sprintf("<@%s> has no active streak currently. Their longest was **%d days**. Start a new one today!", userID, streak.MaxStreakCount)
				color = 0xFFA500
			} else {
				description = fmt.Sprintf("<@%s> hasn't started a streak yet. Join a tracked voice channel for %d+ minutes to begin!", userID, minimumActivityMinutes)
			}
		}
	}

	return &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Fields:      fields,
		Color:       color,
		Timestamp:   GetManilaTimeNow().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "LockIn Calendar Day Streaks • Manila Time"},
	}, nil
}

// Embed creation methods
func (s *StreakService) newStreakStartedEmbed(userID string, streakCount int32) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "🚀 New Streak Started! 🚀",
		Description: fmt.Sprintf("<@%s> has started a new study streak! Currently **%d day** strong. Keep it up! 🔥", userID, streakCount),
		Color:       0x7CFC00,
		Timestamp:   GetManilaTimeNow().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "Manila Time"},
	}
}

func (s *StreakService) streakContinuedEmbed(userID string, streakCount int32) *discordgo.MessageEmbed {
	milestoneEmoji := "🔥"
	milestoneMsg := ""

	switch streakCount {
	case 7:
		milestoneEmoji = "🌟"
		milestoneMsg = " Amazing! One week strong!"
	case 14:
		milestoneEmoji = "💫"
		milestoneMsg = " Incredible! Two weeks!"
	case 30:
		milestoneEmoji = "🏆"
		milestoneMsg = " Outstanding! One month!"
	case 60:
		milestoneEmoji = "👑"
		milestoneMsg = " Legendary! Two months!"
	case 100:
		milestoneEmoji = "🎖️"
		milestoneMsg = " PHENOMENAL! 100 days!"
	}

	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s Day %d Complete! %s", milestoneEmoji, streakCount, milestoneEmoji),
		Description: fmt.Sprintf("<@%s> is now on a **%d day** study streak!%s Keep the momentum going! 🚀", userID, streakCount, milestoneMsg),
		Color:       0x00AAFF,
		Timestamp:   GetManilaTimeNow().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "Manila Time"},
	}
}

func (s *StreakService) streakWarningEmbed(userID string, streakCount int32) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "⏰ Streak Warning! ⏰",
		Description: fmt.Sprintf("<@%s>, your **%d day** study streak is in danger! ⚠️\n\nYou need to join a tracked voice channel for at least **%d minutes** before the end of today to keep your streak alive!\n\n⏳ Time remaining: Until midnight Manila time", userID, streakCount, minimumActivityMinutes),
		Color:       0xFFA500,
		Timestamp:   GetManilaTimeNow().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "Manila Time"},
	}
}

func (s *StreakService) streakEndedEmbed(userID string, lastStreakCount int32) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "💔 Streak Ended 💔",
		Description: fmt.Sprintf("Oh no! <@%s>'s study streak of **%d days** has come to an end. 😢\n\nDon't give up! Join a tracked voice channel today to start a new streak! 💪", userID, lastStreakCount),
		Color:       0xFF0000,
		Timestamp:   GetManilaTimeNow().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "Manila Time"},
	}
}

// dailyActivityCompletedWithStreakEmbed creates completion message with streak count (new format)
func (s *StreakService) dailyActivityCompletedWithStreakEmbed(userID string, minutes int, streakCount int32) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "🎉 Daily Activity Complete! 🎉",
		Description: fmt.Sprintf("<@%s> has completed **%d minutes** today! Now on a **%d-day** locking in streak! 🔥", userID, minutes, streakCount),
		Color:       0x00FF00,
		Timestamp:   GetManilaTimeNow().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "Manila Time"},
	}
}

// basicDailyActivityCompletedEmbed creates basic completion message (fallback)
func (s *StreakService) basicDailyActivityCompletedEmbed(userID string, minutes int) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "✅ Daily Activity Complete! ✅",
		Description: fmt.Sprintf("<@%s> has completed **%d minutes** of voice activity today! 🎯", userID, minutes),
		Color:       0x00FF00,
		Timestamp:   GetManilaTimeNow().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "Manila Time"},
	}
}

func (s *StreakService) sendStreakEmbed(guildID string, embed *discordgo.MessageEmbed) {
	if s.streakNotificationChannel == "" {
		fmt.Println("StreakService: Streak notification channel ID is not configured")
		return
	}

	_, err := s.discordSession.ChannelMessageSendEmbed(s.streakNotificationChannel, embed)
	if err != nil {
		fmt.Printf("StreakService: Failed to send embed to channel %s: %v\n", s.streakNotificationChannel, err)

		// Fallback: Try to find any text channel in the guild
		channels, _ := s.discordSession.GuildChannels(guildID)
		for _, ch := range channels {
			if ch.Type == discordgo.ChannelTypeGuildText {
				_, errAlt := s.discordSession.ChannelMessageSendEmbed(ch.ID, embed)
				if errAlt == nil {
					fmt.Printf("StreakService: Successfully sent embed to fallback channel %s (%s)\n", ch.Name, ch.ID)
					return
				}
			}
		}
		fmt.Printf("StreakService: Could not find any suitable channel in guild %s\n", guildID)
	}
}
