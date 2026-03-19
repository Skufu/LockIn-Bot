package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/config"
	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockQuerier implements the database.Querier interface for testing for testing
type MockQuerier struct {
	mock.Mock
}

// Implement all methods for the database.Querier interface
func (m *MockQuerier) GetUser(ctx context.Context, userID string) (database.User, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(database.User), args.Error(1)
}

func (m *MockQuerier) CreateUser(ctx context.Context, params database.CreateUserParams) (database.User, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(database.User), args.Error(1)
}

func (m *MockQuerier) GetUserStats(ctx context.Context, userID string) (database.UserStat, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(database.UserStat), args.Error(1)
}

func (m *MockQuerier) GetLeaderboard(ctx context.Context) ([]database.GetLeaderboardRow, error) {
	args := m.Called(ctx)
	return args.Get(0).([]database.GetLeaderboardRow), args.Error(1)
}

func (m *MockQuerier) CreateStudySession(ctx context.Context, params database.CreateStudySessionParams) (database.StudySession, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(database.StudySession), args.Error(1)
}

func (m *MockQuerier) GetActiveStudySession(ctx context.Context, userID sql.NullString) (database.StudySession, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(database.StudySession), args.Error(1)
}

func (m *MockQuerier) EndStudySession(ctx context.Context, params database.EndStudySessionParams) (database.StudySession, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(database.StudySession), args.Error(1)
}

func (m *MockQuerier) CreateOrUpdateUserStats(ctx context.Context, params database.CreateOrUpdateUserStatsParams) (database.UserStat, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(database.UserStat), args.Error(1)
}

func (m *MockQuerier) DeleteOldStudySession(ctx context.Context, startTime time.Time) error {
	args := m.Called(ctx, startTime)
	return args.Error(0)
}

func (m *MockQuerier) GetUserStreak(ctx context.Context, arg database.GetUserStreakParams) (database.GetUserStreakRow, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(database.GetUserStreakRow), args.Error(1)
}

func (m *MockQuerier) GetUsersForDailyEvaluation(ctx context.Context, streakEvaluatedDate sql.NullTime) ([]database.GetUsersForDailyEvaluationRow, error) {
	args := m.Called(ctx, streakEvaluatedDate)
	return args.Get(0).([]database.GetUsersForDailyEvaluationRow), args.Error(1)
}

func (m *MockQuerier) GetUsersForStreakReset(ctx context.Context, lastActivityDate sql.NullTime) ([]database.GetUsersForStreakResetRow, error) {
	args := m.Called(ctx, lastActivityDate)
	return args.Get(0).([]database.GetUsersForStreakResetRow), args.Error(1)
}

func (m *MockQuerier) GetUsersNeedingWarnings(ctx context.Context, lastActivityDate sql.NullTime) ([]database.GetUsersNeedingWarningsRow, error) {
	args := m.Called(ctx, lastActivityDate)
	return args.Get(0).([]database.GetUsersNeedingWarningsRow), args.Error(1)
}

func (m *MockQuerier) HasActivityForDate(ctx context.Context, arg database.HasActivityForDateParams) (bool, error) {
	args := m.Called(ctx, arg)
	return args.Bool(0), args.Error(1)
}

func (m *MockQuerier) ResetAllStreakDailyFlags(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockQuerier) ResetDailyStudyTime(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockQuerier) ResetMonthlyStudyTime(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockQuerier) ResetUserStreakCount(ctx context.Context, arg database.ResetUserStreakCountParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *MockQuerier) ResetWeeklyStudyTime(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockQuerier) StartDailyActivity(ctx context.Context, arg database.StartDailyActivityParams) (database.StartDailyActivityRow, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(database.StartDailyActivityRow), args.Error(1)
}

