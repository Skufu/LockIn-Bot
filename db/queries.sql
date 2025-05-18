-- name: CreateUser :one
INSERT INTO users (user_id, username)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET username = $2
RETURNING *;

-- name: GetUser :one
SELECT * FROM users
WHERE user_id = $1;

-- name: CreateStudySession :one
INSERT INTO study_sessions (user_id, start_time)
VALUES ($1, $2)
RETURNING *;

-- name: EndStudySession :one
UPDATE study_sessions
SET end_time = $2, duration_ms = EXTRACT(EPOCH FROM ($2 - start_time)) * 1000
WHERE session_id = $1 AND end_time IS NULL
RETURNING *;

-- name: GetActiveStudySession :one
SELECT * FROM study_sessions
WHERE user_id = $1 AND end_time IS NULL
LIMIT 1;

-- name: GetUserStats :one
SELECT * FROM user_stats
WHERE user_id = $1;

-- name: CreateOrUpdateUserStats :one
INSERT INTO user_stats (user_id, total_study_ms, daily_study_ms, weekly_study_ms, monthly_study_ms)
VALUES ($1, $2, $2, $2, $2)
ON CONFLICT (user_id) DO UPDATE
SET 
  total_study_ms = user_stats.total_study_ms + $2,
  daily_study_ms = user_stats.daily_study_ms + $2,
  weekly_study_ms = user_stats.weekly_study_ms + $2,
  monthly_study_ms = user_stats.monthly_study_ms + $2
RETURNING *;

-- name: ResetDailyStudyTime :exec
UPDATE user_stats
SET daily_study_ms = 0;

-- name: ResetWeeklyStudyTime :exec
UPDATE user_stats
SET weekly_study_ms = 0;

-- name: ResetMonthlyStudyTime :exec
UPDATE user_stats
SET monthly_study_ms = 0;

-- name: GetLeaderboard :many
SELECT
    u.username,
    us.total_study_ms,
    u.user_id -- Also select user_id for mentions
FROM
    user_stats us
JOIN
    users u ON us.user_id = u.user_id
WHERE
    us.total_study_ms > 0 -- Only show users who have studied
ORDER BY
    us.total_study_ms DESC
LIMIT 10; -- For top 10 users

-- name: DeleteOldStudySessions :exec
DELETE FROM study_sessions
WHERE start_time < $1; -- $1 will be the cutoff timestamp (e.g., 6 months ago)

-- User Streaks Queries

-- name: GetUserStreak :one
SELECT 
    user_id, 
    guild_id, 
    current_streak_count, 
    max_streak_count, 
    last_streak_activity_timestamp, 
    warning_notified_at,
    created_at,
    updated_at
FROM user_streaks
WHERE user_id = $1 AND guild_id = $2;

-- name: UpsertUserStreak :one
INSERT INTO user_streaks (
    user_id, 
    guild_id, 
    current_streak_count, 
    max_streak_count, 
    last_streak_activity_timestamp, 
    warning_notified_at,
    updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
ON CONFLICT (user_id, guild_id) DO UPDATE SET
    current_streak_count = EXCLUDED.current_streak_count,
    max_streak_count = GREATEST(user_streaks.max_streak_count, EXCLUDED.max_streak_count),
    last_streak_activity_timestamp = EXCLUDED.last_streak_activity_timestamp,
    warning_notified_at = EXCLUDED.warning_notified_at,
    updated_at = NOW()
RETURNING user_id, guild_id, current_streak_count, max_streak_count, last_streak_activity_timestamp, warning_notified_at, created_at, updated_at;

-- name: GetStreaksToReset :many
SELECT 
    user_id, 
    guild_id, 
    current_streak_count, 
    max_streak_count, 
    last_streak_activity_timestamp, 
    warning_notified_at,
    created_at,
    updated_at
FROM user_streaks
WHERE current_streak_count > 0 
  AND last_streak_activity_timestamp < (NOW() - interval '24 hours');

-- name: UpdateStreakWarningNotifiedAt :exec
UPDATE user_streaks
SET warning_notified_at = $1, updated_at = NOW()
WHERE user_id = $2 AND guild_id = $3;

-- name: GetStreaksToWarn :many
SELECT 
    user_id, 
    guild_id, 
    current_streak_count, 
    max_streak_count, 
    last_streak_activity_timestamp, 
    warning_notified_at,
    created_at,
    updated_at
FROM user_streaks
WHERE current_streak_count > 0
  AND last_streak_activity_timestamp BETWEEN (NOW() - interval '24 hours') AND (NOW() - interval '22 hours')
  AND (warning_notified_at IS NULL OR warning_notified_at < (NOW() - interval '23 hours')); -- Avoid re-warning too soon

-- name: ResetUserStreakCount :exec
UPDATE user_streaks
SET current_streak_count = 0, updated_at = NOW()
WHERE user_id = $1 AND guild_id = $2;