package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/config"
	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/bwmarrin/discordgo"
)

// AchievementService handles achievement checking and awarding
type AchievementService struct {
	db                   database.Querier
	discordSession       *discordgo.Session
	cfg                  *config.Config
	achievementChannelID string
}

// NewAchievementService creates a new AchievementService
func NewAchievementService(
	queries database.Querier,
	session *discordgo.Session,
	appConfig *config.Config,
) *AchievementService {
	return &AchievementService{
		db:                   queries,
		discordSession:       session,
		cfg:                  appConfig,
		achievementChannelID: appConfig.AchievementChannelID,
	}
}

// CheckStreakAchievements checks and awards streak-based achievements
func (s *AchievementService) CheckStreakAchievements(ctx context.Context, userID, guildID string, currentStreak int32) error {
	streakAchievements := []struct {
		ID       string
		Required int32
	}{
		{"first_flame", 3},
		{"streak_starter", 7},
		{"consistent", 14},
		{"monthly_master", 30},
		{"legendary", 100},
	}

	for _, ach := range streakAchievements {
		if currentStreak >= ach.Required {
			awarded, err := s.tryAwardAchievement(ctx, userID, guildID, ach.ID)
			if err != nil {
				log.Printf("Error checking achievement %s for user %s: %v", ach.ID, userID, err)
				continue
			}
			if awarded {
				log.Printf("AchievementService: Awarded %s to user %s", ach.ID, userID)
			}
		}
	}

	return nil
}

// CheckDurationAchievements checks and awards duration-based achievements
func (s *AchievementService) CheckDurationAchievements(ctx context.Context, userID, guildID string, totalHours float64, sessionHours float64) error {
	// Check total time achievements
	totalAchievements := []struct {
		ID       string
		Required float64
	}{
		{"getting_started", 1},
		{"focused", 10},
		{"bookworm", 50},
		{"century_club", 100},
	}

	for _, ach := range totalAchievements {
		if totalHours >= ach.Required {
			awarded, err := s.tryAwardAchievement(ctx, userID, guildID, ach.ID)
			if err != nil {
				log.Printf("Error checking total hours achievement %s for user %s: %v", ach.ID, userID, err)
				continue
			}
			if awarded {
				log.Printf("AchievementService: Awarded %s to user %s (total hours: %.2f)", ach.ID, userID, totalHours)
			}
		}
	}

	// Check marathon runner (5+ hours in one session)
	if sessionHours >= 5 {
		awarded, err := s.tryAwardAchievement(ctx, userID, guildID, "marathon_runner")
		if err != nil {
			log.Printf("Error checking marathon_runner achievement for user %s: %v", userID, err)
		} else if awarded {
			log.Printf("AchievementService: Awarded marathon_runner to user %s (session hours: %.2f)", userID, sessionHours)
		}
	}

	return nil
}

// CheckTimeBasedAchievements checks and awards time-of-day based achievements
func (s *AchievementService) CheckTimeBasedAchievements(ctx context.Context, userID, guildID string, sessionStart time.Time) error {
	manilaTime := sessionStart.In(GetManilaLocation())
	hour := manilaTime.Hour()
	weekday := manilaTime.Weekday()

	// Early Bird - before 7 AM
	if hour < 7 {
		s.tryAwardAchievement(ctx, userID, guildID, "early_bird")
	}

	// Night Owl - after midnight (12 AM - 4 AM)
	if hour >= 0 && hour < 4 {
		s.tryAwardAchievement(ctx, userID, guildID, "night_owl")
	}

	// Graveyard Shift - 2 AM to 5 AM
	if hour >= 2 && hour < 5 {
		s.tryAwardAchievement(ctx, userID, guildID, "graveyard_shift")
	}

	// Weekend Warrior - studying on Saturday or Sunday
	if weekday == time.Saturday || weekday == time.Sunday {
		s.tryAwardAchievement(ctx, userID, guildID, "weekend_warrior")
	}

	return nil
}

// CheckCompetitionAchievements checks leaderboard-based achievements
func (s *AchievementService) CheckCompetitionAchievements(ctx context.Context, userID, guildID string, leaderboardRank int) error {
	// Rising Star - top 10
	if leaderboardRank <= 10 && leaderboardRank > 0 {
		s.tryAwardAchievement(ctx, userID, guildID, "rising_star")
	}

	// Study King - #1
	if leaderboardRank == 1 {
		s.tryAwardAchievement(ctx, userID, guildID, "study_king")
	}

	return nil
}