func (m *MockQuerier) UpdateDailyActivityMinutes(ctx context.Context, arg database.UpdateDailyActivityMinutesParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *MockQuerier) UpdateStreakImmediately(ctx context.Context, arg database.UpdateStreakImmediatelyParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *MockQuerier) UpdateUserStreakAfterEvaluation(ctx context.Context, arg database.UpdateUserStreakAfterEvaluationParams) (database.UpdateUserStreakAfterEvaluationRow, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(database.UpdateUserStreakAfterEvaluationRow), args.Error(1)
}

func (m *MockQuerier) UpdateWarningNotifiedAt(ctx context.Context, arg database.UpdateWarningNotifiedAtParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *MockQuerier) HasAchievement(ctx context.Context, arg database.HasAchievementParams) (bool, error) {
	args := m.Called(ctx, arg)
	return args.Bool(0), args.Error(1)
}

func (m *MockQuerier) AwardAchievement(ctx context.Context, arg database.AwardAchievementParams) (database.UserAchievement, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(database.UserAchievement), args.Error(1)
}

func (m *MockQuerier) GetUniqueStudyHours(ctx context.Context, userID sql.NullString) (int32, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int32), args.Error(1)
}

func (m *MockQuerier) HasDawnToDuskDay(ctx context.Context, userID sql.NullString) (bool, error) {
	args := m.Called(ctx, userID)
	return args.Bool(0), args.Error(1)
}

func (m *MockQuerier) GetAchievementByID(ctx context.Context, achievementID string) (database.GetAchievementByIDRow, error) {
	args := m.Called(ctx, achievementID)
	return args.Get(0).(database.GetAchievementByIDRow), args.Error(1)
}

func (m *MockQuerier) GetUserAchievementCount(ctx context.Context, arg database.GetUserAchievementCountParams) (int64, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockQuerier) GetTotalAchievementCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockQuerier) GetUserAchievements(ctx context.Context, arg database.GetUserAchievementsParams) ([]database.GetUserAchievementsRow, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).([]database.GetUserAchievementsRow), args.Error(1)
}

func (m *MockQuerier) GetUserFeaturedBadge(ctx context.Context, userID string) (database.GetUserFeaturedBadgeRow, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(database.GetUserFeaturedBadgeRow), args.Error(1)
}

func (m *MockQuerier) GetAllAchievements(ctx context.Context) ([]database.GetAllAchievementsRow, error) {
	args := m.Called(ctx)
	return args.Get(0).([]database.GetAllAchievementsRow), args.Error(1)
}

func (m *MockQuerier) MarkAchievementNotified(ctx context.Context, arg database.MarkAchievementNotifiedParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *MockQuerier) CountStudySessions(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockQuerier) DeleteAllStudySessions(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockQuerier) DeleteOldStudySessions(ctx context.Context, startTime time.Time) error {
	args := m.Called(ctx, startTime)
	return args.Error(0)
}

func (m *MockQuerier) DeleteOldStudySessionsWithCount(ctx context.Context, startTime time.Time) (int64, error) {
	args := m.Called(ctx, startTime)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockQuerier) GetAchievementsByCategory(ctx context.Context, category string) ([]database.GetAchievementsByCategoryRow, error) {
	args := m.Called(ctx, category)
	return args.Get(0).([]database.GetAchievementsByCategoryRow), args.Error(1)
}

func (m *MockQuerier) GetAchievementsByRequirementType(ctx context.Context, requirementType string) ([]database.GetAchievementsByRequirementTypeRow, error) {
	args := m.Called(ctx, requirementType)
	return args.Get(0).([]database.GetAchievementsByRequirementTypeRow), args.Error(1)
}

func (m *MockQuerier) GetUnnotifiedAchievements(ctx context.Context, arg database.GetUnnotifiedAchievementsParams) ([]database.GetUnnotifiedAchievementsRow, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).([]database.GetUnnotifiedAchievementsRow), args.Error(1)
}

func (m *MockQuerier) SetFeaturedBadge(ctx context.Context, arg database.SetFeaturedBadgeParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

// Mock for Discord session to avoid actual calls in tests
type MockDiscordSession struct {
	mock.Mock
}

func (m *MockDiscordSession) ChannelMessageSendEmbed(channelID string, embed *discordgo.MessageEmbed) (*discordgo.Message, error) {
	args := m.Called(channelID, embed)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*discordgo.Message), args.Error(1)
}

