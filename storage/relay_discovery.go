package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/jmoiron/sqlx"
)

type DiscoveredRelay struct {
	URL               string
	FirstSeen         time.Time
	LastSync          time.Time
	SyncAttempts      int64
	SyncSuccesses     int64
	EventsContributed int64
	IsActive          bool
}

func (s *Storage) getDBConn() *sqlx.DB {
	switch db := s.db.(type) {
	case *sqlite3.SQLite3Backend:
		return db.DB
	default:
		return nil
	}
}

func (s *Storage) InitRelayDiscoverySchema() error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	schema := `
	CREATE TABLE IF NOT EXISTS discovered_relays (
		url TEXT PRIMARY KEY,
		first_seen INTEGER NOT NULL,
		last_sync INTEGER NOT NULL DEFAULT 0,
		sync_attempts INTEGER NOT NULL DEFAULT 0,
		sync_successes INTEGER NOT NULL DEFAULT 0,
		events_contributed INTEGER NOT NULL DEFAULT 0,
		is_active INTEGER NOT NULL DEFAULT 1
	);

	CREATE INDEX IF NOT EXISTS idx_last_sync ON discovered_relays(last_sync);
	CREATE INDEX IF NOT EXISTS idx_is_active ON discovered_relays(is_active);
	`

	_, err := dbConn.Exec(schema)
	return err
}

func (s *Storage) AddDiscoveredRelay(ctx context.Context, url string) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	now := time.Now().Unix()
	_, err := dbConn.ExecContext(ctx, `
		INSERT INTO discovered_relays (url, first_seen, is_active)
		VALUES (?, ?, 1)
		ON CONFLICT(url) DO NOTHING
	`, url, now)

	return err
}

func (s *Storage) GetRelayQueue(ctx context.Context) ([]DiscoveredRelay, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT url, first_seen, last_sync, sync_attempts, sync_successes, events_contributed, is_active
		FROM discovered_relays
		WHERE is_active = 1
		ORDER BY last_sync ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relays []DiscoveredRelay
	for rows.Next() {
		var r DiscoveredRelay
		var firstSeen, lastSync int64
		var isActive int

		err := rows.Scan(&r.URL, &firstSeen, &lastSync, &r.SyncAttempts, &r.SyncSuccesses, &r.EventsContributed, &isActive)
		if err != nil {
			return nil, err
		}

		r.FirstSeen = time.Unix(firstSeen, 0)
		r.LastSync = time.Unix(lastSync, 0)
		r.IsActive = isActive == 1

		relays = append(relays, r)
	}

	return relays, rows.Err()
}

func (s *Storage) UpdateSyncStats(ctx context.Context, url string, success bool, eventsContributed int64) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	now := time.Now().Unix()

	if success {
		_, err := dbConn.ExecContext(ctx, `
			UPDATE discovered_relays
			SET last_sync = ?,
			    sync_attempts = sync_attempts + 1,
			    sync_successes = sync_successes + 1,
			    events_contributed = events_contributed + ?
			WHERE url = ?
		`, now, eventsContributed, url)
		return err
	}

	_, err := dbConn.ExecContext(ctx, `
		UPDATE discovered_relays
		SET sync_attempts = sync_attempts + 1
		WHERE url = ?
	`, url)
	return err
}

func (s *Storage) GetRelayStats(ctx context.Context) ([]DiscoveredRelay, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT url, first_seen, last_sync, sync_attempts, sync_successes, events_contributed, is_active
		FROM discovered_relays
		ORDER BY events_contributed DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relays []DiscoveredRelay
	for rows.Next() {
		var r DiscoveredRelay
		var firstSeen, lastSync int64
		var isActive int

		err := rows.Scan(&r.URL, &firstSeen, &lastSync, &r.SyncAttempts, &r.SyncSuccesses, &r.EventsContributed, &isActive)
		if err != nil {
			return nil, err
		}

		r.FirstSeen = time.Unix(firstSeen, 0)
		r.LastSync = time.Unix(lastSync, 0)
		r.IsActive = isActive == 1

		relays = append(relays, r)
	}

	return relays, rows.Err()
}