// CheckComebackKid checks if user qualifies for comeback achievement
func (s *AchievementService) CheckComebackKid(ctx context.Context, userID, guildID string, previousStreak, newStreak int32) error {
	// If they had a 7+ day streak, lost it (previousStreak is 0 or newStreak is starting fresh),
	// and now have rebuilt to 7+
	if newStreak >= 7 {
		// Check if they already have first_flame (which means they've had streaks before)
		hasFirstFlame, err := s.db.HasAchievement(ctx, database.HasAchievementParams{
			UserID:        userID,
			GuildID:       guildID,
			AchievementID: "streak_starter", // They must have previously had 7+ to "come back"
		})
		if err != nil {
			return err
		}

		// If they've had streak_starter before and now have 7+ again, they came back
		if hasFirstFlame {
			s.tryAwardAchievement(ctx, userID, guildID, "comeback_kid")
		}
	}

	return nil
}

// CheckUndefeated checks if user deserves the undefeated achievement
// Simplified: award if user is #1 on leaderboard AND has streak_starter achievement
func (s *AchievementService) CheckUndefeated(ctx context.Context, userID, guildID string, leaderboardRank int) error {
	// Only check if user is #1
	if leaderboardRank != 1 {
		return nil
	}

	// Check if user has streak_starter (7+ day streak) as a proxy for being active long enough
	hasStreakStarter, err := s.db.HasAchievement(ctx, database.HasAchievementParams{
		UserID:        userID,
		GuildID:       guildID,
		AchievementID: "streak_starter",
	})
	if err != nil {
		return err
	}

	if hasStreakStarter {
		s.tryAwardAchievement(ctx, userID, guildID, "undefeated")
	}
	return nil
}

// CheckGlobalCitizen checks if user studied during 12 unique hours of the day
func (s *AchievementService) CheckGlobalCitizen(ctx context.Context, userID, guildID string) error {
	uniqueHours, err := s.db.GetUniqueStudyHours(ctx, sql.NullString{String: userID, Valid: true})
	if err != nil {
		return fmt.Errorf("failed to get unique study hours: %w", err)
	}

	if uniqueHours >= 12 {
		s.tryAwardAchievement(ctx, userID, guildID, "global_citizen")
	}
	return nil
}

// CheckDawnToDusk checks if user has studied 12+ hours in any single day
func (s *AchievementService) CheckDawnToDusk(ctx context.Context, userID, guildID string) error {
	hasDawnToDusk, err := s.db.HasDawnToDuskDay(ctx, sql.NullString{String: userID, Valid: true})
	if err != nil {
		return fmt.Errorf("failed to check dawn to dusk: %w", err)
	}

	if hasDawnToDusk {
		s.tryAwardAchievement(ctx, userID, guildID, "dawn_to_dusk")
	}
	return nil
}

// tryAwardAchievement attempts to award an achievement, returns true if newly awarded
func (s *AchievementService) tryAwardAchievement(ctx context.Context, userID, guildID, achievementID string) (bool, error) {
	// Check if already has achievement
	hasAchievement, err := s.db.HasAchievement(ctx, database.HasAchievementParams{
		UserID:        userID,
		GuildID:       guildID,
		AchievementID: achievementID,
	})
	if err != nil {
		return false, fmt.Errorf("failed to check achievement: %w", err)
	}

	if hasAchievement {
		return false, nil // Already has it
	}

	// Award the achievement
	_, err = s.db.AwardAchievement(ctx, database.AwardAchievementParams{
		UserID:        userID,
		GuildID:       guildID,
		AchievementID: achievementID,
	})
	if err != nil {
		return false, fmt.Errorf("failed to award achievement: %w", err)
	}

	// Send notification
	go s.sendAchievementNotification(userID, guildID, achievementID)

	return true, nil
}