func (m *MockDiscordSession) UpdateStatusComplex(status discordgo.UpdateStatusData) error {
	return nil
}

func (m *MockDiscordSession) Close() error {
	return nil
}

// Helper function to create test achievement service (uses direct field assignment to bypass constructor type requirements)
func createTestAchievementService(mockDB *MockQuerier) (*AchievementService, *MockDiscordSession) {
	cfg := &config.Config{
		AchievementChannelID: "", // Empty to skip notification goroutine
	}

	mockSession := new(MockDiscordSession)

	service := &AchievementService{
		db:                   mockDB,
		discordSession:       &discordgo.Session{},
		cfg:                  cfg,
		achievementChannelID: cfg.AchievementChannelID,
	}

	return service, mockSession
}

// Helper function to setup mocks for achievement notification
func setupAchievementNotificationMocks(mockDB *MockQuerier, achievementID string) {
	mockDB.On("GetAchievementByID", mock.Anything, achievementID).Return(database.GetAchievementByIDRow{
		AchievementID: achievementID,
		Name:          "Test Achievement",
		Description:   "Test Description",
		Icon:          "🏆",
	}, nil).Maybe()
}

// Helper function to setup mocks for all notification-related calls
func setupFullNotificationMocks(mockDB *MockQuerier, userID, guildID, achievementID string) {
	mockDB.On("GetAchievementByID", mock.Anything, achievementID).Return(database.GetAchievementByIDRow{
		AchievementID: achievementID,
		Name:          "Test Achievement",
		Description:   "Test Description",
		Icon:          "🏆",
	}, nil).Maybe()

	mockDB.On("GetUserAchievementCount", mock.Anything, mock.MatchedBy(func(params database.GetUserAchievementCountParams) bool {
		return params.UserID == userID && params.GuildID == guildID
	})).Return(int64(1), nil).Maybe()

	mockDB.On("GetTotalAchievementCount", mock.Anything).Return(int64(20), nil).Maybe()

	mockDB.On("MarkAchievementNotified", mock.Anything, mock.MatchedBy(func(params database.MarkAchievementNotifiedParams) bool {
		return params.UserID == userID && params.GuildID == guildID && params.AchievementID == achievementID
	})).Return(nil).Maybe()
}

// Test basic functionality of CheckStreakAchievements
func TestCheckStreakAchievements_AllLevels(t *testing.T) {
	mockDB := new(MockQuerier)
	service, _ := createTestAchievementService(mockDB)

	userID := "test-user"
	guildID := "test-guild"

	testCases := []struct {
		name          string
		currentStreak int32
	}{
		{"Below minimum streak", 1},
		{"3 streak - first flame", 3},
		{"5 streak - below starter", 5},
		{"7 streak - streak starter", 7},
		{"10 streak - between starter and consistent", 10},
		{"14 streak - consistent", 14},
		{"20 streak - between consistent and monthly master", 20},
		{"30 streak - monthly master", 30},
		{"50 streak - between monthly master and legendary", 50},
		{"100 streak - legendary", 100},
		{"200 streak - all achieved", 200},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Expected achievements based on the current streak
			expectedAchievements := []string{}
			if tc.currentStreak >= 3 {
				expectedAchievements = append(expectedAchievements, "first_flame")
			}
			if tc.currentStreak >= 7 {
				expectedAchievements = append(expectedAchievements, "streak_starter")
			}
			if tc.currentStreak >= 14 {
				expectedAchievements = append(expectedAchievements, "consistent")
			}
			if tc.currentStreak >= 30 {
				expectedAchievements = append(expectedAchievements, "monthly_master")
			}
			if tc.currentStreak >= 100 {
				expectedAchievements = append(expectedAchievements, "legendary")
			}

			// Set up expectations for each potential achievement check
			for _, achievementID := range expectedAchievements {
				mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == achievementID
				})).Return(false, nil).Once()

				mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == achievementID
				})).Return(database.UserAchievement{}, nil).Once()

				setupFullNotificationMocks(mockDB, userID, guildID, achievementID)
			}

			err := service.CheckStreakAchievements(context.Background(), userID, guildID, tc.currentStreak)
			assert.NoError(t, err)

			mockDB.AssertExpectations(t)
		})
	}
}