func (s *Storage) GetDiscoveredRelayCount(ctx context.Context) (int64, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, nil
	}

	var count int64
	err := dbConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM discovered_relays`).Scan(&count)
	return count, err
}

func (s *Storage) InitProfileHydrationSchema() error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	schema := `
	CREATE TABLE IF NOT EXISTS profile_fetch_attempts (
		pubkey TEXT PRIMARY KEY,
		last_attempt INTEGER NOT NULL,
		fetched_kind_0 INTEGER NOT NULL DEFAULT 0,
		fetched_kind_3 INTEGER NOT NULL DEFAULT 0,
		fetched_kind_10002 INTEGER NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_last_attempt ON profile_fetch_attempts(last_attempt);
	`

	_, err := dbConn.Exec(schema)
	return err
}

type ProfileFetchAttempt struct {
	Pubkey          string
	LastAttempt     int64
	FetchedKind0    bool
	FetchedKind3    bool
	FetchedKind10002 bool
}

func (s *Storage) GetProfileFetchAttempt(ctx context.Context, pubkey string) (*ProfileFetchAttempt, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	var attempt ProfileFetchAttempt
	var k0, k3, k10002 int

	err := dbConn.QueryRowContext(ctx, `
		SELECT pubkey, last_attempt, fetched_kind_0, fetched_kind_3, fetched_kind_10002
		FROM profile_fetch_attempts
		WHERE pubkey = ?
	`, pubkey).Scan(&attempt.Pubkey, &attempt.LastAttempt, &k0, &k3, &k10002)

	if err != nil {
		return nil, nil
	}

	attempt.FetchedKind0 = k0 == 1
	attempt.FetchedKind3 = k3 == 1
	attempt.FetchedKind10002 = k10002 == 1

	return &attempt, nil
}

func (s *Storage) RecordProfileFetchAttempt(ctx context.Context, pubkey string, fetchedK0, fetchedK3, fetchedK10002 bool) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	now := time.Now().Unix()
	k0, k3, k10002 := 0, 0, 0
	if fetchedK0 {
		k0 = 1
	}
	if fetchedK3 {
		k3 = 1
	}
	if fetchedK10002 {
		k10002 = 1
	}

	_, err := dbConn.ExecContext(ctx, `
		INSERT INTO profile_fetch_attempts (pubkey, last_attempt, fetched_kind_0, fetched_kind_3, fetched_kind_10002)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(pubkey) DO UPDATE SET
			last_attempt = excluded.last_attempt,
			fetched_kind_0 = CASE WHEN excluded.fetched_kind_0 = 1 THEN 1 ELSE fetched_kind_0 END,
			fetched_kind_3 = CASE WHEN excluded.fetched_kind_3 = 1 THEN 1 ELSE fetched_kind_3 END,
			fetched_kind_10002 = CASE WHEN excluded.fetched_kind_10002 = 1 THEN 1 ELSE fetched_kind_10002 END
	`, pubkey, now, k0, k3, k10002)

	return err
}

func (s *Storage) GetProfileFetchAttemptCount(ctx context.Context) (int64, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, nil
	}

	var count int64
	err := dbConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM profile_fetch_attempts`).Scan(&count)
	return count, err
}

func (s *Storage) GetRecentProfileFetchAttempts(ctx context.Context, limit int) ([]ProfileFetchAttempt, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT pubkey, last_attempt, fetched_kind_0, fetched_kind_3, fetched_kind_10002
		FROM profile_fetch_attempts
		ORDER BY last_attempt DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []ProfileFetchAttempt
	for rows.Next() {
		var attempt ProfileFetchAttempt
		var k0, k3, k10002 int

		err := rows.Scan(&attempt.Pubkey, &attempt.LastAttempt, &k0, &k3, &k10002)
		if err != nil {
			return nil, err
		}

		attempt.FetchedKind0 = k0 == 1
		attempt.FetchedKind3 = k3 == 1
		attempt.FetchedKind10002 = k10002 == 1

		attempts = append(attempts, attempt)
	}

	return attempts, rows.Err()
}

