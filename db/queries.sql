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