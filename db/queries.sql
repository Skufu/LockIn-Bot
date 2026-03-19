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
/* ACTIVE_SESSION_QUERY_1_PARAM */ SELECT session_id, user_id, start_time, end_time, duration_ms FROM study_sessions
WHERE user_id = $1 AND end_time IS NULL
ORDER BY start_time DESC
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

-- name: DeleteAllStudySessions :exec
DELETE FROM study_sessions;

-- name: CountStudySessions :one
SELECT COUNT(*) FROM study_sessions;

-- name: DeleteOldStudySessionsWithCount :one
WITH deleted AS (
    DELETE FROM study_sessions
    WHERE start_time < $1
    RETURNING session_id
)
SELECT COUNT(*) FROM deleted;

-- Calendar Day-Based User Streaks Queries

-- name: GetUserStreak :one
SELECT
    user_id,
    guild_id,
    current_streak_count,
    max_streak_count,
    last_activity_date,
    streak_evaluated_date,
    daily_activity_minutes,
    activity_start_time,
    streak_incremented_today,
    warning_notified_at,
    created_at,
    updated_at
FROM user_streaks
WHERE user_id = $1 AND guild_id = $2;

-- name: HasActivityForDate :one
/* ACTIVITY_CHECK_QUERY_4_PARAM */ SELECT EXISTS(
    SELECT 1 FROM user_streaks 
    WHERE user_id = $1 
    AND guild_id = $2 
    AND last_activity_date = $3 
    AND daily_activity_minutes >= $4
);

-- name: StartDailyActivity :one
INSERT INTO user_streaks (
    user_id, 
    guild_id, 
    current_streak_count,
    max_streak_count,
    last_activity_date,
    daily_activity_minutes,
    activity_start_time,
    streak_incremented_today,
    updated_at
)
VALUES ($1, $2, 0, 0, $3, 0, $4, FALSE, NOW())
ON CONFLICT (user_id, guild_id) DO UPDATE SET
    last_activity_date = CASE 
        WHEN user_streaks.last_activity_date != $3 THEN $3
        ELSE user_streaks.last_activity_date
    END,
    daily_activity_minutes = CASE 
        WHEN user_streaks.last_activity_date != $3 THEN 0
        ELSE user_streaks.daily_activity_minutes
    END,
    activity_start_time = CASE 
        WHEN user_streaks.last_activity_date != $3 THEN $4
        WHEN user_streaks.activity_start_time IS NULL THEN $4
        ELSE user_streaks.activity_start_time
    END,
    streak_incremented_today = CASE 
        WHEN user_streaks.last_activity_date != $3 THEN FALSE
        ELSE user_streaks.streak_incremented_today
    END,
    updated_at = NOW()
RETURNING user_id, guild_id, current_streak_count, max_streak_count, last_activity_date, streak_evaluated_date, daily_activity_minutes, activity_start_time, streak_incremented_today, warning_notified_at, created_at, updated_at;

-- name: UpdateDailyActivityMinutes :exec
UPDATE user_streaks
SET 
    daily_activity_minutes = $3,
    updated_at = NOW()
WHERE user_id = $1 AND guild_id = $2;

-- name: GetUsersForDailyEvaluation :many
SELECT 
    user_id, 
    guild_id, 
    current_streak_count, 
    max_streak_count, 
    last_activity_date,
    streak_evaluated_date,
    daily_activity_minutes,
    warning_notified_at,
    created_at,
    updated_at
FROM user_streaks
WHERE streak_evaluated_date IS NULL 
   OR streak_evaluated_date < $1; -- $1 is today's date in Manila timezone

-- name: UpdateUserStreakAfterEvaluation :one
UPDATE user_streaks
SET 
    current_streak_count = $3,
    max_streak_count = GREATEST(max_streak_count, $4),
    streak_evaluated_date = $5,
    updated_at = NOW()
WHERE user_id = $1 AND guild_id = $2
RETURNING user_id, guild_id, current_streak_count, max_streak_count, last_activity_date, streak_evaluated_date, daily_activity_minutes, activity_start_time, warning_notified_at, created_at, updated_at;

-- name: GetUsersNeedingWarnings :many
SELECT 
    user_id, 
    guild_id, 
    current_streak_count, 
    max_streak_count, 
    last_activity_date,
    daily_activity_minutes,
    warning_notified_at,
    created_at,
    updated_at
FROM user_streaks
WHERE current_streak_count > 0
  AND (last_activity_date IS NULL OR last_activity_date < $1) -- Haven't been active today
  AND (warning_notified_at IS NULL OR DATE(warning_notified_at) < $1); -- Haven't been warned today

-- name: UpdateWarningNotifiedAt :exec
UPDATE user_streaks
SET 
    warning_notified_at = $3,
    updated_at = NOW()
WHERE user_id = $1 AND guild_id = $2;

-- name: UpdateStreakImmediately :exec
UPDATE user_streaks
SET 
    current_streak_count = $3,
    max_streak_count = GREATEST(max_streak_count, $4),
    streak_incremented_today = TRUE,
    updated_at = NOW()
