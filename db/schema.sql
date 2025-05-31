-- Create users table
CREATE TABLE IF NOT EXISTS users (
    user_id TEXT PRIMARY KEY,
    username TEXT
);

-- Create study sessions table
CREATE TABLE IF NOT EXISTS study_sessions (
    session_id SERIAL PRIMARY KEY,
    user_id TEXT REFERENCES users(user_id),
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ,
    duration_ms BIGINT
);

-- Create user stats table
CREATE TABLE IF NOT EXISTS user_stats (
    user_id TEXT PRIMARY KEY REFERENCES users(user_id),
    total_study_ms BIGINT DEFAULT 0,
    daily_study_ms BIGINT DEFAULT 0,
    weekly_study_ms BIGINT DEFAULT 0,
    monthly_study_ms BIGINT DEFAULT 0,
    current_streak INTEGER DEFAULT 0,
    max_streak INTEGER DEFAULT 0,
    last_streak_date DATE,
    streak_freezes INTEGER DEFAULT 0
);