// Test idempotency for CheckStreakAchievements
func TestCheckStreakAchievements_Idempotency(t *testing.T) {
	mockDB := new(MockQuerier)
	service, _ := createTestAchievementService(mockDB)

	userID := "test-user"
	guildID := "test-guild"
	currentStreak := int32(50)

	// Test that user who already has achievements doesn't receive them again
	mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "first_flame"
	})).Return(true, nil).Once() // Already has first_flame

	// Check for streak_starter
	mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "streak_starter"
	})).Return(true, nil).Once() // Already has streak_starter

	// Check for consistent
	mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "consistent"
	})).Return(true, nil).Once() // Already has consistent

	// Check for monthly_master - this could be new
	mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "monthly_master"
	})).Return(false, nil).Once() // Doesn't have it yet

	mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "monthly_master"
	})).Return(database.UserAchievement{}, nil).Once()

	setupFullNotificationMocks(mockDB, userID, guildID, "monthly_master") // Award it

	err := service.CheckStreakAchievements(context.Background(), userID, guildID, currentStreak)
	assert.NoError(t, err)

	mockDB.AssertExpectations(t)
}

// Test duration achievements - total time based
func TestCheckDurationAchievements_TotalAndSession(t *testing.T) {
	mockDB := new(MockQuerier)
	service, _ := createTestAchievementService(mockDB)

	userID := "test-user"
	guildID := "test-guild"

	testCases := []struct {
		name         string
		totalHours   float64
		sessionHours float64
	}{
		{"No achievements unlocked", 0.5, 1.0},
		{"Getting Started unlocked", 1.5, 1.0},
		{"Getting Started and Focused unlocked", 11.0, 1.0},
		{"All total achievements unlocked", 101.0, 1.0},
		{"Marathon runner unlocked", 10.0, 6.0},
		{"All achievements unlocked", 101.0, 6.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expectedTotalAchievements := []string{}
			if tc.totalHours >= 1 {
				expectedTotalAchievements = append(expectedTotalAchievements, "getting_started")
			}
			if tc.totalHours >= 10 {
				expectedTotalAchievements = append(expectedTotalAchievements, "focused")
			}
			if tc.totalHours >= 50 {
				expectedTotalAchievements = append(expectedTotalAchievements, "bookworm")
			}
			if tc.totalHours >= 100 {
				expectedTotalAchievements = append(expectedTotalAchievements, "century_club")
			}

			awardMarathonRunner := tc.sessionHours >= 5

			// Expected awards
			for _, achievementID := range expectedTotalAchievements {
				mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == achievementID
				})).Return(false, nil).Once()

				mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == achievementID
				})).Return(database.UserAchievement{}, nil).Once()

				setupFullNotificationMocks(mockDB, userID, guildID, achievementID)
			}

			if awardMarathonRunner {
				mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == "marathon_runner"
				})).Return(false, nil).Once()

				mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == "marathon_runner"
				})).Return(database.UserAchievement{}, nil).Once()

				setupFullNotificationMocks(mockDB, userID, guildID, "marathon_runner")
			}

			err := service.CheckDurationAchievements(context.Background(), userID, guildID, tc.totalHours, tc.sessionHours)
			assert.NoError(t, err)

			mockDB.AssertExpectations(t)
		})
	}
}

