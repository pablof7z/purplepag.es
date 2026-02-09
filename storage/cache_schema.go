package storage

func (s *Storage) InitCacheSchema() error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	schema := `
	CREATE TABLE IF NOT EXISTS cached_social_counts (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		updated_at INTEGER NOT NULL,
		mute_list_count INTEGER NOT NULL,
		interest_list_count INTEGER NOT NULL,
		community_list_count INTEGER NOT NULL,
		contact_list_count INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS follower_counts (
		pubkey TEXT PRIMARY KEY,
		follower_count INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_follower_counts_count ON follower_counts(follower_count DESC);

	CREATE TABLE IF NOT EXISTS follower_edges (
		follower TEXT NOT NULL,
		followed TEXT NOT NULL,
		PRIMARY KEY (follower, followed)
	);
	CREATE INDEX IF NOT EXISTS idx_follower_edges_follower ON follower_edges(follower);
	CREATE INDEX IF NOT EXISTS idx_follower_edges_followed ON follower_edges(followed);

	CREATE TABLE IF NOT EXISTS follower_trend_changes (
		day INTEGER NOT NULL,
		pubkey TEXT NOT NULL,
		gained INTEGER NOT NULL,
		lost INTEGER NOT NULL,
		PRIMARY KEY (day, pubkey)
	);
	CREATE INDEX IF NOT EXISTS idx_follower_trend_day ON follower_trend_changes(day);
	CREATE INDEX IF NOT EXISTS idx_follower_trend_pubkey ON follower_trend_changes(pubkey);

	CREATE TABLE IF NOT EXISTS cached_most_muted (
		rank INTEGER PRIMARY KEY,
		pubkey TEXT NOT NULL,
		mute_count INTEGER NOT NULL,
		follower_count INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS cached_top_interests (
		rank INTEGER PRIMARY KEY,
		interest TEXT NOT NULL,
		interest_count INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS cached_follower_trends (
		direction TEXT NOT NULL,
		rank INTEGER NOT NULL,
		pubkey TEXT NOT NULL,
		net_change INTEGER NOT NULL,
		gained INTEGER NOT NULL,
		lost INTEGER NOT NULL,
		PRIMARY KEY (direction, rank)
	);

	CREATE TABLE IF NOT EXISTS cached_relay_stats (
		rank INTEGER PRIMARY KEY,
		url TEXT NOT NULL,
		pubkey_count INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_relay_stats_count ON cached_relay_stats(pubkey_count DESC);
	`

	_, err := dbConn.Exec(schema)
	return err
}