func (s *Storage) BackupTo(destPath string) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return fmt.Errorf("database connection not available")
	}

	// Simple file-based backup for SQLite
	// Note: This uses VACUUM INTO which creates a clean copy
	_, err := dbConn.Exec(fmt.Sprintf("VACUUM INTO '%s'", destPath))
	return err
}

func (s *Storage) GetFollowerCounts(ctx context.Context, minFollowers int) (map[string]int, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	// Optimized query: find latest kind 3 per author, extract p tags, count followers
	query := `
		WITH latest_contact_lists AS (
			SELECT e1.id, e1.tags
			FROM event e1
			INNER JOIN (
				SELECT pubkey, MAX(created_at) as max_created_at
				FROM event
				WHERE kind = 3
				GROUP BY pubkey
			) e2 ON e1.pubkey = e2.pubkey AND e1.created_at = e2.max_created_at
			WHERE e1.kind = 3
		)
		SELECT
			json_extract(tag.value, '$[1]') as followed_pubkey,
			COUNT(*) as follower_count
		FROM latest_contact_lists,
			json_each(latest_contact_lists.tags) as tag
		WHERE json_extract(tag.value, '$[0]') = 'p'
		  AND json_extract(tag.value, '$[1]') IS NOT NULL
		GROUP BY followed_pubkey
		HAVING follower_count >= ?
	`

	rows, err := dbConn.QueryContext(ctx, query, minFollowers)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	followerCounts := make(map[string]int)
	for rows.Next() {
		var pubkey string
		var count int
		if err := rows.Scan(&pubkey, &count); err != nil {
			return nil, err
		}
		followerCounts[pubkey] = count
	}

	return followerCounts, rows.Err()
}

type PubkeyEventKinds struct {
	Pubkey        string
	HasKind0      bool
	HasKind3      bool
	HasKind10002  bool
}

func (s *Storage) CheckPubkeyEventKinds(ctx context.Context, pubkeys []string) (map[string]PubkeyEventKinds, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	if len(pubkeys) == 0 {
		return make(map[string]PubkeyEventKinds), nil
	}

	// Build query with placeholders for all pubkeys
	placeholders := make([]interface{}, len(pubkeys))
	for i, pk := range pubkeys {
		placeholders[i] = pk
	}

	// Single query that checks all three kinds for all pubkeys
	query := `
		SELECT
			pubkey,
			MAX(CASE WHEN kind = 0 THEN 1 ELSE 0 END) as has_kind_0,
			MAX(CASE WHEN kind = 3 THEN 1 ELSE 0 END) as has_kind_3,
			MAX(CASE WHEN kind = 10002 THEN 1 ELSE 0 END) as has_kind_10002
		FROM event
		WHERE pubkey IN (?` + string(make([]byte, len(pubkeys)-1)) + `)
		  AND kind IN (0, 3, 10002)
		GROUP BY pubkey
	`

	// Replace placeholders
	query = "SELECT pubkey, MAX(CASE WHEN kind = 0 THEN 1 ELSE 0 END) as has_kind_0, MAX(CASE WHEN kind = 3 THEN 1 ELSE 0 END) as has_kind_3, MAX(CASE WHEN kind = 10002 THEN 1 ELSE 0 END) as has_kind_10002 FROM event WHERE pubkey IN (?"
	for i := 1; i < len(pubkeys); i++ {
		query += ",?"
	}
	query += ") AND kind IN (0, 3, 10002) GROUP BY pubkey"

	rows, err := dbConn.QueryContext(ctx, query, placeholders...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Initialize result map with all pubkeys (default: no events)
	result := make(map[string]PubkeyEventKinds)
	for _, pk := range pubkeys {
		result[pk] = PubkeyEventKinds{Pubkey: pk}
	}

	// Update with actual data
	for rows.Next() {
		var pubkey string
		var hasK0, hasK3, hasK10002 int
		if err := rows.Scan(&pubkey, &hasK0, &hasK3, &hasK10002); err != nil {
			return nil, err
		}
		result[pubkey] = PubkeyEventKinds{
			Pubkey:       pubkey,
			HasKind0:     hasK0 == 1,
			HasKind3:     hasK3 == 1,
			HasKind10002: hasK10002 == 1,
		}
	}

	return result, rows.Err()
}