// Test time-based achievements
func TestCheckTimeBasedAchievements_AllTimeRanges(t *testing.T) {
	mockDB := new(MockQuerier)
	service, _ := createTestAchievementService(mockDB)

	userID := "test-user"
	guildID := "test-guild"

	// Test Early Bird (before 7 AM)
	t.Run("Early Bird - 6 AM Manila", func(t *testing.T) {
		sessionTime := time.Date(2024, time.March, 1, 6, 30, 0, 0, GetManilaLocation())

		mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
			return params.UserID == userID &&
				params.GuildID == guildID &&
				params.AchievementID == "early_bird"
		})).Return(false, nil).Once()

		mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
			return params.UserID == userID &&
				params.GuildID == guildID &&
				params.AchievementID == "early_bird"
		})).Return(database.UserAchievement{}, nil).Once()

		setupFullNotificationMocks(mockDB, userID, guildID, "early_bird")

		err := service.CheckTimeBasedAchievements(context.Background(), userID, guildID, sessionTime)
		assert.NoError(t, err)
		mockDB.AssertExpectations(t)
	})

	// Test Night Owl and Graveyard Shift (0-4 AM)
	t.Run("Night Owl - 2 AM Manila", func(t *testing.T) {
		sessionTime := time.Date(2024, time.March, 1, 2, 15, 0, 0, GetManilaLocation())

		// early_bird would be tried first
		mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
			return params.UserID == userID &&
				params.GuildID == guildID &&
				params.AchievementID == "early_bird"
		})).Return(false, nil).Once()

		mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
			return params.UserID == userID &&
				params.GuildID == guildID &&
				params.AchievementID == "early_bird"
		})).Return(database.UserAchievement{}, nil).Once()

		setupFullNotificationMocks(mockDB, userID, guildID, "early_bird")

		// night_owl would be tried - it's 0-4AM, this is 2AM
		mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
			return params.UserID == userID &&
				params.GuildID == guildID &&
				params.AchievementID == "night_owl"
		})).Return(false, nil).Once()

		mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
			return params.UserID == userID &&
				params.GuildID == guildID &&
				params.AchievementID == "night_owl"
		})).Return(database.UserAchievement{}, nil).Once()

		setupFullNotificationMocks(mockDB, userID, guildID, "night_owl")

		// graveyard_shift - it's 2-5AM, this is 2AM
		mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
			return params.UserID == userID &&
				params.GuildID == guildID &&
				params.AchievementID == "graveyard_shift"
		})).Return(false, nil).Once()

		mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
			return params.UserID == userID &&
				params.GuildID == guildID &&
				params.AchievementID == "graveyard_shift"
		})).Return(database.UserAchievement{}, nil).Once()

		setupFullNotificationMocks(mockDB, userID, guildID, "graveyard_shift")

		err := service.CheckTimeBasedAchievements(context.Background(), userID, guildID, sessionTime)
		assert.NoError(t, err)
		mockDB.AssertExpectations(t)
	})

	// Test Weekend Warrior (weekend)
	t.Run("Weekend Warrior - Saturday", func(t *testing.T) {
		sessionTime := time.Date(2024, time.March, 2, 15, 0, 0, 0, GetManilaLocation()) // March 2, 2024 is a Saturday

		mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
			return params.UserID == userID &&
				params.GuildID == guildID &&
				params.AchievementID == "weekend_warrior"
		})).Return(false, nil).Once()

		mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
			return params.UserID == userID &&
				params.GuildID == guildID &&
				params.AchievementID == "weekend_warrior"
		})).Return(database.UserAchievement{}, nil).Once()

		setupFullNotificationMocks(mockDB, userID, guildID, "weekend_warrior")

		err := service.CheckTimeBasedAchievements(context.Background(), userID, guildID, sessionTime)
		assert.NoError(t, err)
		mockDB.AssertExpectations(t)
	})
}

