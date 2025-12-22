package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/jmoiron/sqlx"
	"github.com/nbd-wtf/go-nostr"
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

func (s *Storage) InitTrustedSyncSchema() error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	schema := `
	CREATE TABLE IF NOT EXISTS trusted_sync_state (
		pubkey TEXT PRIMARY KEY,
		last_synced_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_trusted_sync_last_synced ON trusted_sync_state(last_synced_at);

	CREATE TABLE IF NOT EXISTS trusted_sync_relay_stats (
		relay_url TEXT NOT NULL,
		pubkey TEXT NOT NULL,
		events_fetched INTEGER NOT NULL DEFAULT 0,
		last_sync_at INTEGER NOT NULL,
		PRIMARY KEY (relay_url, pubkey)
	);

	CREATE INDEX IF NOT EXISTS idx_trusted_sync_relay_stats_relay ON trusted_sync_relay_stats(relay_url);
	CREATE INDEX IF NOT EXISTS idx_trusted_sync_relay_stats_pubkey ON trusted_sync_relay_stats(pubkey);
	`

	_, err := dbConn.Exec(schema)
	return err
}

type TrustedSyncState struct {
	Pubkey       string
	LastSyncedAt int64
}

func (s *Storage) GetTrustedSyncQueue(ctx context.Context, pubkeys []string, limit int) ([]TrustedSyncState, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	if len(pubkeys) == 0 {
		return nil, nil
	}

	// Query returns pubkeys ordered by last_synced_at (nulls/missing first via LEFT JOIN)
	query := `
		WITH input_pubkeys(pk) AS (
			SELECT value FROM json_each(?)
		)
		SELECT ip.pk, COALESCE(ts.last_synced_at, 0) as last_synced_at
		FROM input_pubkeys ip
		LEFT JOIN trusted_sync_state ts ON ip.pk = ts.pubkey
		ORDER BY last_synced_at ASC
		LIMIT ?
	`

	// Convert pubkeys to JSON array for json_each
	pubkeysJSON := "["
	for i, pk := range pubkeys {
		if i > 0 {
			pubkeysJSON += ","
		}
		pubkeysJSON += fmt.Sprintf(`"%s"`, pk)
	}
	pubkeysJSON += "]"

	rows, err := dbConn.QueryContext(ctx, query, pubkeysJSON, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []TrustedSyncState
	for rows.Next() {
		var state TrustedSyncState
		if err := rows.Scan(&state.Pubkey, &state.LastSyncedAt); err != nil {
			return nil, err
		}
		states = append(states, state)
	}

	return states, rows.Err()
}

func (s *Storage) UpdateTrustedSyncState(ctx context.Context, pubkey string) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	now := time.Now().Unix()
	_, err := dbConn.ExecContext(ctx, `
		INSERT INTO trusted_sync_state (pubkey, last_synced_at)
		VALUES (?, ?)
		ON CONFLICT(pubkey) DO UPDATE SET last_synced_at = excluded.last_synced_at
	`, pubkey, now)

	return err
}

func (s *Storage) RecordTrustedSyncRelayStat(ctx context.Context, relayURL, pubkey string, eventsFetched int) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	now := time.Now().Unix()
	_, err := dbConn.ExecContext(ctx, `
		INSERT INTO trusted_sync_relay_stats (relay_url, pubkey, events_fetched, last_sync_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(relay_url, pubkey) DO UPDATE SET
			events_fetched = events_fetched + excluded.events_fetched,
			last_sync_at = excluded.last_sync_at
	`, relayURL, pubkey, eventsFetched, now)

	return err
}

type TrustedSyncRelayStat struct {
	RelayURL      string
	TotalEvents   int64
	UniquePubkeys int64
	LastSyncAt    int64
}

func (s *Storage) GetTrustedSyncRelayStats(ctx context.Context) ([]TrustedSyncRelayStat, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT
			relay_url,
			SUM(events_fetched) as total_events,
			COUNT(DISTINCT pubkey) as unique_pubkeys,
			MAX(last_sync_at) as last_sync_at
		FROM trusted_sync_relay_stats
		GROUP BY relay_url
		ORDER BY total_events DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []TrustedSyncRelayStat
	for rows.Next() {
		var stat TrustedSyncRelayStat
		if err := rows.Scan(&stat.RelayURL, &stat.TotalEvents, &stat.UniquePubkeys, &stat.LastSyncAt); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

type TrustedSyncPubkeyStat struct {
	Pubkey       string
	TotalEvents  int64
	RelayCount   int64
	LastSyncAt   int64
}

func (s *Storage) GetTrustedSyncPubkeyStats(ctx context.Context, limit int) ([]TrustedSyncPubkeyStat, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT
			pubkey,
			SUM(events_fetched) as total_events,
			COUNT(DISTINCT relay_url) as relay_count,
			MAX(last_sync_at) as last_sync_at
		FROM trusted_sync_relay_stats
		GROUP BY pubkey
		ORDER BY total_events DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []TrustedSyncPubkeyStat
	for rows.Next() {
		var stat TrustedSyncPubkeyStat
		if err := rows.Scan(&stat.Pubkey, &stat.TotalEvents, &stat.RelayCount, &stat.LastSyncAt); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

func (s *Storage) GetTrustedSyncTotalStats(ctx context.Context) (totalEvents int64, totalPubkeys int64, totalRelays int64, err error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, 0, 0, nil
	}

	err = dbConn.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(events_fetched), 0),
			COUNT(DISTINCT pubkey),
			COUNT(DISTINCT relay_url)
		FROM trusted_sync_relay_stats
	`).Scan(&totalEvents, &totalPubkeys, &totalRelays)

	return
}

func (s *Storage) GetPubkeyRelayList(ctx context.Context, pubkey string) ([]string, error) {
	// Get the latest kind 10002 event for this pubkey and extract write relay URLs
	events, err := s.QueryEvents(ctx, nostr.Filter{
		Kinds:   []int{10002},
		Authors: []string{pubkey},
		Limit:   1,
	})
	if err != nil {
		return nil, err
	}

	if len(events) == 0 {
		return nil, nil
	}

	var writeRelays []string
	for _, tag := range events[0].Tags {
		if len(tag) < 2 || tag[0] != "r" {
			continue
		}

		url := tag[1]

		// Check if it's a write relay (no marker = both, "write" = write only)
		// "read" marker means read-only, skip those
		if len(tag) >= 3 && tag[2] == "read" {
			continue
		}

		writeRelays = append(writeRelays, url)
	}

	return writeRelays, nil
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
