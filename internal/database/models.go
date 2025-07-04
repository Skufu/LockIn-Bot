// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.29.0

package database

import (
	"database/sql"
	"time"
)

type StudySession struct {
	SessionID  int32          `json:"sessionId"`
	UserID     sql.NullString `json:"userId"`
	StartTime  time.Time      `json:"startTime"`
	EndTime    sql.NullTime   `json:"endTime"`
	DurationMs sql.NullInt64  `json:"durationMs"`
}

type User struct {
	UserID   string         `json:"userId"`
	Username sql.NullString `json:"username"`
}

type UserStat struct {
	UserID         string        `json:"userId"`
	TotalStudyMs   sql.NullInt64 `json:"totalStudyMs"`
	DailyStudyMs   sql.NullInt64 `json:"dailyStudyMs"`
	WeeklyStudyMs  sql.NullInt64 `json:"weeklyStudyMs"`
	MonthlyStudyMs sql.NullInt64 `json:"monthlyStudyMs"`
	CurrentStreak  sql.NullInt32 `json:"currentStreak"`
	MaxStreak      sql.NullInt32 `json:"maxStreak"`
	LastStreakDate sql.NullTime  `json:"lastStreakDate"`
	StreakFreezes  sql.NullInt32 `json:"streakFreezes"`
}

type UserStreak struct {
	UserID                      string        `json:"userId"`
	GuildID                     string        `json:"guildId"`
	CurrentStreakCount          int32         `json:"currentStreakCount"`
	MaxStreakCount              int32         `json:"maxStreakCount"`
	WarningNotifiedAt           sql.NullTime  `json:"warningNotifiedAt"`
	CreatedAt                   time.Time     `json:"createdAt"`
	UpdatedAt                   time.Time     `json:"updatedAt"`
	LastStreakActivityTimestamp sql.NullTime  `json:"lastStreakActivityTimestamp"`
	LastActivityDate            sql.NullTime  `json:"lastActivityDate"`
	StreakEvaluatedDate         sql.NullTime  `json:"streakEvaluatedDate"`
	DailyActivityMinutes        sql.NullInt32 `json:"dailyActivityMinutes"`
	ActivityStartTime           sql.NullTime  `json:"activityStartTime"`
	StreakIncrementedToday      bool          `json:"streakIncrementedToday"`
}
