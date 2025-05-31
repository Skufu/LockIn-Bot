CREATE SEQUENCE study_sessions_session_id_seq;

CREATE TABLE goose_db_version (
    id INTEGER NOT NULL,
    version_id BIGINT NOT NULL,
    is_applied BOOLEAN NOT NULL,
    tstamp TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW() NOT NULL
);

CREATE TABLE study_sessions (
    session_id INTEGER DEFAULT nextval('study_sessions_session_id_seq'::regclass) NOT NULL,
    user_id TEXT,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE,
    duration_ms BIGINT
);

CREATE TABLE user_stats (
    user_id TEXT NOT NULL,
    total_study_ms BIGINT DEFAULT 0,
    daily_study_ms BIGINT DEFAULT 0,
    weekly_study_ms BIGINT DEFAULT 0,
    monthly_study_ms BIGINT DEFAULT 0,
    current_streak INTEGER DEFAULT 0,
    max_streak INTEGER DEFAULT 0,
    last_streak_date DATE,
    streak_freezes INTEGER DEFAULT 0
);

CREATE TABLE user_streaks (
    user_id TEXT NOT NULL,
    guild_id TEXT NOT NULL,
    current_streak_count INTEGER DEFAULT 0 NOT NULL,
    max_streak_count INTEGER DEFAULT 0 NOT NULL,
    warning_notified_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    last_streak_activity_timestamp TIMESTAMP WITH TIME ZONE
);

CREATE TABLE users (
    user_id TEXT NOT NULL,
    username TEXT
);