// Test competition achievements
func TestCheckCompetitionAchievements_RankThresholds(t *testing.T) {
	mockDB := new(MockQuerier)
	service, _ := createTestAchievementService(mockDB)

	userID := "test-user"
	guildID := "test-guild"

	testCases := []struct {
		name   string
		rank   int
		awards []string
	}{
		{"Not in top 10", 15, []string{}},
		{"Exactly rank 10", 10, []string{"rising_star"}},
		{"Rank 7", 7, []string{"rising_star"}},
		{"Rank 1", 1, []string{"rising_star", "study_king"}},
		{"Rank 3", 3, []string{"rising_star"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, achievementID := range tc.awards {
				mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == achievementID
				})).Return(false, nil).Once()

				mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == achievementID
				})).Return(database.UserAchievement{}, nil).Once()

				setupFullNotificationMocks(mockDB, userID, guildID, achievementID)
			}

			err := service.CheckCompetitionAchievements(context.Background(), userID, guildID, tc.rank)
			assert.NoError(t, err)

			mockDB.AssertExpectations(t)
		})
	}
}

// Test global citizen achievement
func TestCheckGlobalCitizen_UniqueHours(t *testing.T) {
	mockDB := new(MockQuerier)
	service, _ := createTestAchievementService(mockDB)

	userID := "test-user"
	guildID := "test-guild"

	testCases := []struct {
		name        string
		uniqueHours int32
		shouldAward bool
	}{
		{"Below requirement", 8, false},
		{"Exact requirement", 12, true},
		{"Above requirement", 15, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB.On("GetUniqueStudyHours", mock.Anything, mock.MatchedBy(func(searchUserID sql.NullString) bool {
				return searchUserID.String == userID && searchUserID.Valid
			})).Return(tc.uniqueHours, nil).Once()

			if tc.shouldAward {
				mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == "global_citizen"
				})).Return(false, nil).Once()

				mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == "global_citizen"
				})).Return(database.UserAchievement{}, nil).Once()

				setupFullNotificationMocks(mockDB, userID, guildID, "global_citizen")
			}

			err := service.CheckGlobalCitizen(context.Background(), userID, guildID)
			assert.NoError(t, err)

			mockDB.AssertExpectations(t)
		})
	}
}

// Test dawn-to-dusk achievement
func TestCheckDawnToDusk_DailyHours(t *testing.T) {
	mockDB := new(MockQuerier)
	service, _ := createTestAchievementService(mockDB)

	userID := "test-user"
	guildID := "test-guild"

	testCases := []struct {
		name          string
		hasDawnToDusk bool
		shouldAward   bool
	}{
		{"No dawn-to-dusk day", false, false},
		{"Has dawn-to-dusk day", true, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB.On("HasDawnToDuskDay", mock.Anything, mock.MatchedBy(func(searchUserID sql.NullString) bool {
				return searchUserID.String == userID && searchUserID.Valid
			})).Return(tc.hasDawnToDusk, nil).Once()

			if tc.shouldAward {
				mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == "dawn_to_dusk"
				})).Return(false, nil).Once()

				mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == "dawn_to_dusk"
				})).Return(database.UserAchievement{}, nil).Once()

				setupFullNotificationMocks(mockDB, userID, guildID, "dawn_to_dusk")
			}

			err := service.CheckDawnToDusk(context.Background(), userID, guildID)
			assert.NoError(t, err)

			mockDB.AssertExpectations(t)
		})
	}
}

// Test Undefeated achievement checks
func TestCheckUndefeated_RankOne(t *testing.T) {
	mockDB := new(MockQuerier)
	service, _ := createTestAchievementService(mockDB)

	userID := "test-user"
	guildID := "test-guild"

	testCases := []struct {
		name             string
		rank             int
		hasStreakStarter bool
		shouldAward      bool
	}{
		{"Rank 1 with streak_starter", 1, true, true},
		{"Rank 1 without streak_starter", 1, false, false},
		{"Not rank 1 with streak_starter", 5, true, false},
		{"Not rank 1 without streak_starter", 10, false, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.rank == 1 {
				mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == "streak_starter"
				})).Return(tc.hasStreakStarter, nil).Once()

				if tc.shouldAward {
					mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
						return params.UserID == userID &&
							params.GuildID == guildID &&
							params.AchievementID == "undefeated"
					})).Return(false, nil).Once()

					mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
						return params.UserID == userID &&
							params.GuildID == guildID &&
							params.AchievementID == "undefeated"
					})).Return(database.UserAchievement{}, nil).Once()

					setupFullNotificationMocks(mockDB, userID, guildID, "undefeated")
				}
			}

			err := service.CheckUndefeated(context.Background(), userID, guildID, tc.rank)
			assert.NoError(t, err)

			mockDB.AssertExpectations(t)
		})
	}
}