WHERE user_id = $1 AND guild_id = $2;

-- name: ResetAllStreakDailyFlags :exec
UPDATE user_streaks
SET 
    streak_incremented_today = FALSE,
    updated_at = NOW();

-- name: GetUsersForStreakReset :many
SELECT 
    user_id, 
    guild_id, 
    current_streak_count
FROM user_streaks
WHERE current_streak_count > 0
  AND streak_incremented_today = FALSE
  AND (last_activity_date IS NULL OR last_activity_date < $1); -- Haven't been active today

-- name: ResetUserStreakCount :exec
UPDATE user_streaks
SET 
    current_streak_count = 0,
    updated_at = NOW()
WHERE user_id = $1 AND guild_id = $2;

-- =============================================
-- Achievement System Queries
-- =============================================

-- name: GetAllAchievements :many
SELECT 
    achievement_id,
    name,
    description,
    icon,
    category,
    requirement_type,
    requirement_value,
    is_secret,
    sort_order
FROM achievements
ORDER BY sort_order ASC;

-- name: GetAchievementByID :one
SELECT 
    achievement_id,
    name,
    description,
    icon,
    category,
    requirement_type,
    requirement_value,
    is_secret,
    sort_order
FROM achievements
WHERE achievement_id = $1;

-- name: GetUserAchievements :many
SELECT 
    ua.user_id,
    ua.guild_id,
    ua.achievement_id,
    ua.earned_at,
    ua.notified,
    a.name,
    a.description,
    a.icon,
    a.category,
    a.sort_order
FROM user_achievements ua
JOIN achievements a ON ua.achievement_id = a.achievement_id
WHERE ua.user_id = $1 AND ua.guild_id = $2
ORDER BY a.sort_order ASC;

-- name: GetUserAchievementCount :one
SELECT COUNT(*) as count
FROM user_achievements
WHERE user_id = $1 AND guild_id = $2;

-- name: HasAchievement :one
SELECT EXISTS(
    SELECT 1 FROM user_achievements 
    WHERE user_id = $1 
    AND guild_id = $2 
    AND achievement_id = $3
);

-- name: AwardAchievement :one
INSERT INTO user_achievements (user_id, guild_id, achievement_id, earned_at, notified)
VALUES ($1, $2, $3, NOW(), FALSE)
ON CONFLICT (user_id, guild_id, achievement_id) DO NOTHING
RETURNING user_id, guild_id, achievement_id, earned_at, notified;

-- name: MarkAchievementNotified :exec
UPDATE user_achievements
SET notified = TRUE
WHERE user_id = $1 AND guild_id = $2 AND achievement_id = $3;

-- name: GetUnnotifiedAchievements :many
SELECT 
    ua.user_id,
    ua.guild_id,
    ua.achievement_id,
    ua.earned_at,
    a.name,
    a.description,
    a.icon,
    a.category
FROM user_achievements ua
JOIN achievements a ON ua.achievement_id = a.achievement_id
WHERE ua.user_id = $1 AND ua.guild_id = $2 AND ua.notified = FALSE
ORDER BY ua.earned_at ASC;

-- name: GetAchievementsByCategory :many
SELECT 
    achievement_id,
    name,
    description,
    icon,
    category,
    requirement_type,
    requirement_value,
    is_secret,
    sort_order
FROM achievements
WHERE category = $1
ORDER BY sort_order ASC;

-- name: SetFeaturedBadge :exec
UPDATE users
SET featured_badge = $2
WHERE user_id = $1;

-- name: GetUserFeaturedBadge :one
SELECT 
    u.featured_badge,
    a.name,
    a.description,
    a.icon
FROM users u
LEFT JOIN achievements a ON u.featured_badge = a.achievement_id
WHERE u.user_id = $1;

-- name: GetTotalAchievementCount :one
SELECT COUNT(*) as count FROM achievements;

-- name: GetUniqueStudyHours :one
SELECT COUNT(DISTINCT EXTRACT(HOUR FROM start_time AT TIME ZONE 'Asia/Manila'))::integer
FROM study_sessions
WHERE user_id = $1;

-- name: HasDawnToDuskDay :one
SELECT EXISTS(
  SELECT 1 FROM (
    SELECT DATE(start_time AT TIME ZONE 'Asia/Manila') as study_date,
           SUM(EXTRACT(EPOCH FROM COALESCE(end_time, NOW()) - start_time)) / 3600 as hours
    FROM study_sessions
    WHERE user_id = $1
    GROUP BY study_date
    HAVING SUM(EXTRACT(EPOCH FROM COALESCE(end_time, NOW()) - start_time)) / 3600 >= 12
  ) as daily_hours
) as has_dawn_to_dusk;

-- name: GetAchievementsByRequirementType :many
SELECT 
    achievement_id,
    name,
    description,
    icon,
    category,
    requirement_type,
    requirement_value,
    is_secret,
    sort_order
FROM achievements
WHERE requirement_type = $1
ORDER BY requirement_value ASC;
