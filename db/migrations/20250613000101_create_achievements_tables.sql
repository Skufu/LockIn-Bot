-- +goose Up
-- +goose StatementBegin

-- Create achievements table with all 20 pre-seeded badges
CREATE TABLE IF NOT EXISTS achievements (
    achievement_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    icon TEXT NOT NULL,
    category TEXT NOT NULL,  -- 'streak', 'time', 'duration', 'competition', 'special'
    requirement_type TEXT NOT NULL,  -- How to check: 'streak_count', 'total_hours', 'session_hours', etc.
    requirement_value INTEGER NOT NULL,  -- The threshold value
    is_secret BOOLEAN DEFAULT FALSE,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create user achievements table to track earned badges
CREATE TABLE IF NOT EXISTS user_achievements (
    user_id TEXT NOT NULL,
    guild_id TEXT NOT NULL,
    achievement_id TEXT NOT NULL REFERENCES achievements(achievement_id),
    earned_at TIMESTAMPTZ DEFAULT NOW(),
    notified BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (user_id, guild_id, achievement_id)
);

-- Index for fast lookup of user's achievements
CREATE INDEX IF NOT EXISTS idx_user_achievements_user_guild ON user_achievements(user_id, guild_id);

-- Add featured_badge column to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS featured_badge TEXT REFERENCES achievements(achievement_id);

-- Seed the 20 achievements
-- Streak Badges (5)
INSERT INTO achievements (achievement_id, name, description, icon, category, requirement_type, requirement_value, sort_order) VALUES
('first_flame', 'First Flame', 'Start your study streak journey', '🔥', 'streak', 'streak_count', 3, 1),
('streak_starter', 'Streak Starter', 'Building momentum!', '⚡', 'streak', 'streak_count', 7, 2),
('consistent', 'Consistent', 'Two weeks of dedication', '🌟', 'streak', 'streak_count', 14, 3),
('monthly_master', 'Monthly Master', 'A full month of studying', '💫', 'streak', 'streak_count', 30, 4),
('legendary', 'Legendary', 'Unstoppable dedication', '🌈', 'streak', 'streak_count', 100, 5)
ON CONFLICT (achievement_id) DO NOTHING;

-- Time-Based Badges (5)
INSERT INTO achievements (achievement_id, name, description, icon, category, requirement_type, requirement_value, sort_order) VALUES
('early_bird', 'Early Bird', 'The early scholar catches success', '🌅', 'time', 'study_before_hour', 7, 10),
('night_owl', 'Night Owl', 'Burning the midnight oil', '🦉', 'time', 'study_after_hour', 0, 11),
('dawn_to_dusk', 'Dawn to Dusk', 'Study spanning sunrise to sunset', '🌙', 'time', 'daily_hours', 12, 12),
('weekend_warrior', 'Weekend Warrior', 'No days off!', '☀️', 'time', 'weekend_study', 1, 13),
('graveyard_shift', 'Graveyard Shift', 'Studying in the dead of night', '🎃', 'time', 'study_between_hours', 2, 14)
ON CONFLICT (achievement_id) DO NOTHING;

-- Duration Badges (5)
INSERT INTO achievements (achievement_id, name, description, icon, category, requirement_type, requirement_value, sort_order) VALUES
('getting_started', 'Getting Started', 'Every journey begins with one step', '⏱️', 'duration', 'total_hours', 1, 20),
('focused', 'Focused', 'Building your study habit', '🎯', 'duration', 'total_hours', 10, 21),
('bookworm', 'Bookworm', 'Serious dedication to learning', '📚', 'duration', 'total_hours', 50, 22),
('marathon_runner', 'Marathon Runner', 'A single legendary session', '💪', 'duration', 'session_hours', 5, 23),
('century_club', 'Century Club', 'Triple digit hero', '🏆', 'duration', 'total_hours', 100, 24)
ON CONFLICT (achievement_id) DO NOTHING;

-- Competition Badges (3)
INSERT INTO achievements (achievement_id, name, description, icon, category, requirement_type, requirement_value, sort_order) VALUES
('rising_star', 'Rising Star', 'Making your mark', '📈', 'competition', 'leaderboard_rank', 10, 30),
('study_king', 'Study King', 'Wear the crown', '👑', 'competition', 'leaderboard_rank', 1, 31),
('undefeated', 'Undefeated', 'Dominant performance', '🥇', 'competition', 'rank_one_days', 7, 32)
ON CONFLICT (achievement_id) DO NOTHING;

-- Special Badges (2)
INSERT INTO achievements (achievement_id, name, description, icon, category, requirement_type, requirement_value, is_secret, sort_order) VALUES
('comeback_kid', 'Comeback Kid', 'Bounced back from a broken streak', '🎭', 'special', 'streak_comeback', 7, TRUE, 40),
('global_citizen', 'Global Citizen', 'Around the clock studying', '🌍', 'special', 'unique_hours', 12, FALSE, 41)
ON CONFLICT (achievement_id) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE users DROP COLUMN IF EXISTS featured_badge;
DROP INDEX IF EXISTS idx_user_achievements_user_guild;
DROP TABLE IF EXISTS user_achievements;
DROP TABLE IF EXISTS achievements;

-- +goose StatementEnd