// Test Comeback Kid achievement
func TestCheckComebackKid_StreakRecovery(t *testing.T) {
	mockDB := new(MockQuerier)
	service, _ := createTestAchievementService(mockDB)

	userID := "test-user"
	guildID := "test-guild"

	testCases := []struct {
		name             string
		previousStreak   int32
		newStreak        int32
		hasStreakStarter bool // indicates they previously had 7+ streak
		shouldAward      bool
	}{
		{"New streak 7+, no prior streak_starter", 0, 7, false, false},
		{"New streak 7+, had prior streak_starter", 0, 9, true, true},
		{"New streak below 7, had prior streak_starter", 0, 5, true, false},
		{"New streak 7+, no prior streak_starter", 5, 8, false, false},
		{"New streak 7+, with prior streak_starter", 0, 10, true, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.newStreak >= 7 {
				// Check for streak_starter to determine if user had 7+ before
				mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
					return params.UserID == userID &&
						params.GuildID == guildID &&
						params.AchievementID == "streak_starter"
				})).Return(tc.hasStreakStarter, nil).Once()

				if tc.shouldAward {
					mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
						return params.UserID == userID &&
							params.GuildID == guildID &&
							params.AchievementID == "comeback_kid"
					})).Return(false, nil).Once()

					mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
						return params.UserID == userID &&
							params.GuildID == guildID &&
							params.AchievementID == "comeback_kid"
					})).Return(database.UserAchievement{}, nil).Once()

					setupFullNotificationMocks(mockDB, userID, guildID, "comeback_kid")
				}
			}

			err := service.CheckComebackKid(context.Background(), userID, guildID, tc.previousStreak, tc.newStreak)
			assert.NoError(t, err)

			mockDB.AssertExpectations(t)
		})
	}
}

// Test idempotency of tryAwardAchievement (already earned)
func TestTryAwardAchievement_AlreadyEarned(t *testing.T) {
	mockDB := new(MockQuerier)
	service, _ := createTestAchievementService(mockDB)

	userID := "test-user"
	guildID := "test-guild"

	sessionTime := time.Date(2024, time.March, 1, 6, 30, 0, 0, GetManilaLocation())

	mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "early_bird"
	})).Return(true, nil).Once()
	// Since user already has the achievement, AwardAchievement should NOT be called

	err := service.CheckTimeBasedAchievements(context.Background(), userID, guildID, sessionTime)
	assert.NoError(t, err)

	mockDB.AssertExpectations(t)
}

// Test successful achievement awarding (new award)
func TestTryAwardAchievement_NewAward(t *testing.T) {
	mockDB := new(MockQuerier)
	service, mockSession := createTestAchievementService(mockDB)

	userID := "test-user"
	guildID := "test-guild"

	// Set up expectations for getting achievement details for notification
	achievementDetails := database.GetAchievementByIDRow{
		Name:        "First Flame",
		Description: "Complete your first day of studying",
		Icon:        "🔥",
	}

	// Expectations for achievement checks and awarding
	mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "first_flame"
	})).Return(false, nil).Once()

	mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "first_flame"
	})).Return(database.UserAchievement{}, nil).Once()

	// Additional expectations during execution (for notification)
	mockDB.On("GetAchievementByID", mock.Anything, "first_flame").Return(achievementDetails, nil).Maybe()
	mockDB.On("GetUserAchievementCount", mock.Anything, mock.MatchedBy(func(params database.GetUserAchievementCountParams) bool {
		return params.UserID == userID && params.GuildID == guildID
	})).Return(int64(1), nil).Maybe()
	mockDB.On("GetTotalAchievementCount", mock.Anything).Return(int64(20), nil).Maybe()
	mockDB.On("MarkAchievementNotified", mock.Anything, mock.MatchedBy(func(params database.MarkAchievementNotifiedParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "first_flame"
	})).Return(nil).Maybe()

	mockSession.On("ChannelMessageSendEmbed", "test-achievements-channel", mock.AnythingOfType("*discordgo.MessageEmbed")).Return(&discordgo.Message{}, nil).Maybe()

	// Call with a streak that includes first_flame (streak >= 3) but user hasn't earned it yet
	err := service.CheckStreakAchievements(context.Background(), userID, guildID, 5)
	assert.NoError(t, err)

	// Core assertion: verify the award mechanism works correctly
	mockDB.AssertExpectations(t)
}