// sendAchievementNotification sends an achievement unlock notification
func (s *AchievementService) sendAchievementNotification(userID, guildID, achievementID string) {
	if s.achievementChannelID == "" {
		log.Println("AchievementService: Achievement channel not configured, skipping notification")
		return
	}

	ctx := context.Background()

	// Get achievement details
	achievement, err := s.db.GetAchievementByID(ctx, achievementID)
	if err != nil {
		log.Printf("AchievementService: Failed to get achievement details for %s: %v", achievementID, err)
		return
	}

	// Get user's total achievement count
	count, err := s.db.GetUserAchievementCount(ctx, database.GetUserAchievementCountParams{
		UserID:  userID,
		GuildID: guildID,
	})
	if err != nil {
		log.Printf("AchievementService: Failed to get user achievement count: %v", err)
		count = 0
	}

	// Get total achievements count
	totalCount, err := s.db.GetTotalAchievementCount(ctx)
	if err != nil {
		log.Printf("AchievementService: Failed to get total achievement count: %v", err)
		totalCount = 20
	}

	// Create the embed
	embed := &discordgo.MessageEmbed{
		Title:       "🎉 Achievement Unlocked! 🎉",
		Description: fmt.Sprintf("<@%s> just earned a new badge!", userID),
		Color:       0xFFD700, // Gold
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Badge",
				Value:  fmt.Sprintf("%s **%s**", achievement.Icon, achievement.Name),
				Inline: true,
			},
			{
				Name:   "Description",
				Value:  achievement.Description,
				Inline: true,
			},
			{
				Name:   "Progress",
				Value:  fmt.Sprintf("%d/%d badges", count, totalCount),
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Use /profile to see all your badges!",
		},
	}

	_, err = s.discordSession.ChannelMessageSendEmbed(s.achievementChannelID, embed)
	if err != nil {
		log.Printf("AchievementService: Failed to send achievement notification: %v", err)
	}

	// Mark as notified
	err = s.db.MarkAchievementNotified(ctx, database.MarkAchievementNotifiedParams{
		UserID:        userID,
		GuildID:       guildID,
		AchievementID: achievementID,
	})
	if err != nil {
		log.Printf("AchievementService: Failed to mark achievement as notified: %v", err)
	}
}

// GetUserProfile returns profile data including achievements for embed
func (s *AchievementService) GetUserProfile(ctx context.Context, userID, guildID string) (*ProfileData, error) {
	// Get user's achievements
	achievements, err := s.db.GetUserAchievements(ctx, database.GetUserAchievementsParams{
		UserID:  userID,
		GuildID: guildID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get user achievements: %w", err)
	}

	// Get total achievement count
	totalCount, err := s.db.GetTotalAchievementCount(ctx)
	if err != nil {
		totalCount = 20
	}

	// Get featured badge
	featuredBadge, err := s.db.GetUserFeaturedBadge(ctx, userID)
	if err != nil {
		// Not critical, just no featured badge
		featuredBadge = database.GetUserFeaturedBadgeRow{}
	}

	// Build badge string (just the icons)
	var badgeIcons string
	for _, ach := range achievements {
		badgeIcons += ach.Icon
	}

	// If no badges, show placeholder
	if badgeIcons == "" {
		badgeIcons = "No badges yet! Start studying to earn badges."
	}

	return &ProfileData{
		UserID:        userID,
		GuildID:       guildID,
		BadgeCount:    int(len(achievements)),
		TotalBadges:   int(totalCount),
		BadgeIcons:    badgeIcons,
		Achievements:  achievements,
		FeaturedBadge: featuredBadge,
	}, nil
}

// ProfileData contains user profile information
type ProfileData struct {
	UserID        string
	GuildID       string
	BadgeCount    int
	TotalBadges   int
	BadgeIcons    string
	Achievements  []database.GetUserAchievementsRow
	FeaturedBadge database.GetUserFeaturedBadgeRow
}

// GetAllAchievementsWithProgress returns all achievements with user progress
func (s *AchievementService) GetAllAchievementsWithProgress(ctx context.Context, userID, guildID string) ([]AchievementWithProgress, error) {
	// Get all achievements
	allAchievements, err := s.db.GetAllAchievements(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all achievements: %w", err)
	}

	// Get user's earned achievements
	userAchievements, err := s.db.GetUserAchievements(ctx, database.GetUserAchievementsParams{
		UserID:  userID,
		GuildID: guildID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get user achievements: %w", err)
	}

	// Create a map of earned achievements
	earnedMap := make(map[string]bool)
	for _, ua := range userAchievements {
		earnedMap[ua.AchievementID] = true
	}

	// Build result
	result := make([]AchievementWithProgress, 0, len(allAchievements))
	for _, ach := range allAchievements {
		// Skip secret achievements that aren't earned
		if ach.IsSecret.Bool && !earnedMap[ach.AchievementID] {
			result = append(result, AchievementWithProgress{
				AchievementID: ach.AchievementID,
				Name:          "???",
				Description:   "This is a secret achievement!",
				Icon:          "❓",
				Category:      ach.Category,
				Earned:        false,
				IsSecret:      true,
			})
			continue
		}

		result = append(result, AchievementWithProgress{
			AchievementID: ach.AchievementID,
			Name:          ach.Name,
			Description:   ach.Description,
			Icon:          ach.Icon,
			Category:      ach.Category,
			Earned:        earnedMap[ach.AchievementID],
			IsSecret:      ach.IsSecret.Bool,
		})
	}

	return result, nil
}

// AchievementWithProgress represents an achievement with earned status
type AchievementWithProgress struct {
	AchievementID string
	Name          string
	Description   string
	Icon          string
	Category      string
	Earned        bool
	IsSecret      bool
}