// Integration test - combine multiple achievement checks
func TestAchievementService_ComprehensiveIntegration(t *testing.T) {
	mockDB := new(MockQuerier)
	service, _ := createTestAchievementService(mockDB)

	userID := "test-user"
	guildID := "test-guild"
	currentStreak := int32(15) // Should unlock first_flame, streak_starter, consistent
	totalHours := 25.5         // Should unlock getting_started, focused
	sessionHours := 1.5        // Won't unlock marathon_runner
	rank := 5                  // Should unlock rising_star

	// Streak Achievements (Should check and award all up to current streak: 3, 7, 14 -> first_flame, streak_starter, consistent)
	// first_flame
	mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "first_flame"
	})).Return(false, nil).Once()
	mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "first_flame"
	})).Return(database.UserAchievement{}, nil).Once()

	setupFullNotificationMocks(mockDB, userID, guildID, "first_flame")

	// streak_starter
	mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "streak_starter"
	})).Return(false, nil).Once()
	mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "streak_starter"
	})).Return(database.UserAchievement{}, nil).Once()

	setupFullNotificationMocks(mockDB, userID, guildID, "streak_starter")

	// consistent
	mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "consistent"
	})).Return(false, nil).Once()
	mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "consistent"
	})).Return(database.UserAchievement{}, nil).Once()

	setupFullNotificationMocks(mockDB, userID, guildID, "consistent")

	// Duration Achievements (getting_started, focused)
	// getting_started
	mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "getting_started"
	})).Return(false, nil).Once()
	mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "getting_started"
	})).Return(database.UserAchievement{}, nil).Once()

	setupFullNotificationMocks(mockDB, userID, guildID, "getting_started")

	// focused
	mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "focused"
	})).Return(false, nil).Once()
	mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "focused"
	})).Return(database.UserAchievement{}, nil).Once()

	setupFullNotificationMocks(mockDB, userID, guildID, "focused")

	// Duration Achievements (marathon_runner: should NOT unlock because session was only 1.5 hours)

	// Competition Achievements (rising_star: rank 5 is < 10 so should award)
	mockDB.On("HasAchievement", mock.Anything, mock.MatchedBy(func(params database.HasAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "rising_star"
	})).Return(false, nil).Once()
	mockDB.On("AwardAchievement", mock.Anything, mock.MatchedBy(func(params database.AwardAchievementParams) bool {
		return params.UserID == userID &&
			params.GuildID == guildID &&
			params.AchievementID == "rising_star"
	})).Return(database.UserAchievement{}, nil).Once()

	setupFullNotificationMocks(mockDB, userID, guildID, "rising_star")

	// No call for study_king because rank is not 1

	err := service.CheckStreakAchievements(context.Background(), userID, guildID, currentStreak)
	assert.NoError(t, err)

	err = service.CheckDurationAchievements(context.Background(), userID, guildID, totalHours, sessionHours)
	assert.NoError(t, err)

	err = service.CheckCompetitionAchievements(context.Background(), userID, guildID, rank)
	assert.NoError(t, err)

	mockDB.AssertExpectations(t)
}
