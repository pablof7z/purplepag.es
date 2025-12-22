package storage

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type PubkeyStats struct {
	Pubkey        string
	TotalRequests int64
	LastRequest   time.Time
	ByKind        map[int]int64
}

type CooccurrencePair struct {
	PubkeyA string
	PubkeyB string
	Count   int64
}

type BotCluster struct {
	ID              int64
	DetectedAt      time.Time
	Size            int
	InternalDensity float64
	ExternalRatio   float64
	Members         []string
	IsActive        bool
}

type SpamCandidate struct {
	Pubkey     string
	DetectedAt time.Time
	Reason     string
	EventCount int64
	Purged     bool
}

func (s *Storage) InitAnalyticsSchema() error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	schema := `
	CREATE TABLE IF NOT EXISTS req_analytics (
		pubkey TEXT PRIMARY KEY,
		total_requests INTEGER NOT NULL DEFAULT 0,
		last_request INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_req_total ON req_analytics(total_requests DESC);

	CREATE TABLE IF NOT EXISTS req_analytics_by_kind (
		pubkey TEXT NOT NULL,
		kind INTEGER NOT NULL,
		request_count INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (pubkey, kind)
	);
	CREATE INDEX IF NOT EXISTS idx_req_kind_count ON req_analytics_by_kind(request_count DESC);
	CREATE INDEX IF NOT EXISTS idx_req_kind_pubkey ON req_analytics_by_kind(pubkey);

	CREATE TABLE IF NOT EXISTS req_cooccurrence (
		pair_key TEXT PRIMARY KEY,
		count INTEGER NOT NULL DEFAULT 0,
		last_seen INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_cooccur_count ON req_cooccurrence(count DESC);

	CREATE TABLE IF NOT EXISTS bot_clusters (
		cluster_id INTEGER PRIMARY KEY AUTOINCREMENT,
		detected_at INTEGER NOT NULL,
		cluster_size INTEGER NOT NULL,
		internal_density REAL NOT NULL,
		external_ratio REAL NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS bot_cluster_members (
		cluster_id INTEGER NOT NULL,
		pubkey TEXT NOT NULL,
		PRIMARY KEY (cluster_id, pubkey),
		FOREIGN KEY (cluster_id) REFERENCES bot_clusters(cluster_id)
	);
	CREATE INDEX IF NOT EXISTS idx_cluster_member_pubkey ON bot_cluster_members(pubkey);

	CREATE TABLE IF NOT EXISTS spam_candidates (
		pubkey TEXT PRIMARY KEY,
		detected_at INTEGER NOT NULL,
		reason TEXT NOT NULL,
		event_count INTEGER NOT NULL,
		purged INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_spam_purged ON spam_candidates(purged);

	-- Rejected events by unsupported kind
	CREATE TABLE IF NOT EXISTS rejected_events_by_kind (
		kind INTEGER NOT NULL,
		pubkey TEXT NOT NULL,
		count INTEGER NOT NULL DEFAULT 0,
		last_seen INTEGER NOT NULL,
		PRIMARY KEY (kind, pubkey)
	);
	CREATE INDEX IF NOT EXISTS idx_rejected_events_kind ON rejected_events_by_kind(kind);
	CREATE INDEX IF NOT EXISTS idx_rejected_events_count ON rejected_events_by_kind(count DESC);

	-- REQ stats by kind (all REQs, for tracking over time)
	CREATE TABLE IF NOT EXISTS req_kind_stats (
		kind INTEGER PRIMARY KEY,
		total_requests INTEGER NOT NULL DEFAULT 0,
		last_request INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_req_kind_stats_total ON req_kind_stats(total_requests DESC);

	-- REQ stats by kind per day (for time series)
	CREATE TABLE IF NOT EXISTS req_kind_stats_daily (
		date TEXT NOT NULL,
		kind INTEGER NOT NULL,
		request_count INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (date, kind)
	);
	CREATE INDEX IF NOT EXISTS idx_req_kind_daily_date ON req_kind_stats_daily(date);

	-- Rejected REQs for unsupported kinds
	CREATE TABLE IF NOT EXISTS rejected_req_kinds (
		kind INTEGER PRIMARY KEY,
		count INTEGER NOT NULL DEFAULT 0,
		last_seen INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_rejected_req_count ON rejected_req_kinds(count DESC);

	-- Trusted pubkeys (persisted from trust analysis)
	CREATE TABLE IF NOT EXISTS trusted_pubkeys (
		pubkey TEXT PRIMARY KEY,
		trusted_at INTEGER NOT NULL
	);

	-- Social graph communities
	CREATE TABLE IF NOT EXISTS communities (
		id INTEGER PRIMARY KEY,
		size INTEGER NOT NULL,
		internal_edges INTEGER NOT NULL,
		external_edges INTEGER NOT NULL,
		modularity REAL NOT NULL,
		top_members TEXT NOT NULL,
		detected_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS community_members (
		community_id INTEGER NOT NULL,
		pubkey TEXT NOT NULL,
		PRIMARY KEY (community_id, pubkey)
	);
	CREATE INDEX IF NOT EXISTS idx_community_member_pubkey ON community_members(pubkey);

	CREATE TABLE IF NOT EXISTS community_edges (
		from_id INTEGER NOT NULL,
		to_id INTEGER NOT NULL,
		weight INTEGER NOT NULL,
		PRIMARY KEY (from_id, to_id)
	);

	CREATE TABLE IF NOT EXISTS community_stats (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		total_nodes INTEGER NOT NULL,
		total_edges INTEGER NOT NULL,
		num_communities INTEGER NOT NULL,
		detected_at INTEGER NOT NULL
	);
	`

	_, err := dbConn.Exec(schema)
	return err
}

func (s *Storage) FlushREQAnalytics(
	ctx context.Context,
	pubkeyRequests map[string]int64,
	pubkeyByKind map[string]map[int]int64,
	cooccurrence map[string]int64,
) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	now := time.Now().Unix()

	tx, err := dbConn.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for pubkey, count := range pubkeyRequests {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO req_analytics (pubkey, total_requests, last_request)
			VALUES (?, ?, ?)
			ON CONFLICT(pubkey) DO UPDATE SET
				total_requests = total_requests + excluded.total_requests,
				last_request = excluded.last_request
		`, pubkey, count, now)
		if err != nil {
			return err
		}
	}

	for pubkey, kindCounts := range pubkeyByKind {
		for kind, count := range kindCounts {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO req_analytics_by_kind (pubkey, kind, request_count)
				VALUES (?, ?, ?)
				ON CONFLICT(pubkey, kind) DO UPDATE SET
					request_count = request_count + excluded.request_count
			`, pubkey, kind, count)
			if err != nil {
				return err
			}
		}
	}

	for pairKey, count := range cooccurrence {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO req_cooccurrence (pair_key, count, last_seen)
			VALUES (?, ?, ?)
			ON CONFLICT(pair_key) DO UPDATE SET
				count = count + excluded.count,
				last_seen = excluded.last_seen
		`, pairKey, count, now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Storage) GetPubkeyAnalytics(ctx context.Context, pubkey string) (*PubkeyStats, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	var stats PubkeyStats
	var lastRequest int64

	err := dbConn.QueryRowContext(ctx, `
		SELECT pubkey, total_requests, last_request
		FROM req_analytics
		WHERE pubkey = ?
	`, pubkey).Scan(&stats.Pubkey, &stats.TotalRequests, &lastRequest)
	if err != nil {
		return nil, nil
	}

	stats.LastRequest = time.Unix(lastRequest, 0)
	stats.ByKind = make(map[int]int64)

	rows, err := dbConn.QueryContext(ctx, `
		SELECT kind, request_count
		FROM req_analytics_by_kind
		WHERE pubkey = ?
	`, pubkey)
	if err != nil {
		return &stats, nil
	}
	defer rows.Close()

	for rows.Next() {
		var kind int
		var count int64
		rows.Scan(&kind, &count)
		stats.ByKind[kind] = count
	}

	return &stats, nil
}

func (s *Storage) GetTopRequestedPubkeys(ctx context.Context, limit int) ([]PubkeyStats, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT pubkey, total_requests, last_request
		FROM req_analytics
		ORDER BY total_requests DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []PubkeyStats
	for rows.Next() {
		var stats PubkeyStats
		var lastRequest int64
		rows.Scan(&stats.Pubkey, &stats.TotalRequests, &lastRequest)
		stats.LastRequest = time.Unix(lastRequest, 0)
		results = append(results, stats)
	}

	return results, rows.Err()
}

func (s *Storage) GetTopCooccurrences(ctx context.Context, limit int) ([]CooccurrencePair, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT pair_key, count
		FROM req_cooccurrence
		ORDER BY count DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CooccurrencePair
	for rows.Next() {
		var pairKey string
		var count int64
		rows.Scan(&pairKey, &count)

		parts := strings.SplitN(pairKey, ":", 2)
		if len(parts) == 2 {
			results = append(results, CooccurrencePair{
				PubkeyA: parts[0],
				PubkeyB: parts[1],
				Count:   count,
			})
		}
	}

	return results, rows.Err()
}

func (s *Storage) SaveBotCluster(ctx context.Context, members []string, internalDensity, externalRatio float64) (int64, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, nil
	}

	now := time.Now().Unix()

	tx, err := dbConn.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO bot_clusters (detected_at, cluster_size, internal_density, external_ratio, is_active)
		VALUES (?, ?, ?, ?, 1)
	`, now, len(members), internalDensity, externalRatio)
	if err != nil {
		return 0, err
	}

	clusterID, _ := result.LastInsertId()

	for _, pubkey := range members {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO bot_cluster_members (cluster_id, pubkey)
			VALUES (?, ?)
		`, clusterID, pubkey)
		if err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return clusterID, nil
}

func (s *Storage) GetBotClusters(ctx context.Context, limit int) ([]BotCluster, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT cluster_id, detected_at, cluster_size, internal_density, external_ratio, is_active
		FROM bot_clusters
		WHERE is_active = 1
		ORDER BY detected_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clusters []BotCluster
	for rows.Next() {
		var c BotCluster
		var detectedAt int64
		var isActive int
		rows.Scan(&c.ID, &detectedAt, &c.Size, &c.InternalDensity, &c.ExternalRatio, &isActive)
		c.DetectedAt = time.Unix(detectedAt, 0)
		c.IsActive = isActive == 1
		clusters = append(clusters, c)
	}

	for i := range clusters {
		memberRows, err := dbConn.QueryContext(ctx, `
			SELECT pubkey FROM bot_cluster_members WHERE cluster_id = ?
		`, clusters[i].ID)
		if err != nil {
			continue
		}
		for memberRows.Next() {
			var pubkey string
			memberRows.Scan(&pubkey)
			clusters[i].Members = append(clusters[i].Members, pubkey)
		}
		memberRows.Close()
	}

	return clusters, rows.Err()
}

func (s *Storage) DeactivateBotClusters(ctx context.Context) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	_, err := dbConn.ExecContext(ctx, `UPDATE bot_clusters SET is_active = 0`)
	return err
}

func (s *Storage) IsPubkeyInBotCluster(ctx context.Context, pubkey string) (bool, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return false, nil
	}

	var count int
	err := dbConn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM bot_cluster_members bcm
		JOIN bot_clusters bc ON bcm.cluster_id = bc.cluster_id
		WHERE bcm.pubkey = ? AND bc.is_active = 1
	`, pubkey).Scan(&count)

	return count > 0, err
}

func (s *Storage) SaveSpamCandidate(ctx context.Context, pubkey, reason string, eventCount int64) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	now := time.Now().Unix()
	_, err := dbConn.ExecContext(ctx, `
		INSERT INTO spam_candidates (pubkey, detected_at, reason, event_count, purged)
		VALUES (?, ?, ?, ?, 0)
		ON CONFLICT(pubkey) DO UPDATE SET
			detected_at = excluded.detected_at,
			reason = excluded.reason,
			event_count = excluded.event_count
	`, pubkey, now, reason, eventCount)

	return err
}

func (s *Storage) GetSpamCandidates(ctx context.Context, limit int) ([]SpamCandidate, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT pubkey, detected_at, reason, event_count, purged
		FROM spam_candidates
		WHERE purged = 0
		ORDER BY event_count DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []SpamCandidate
	for rows.Next() {
		var c SpamCandidate
		var detectedAt int64
		var purged int
		rows.Scan(&c.Pubkey, &detectedAt, &c.Reason, &c.EventCount, &purged)
		c.DetectedAt = time.Unix(detectedAt, 0)
		c.Purged = purged == 1
		candidates = append(candidates, c)
	}

	return candidates, rows.Err()
}

func (s *Storage) MarkSpamPurged(ctx context.Context, pubkeys []string) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	for _, pubkey := range pubkeys {
		_, err := dbConn.ExecContext(ctx, `
			UPDATE spam_candidates SET purged = 1 WHERE pubkey = ?
		`, pubkey)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Storage) ClearSpamCandidates(ctx context.Context) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	_, err := dbConn.ExecContext(ctx, `DELETE FROM spam_candidates WHERE purged = 0`)
	return err
}

func (s *Storage) GetAllRequestedPubkeys(ctx context.Context) (map[string]int64, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT pubkey, total_requests FROM req_analytics
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var pubkey string
		var count int64
		rows.Scan(&pubkey, &count)
		result[pubkey] = count
	}

	return result, rows.Err()
}

func (s *Storage) CountEventsForPubkey(ctx context.Context, pubkey string) (int64, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, nil
	}

	var count int64
	err := dbConn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM event WHERE pubkey = ?
	`, pubkey).Scan(&count)

	return count, err
}

func (s *Storage) DeleteEventsForPubkeys(ctx context.Context, pubkeys []string) (int64, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, nil
	}

	var totalDeleted int64
	for _, pubkey := range pubkeys {
		result, err := dbConn.ExecContext(ctx, `DELETE FROM event WHERE pubkey = ?`, pubkey)
		if err != nil {
			return totalDeleted, err
		}
		deleted, _ := result.RowsAffected()
		totalDeleted += deleted
	}

	return totalDeleted, nil
}

// RecordRejectedEvent records an event that was rejected due to unsupported kind
func (s *Storage) RecordRejectedEvent(ctx context.Context, kind int, pubkey string) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	now := time.Now().Unix()
	_, err := dbConn.ExecContext(ctx, `
		INSERT INTO rejected_events_by_kind (kind, pubkey, count, last_seen)
		VALUES (?, ?, 1, ?)
		ON CONFLICT(kind, pubkey) DO UPDATE SET
			count = count + 1,
			last_seen = excluded.last_seen
	`, kind, pubkey, now)

	return err
}

type RejectedEventStat struct {
	Kind     int
	Pubkey   string
	Count    int64
	LastSeen time.Time
}

// GetRejectedEventStats returns stats on rejected events, optionally filtered
func (s *Storage) GetRejectedEventStats(ctx context.Context, limit int) ([]RejectedEventStat, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT kind, pubkey, count, last_seen
		FROM rejected_events_by_kind
		ORDER BY count DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []RejectedEventStat
	for rows.Next() {
		var stat RejectedEventStat
		var lastSeen int64
		if err := rows.Scan(&stat.Kind, &stat.Pubkey, &stat.Count, &lastSeen); err != nil {
			return nil, err
		}
		stat.LastSeen = time.Unix(lastSeen, 0)
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

type RejectedKindSummary struct {
	Kind         int
	TotalCount   int64
	UniquePubkeys int64
	LastSeen     time.Time
}

// GetRejectedEventsByKind returns aggregated stats per kind
func (s *Storage) GetRejectedEventsByKind(ctx context.Context, limit int) ([]RejectedKindSummary, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT kind, SUM(count) as total_count, COUNT(DISTINCT pubkey) as unique_pubkeys, MAX(last_seen) as last_seen
		FROM rejected_events_by_kind
		GROUP BY kind
		ORDER BY total_count DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []RejectedKindSummary
	for rows.Next() {
		var stat RejectedKindSummary
		var lastSeen int64
		if err := rows.Scan(&stat.Kind, &stat.TotalCount, &stat.UniquePubkeys, &lastSeen); err != nil {
			return nil, err
		}
		stat.LastSeen = time.Unix(lastSeen, 0)
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

// RecordRejectedREQ records a REQ for an unsupported kind
func (s *Storage) RecordRejectedREQ(ctx context.Context, kind int) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	now := time.Now().Unix()
	_, err := dbConn.ExecContext(ctx, `
		INSERT INTO rejected_req_kinds (kind, count, last_seen)
		VALUES (?, 1, ?)
		ON CONFLICT(kind) DO UPDATE SET
			count = count + 1,
			last_seen = excluded.last_seen
	`, kind, now)

	return err
}

type RejectedREQStat struct {
	Kind     int
	Count    int64
	LastSeen time.Time
}

// GetRejectedREQStats returns stats on rejected REQs by kind
func (s *Storage) GetRejectedREQStats(ctx context.Context, limit int) ([]RejectedREQStat, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT kind, count, last_seen
		FROM rejected_req_kinds
		ORDER BY count DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []RejectedREQStat
	for rows.Next() {
		var stat RejectedREQStat
		var lastSeen int64
		if err := rows.Scan(&stat.Kind, &stat.Count, &lastSeen); err != nil {
			return nil, err
		}
		stat.LastSeen = time.Unix(lastSeen, 0)
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

// RecordREQKind records a REQ for any kind (for overall stats)
func (s *Storage) RecordREQKind(ctx context.Context, kind int) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	now := time.Now().Unix()
	date := time.Now().Format("2006-01-02")

	// Update total stats
	_, err := dbConn.ExecContext(ctx, `
		INSERT INTO req_kind_stats (kind, total_requests, last_request)
		VALUES (?, 1, ?)
		ON CONFLICT(kind) DO UPDATE SET
			total_requests = total_requests + 1,
			last_request = excluded.last_request
	`, kind, now)
	if err != nil {
		return err
	}

	// Update daily stats
	_, err = dbConn.ExecContext(ctx, `
		INSERT INTO req_kind_stats_daily (date, kind, request_count)
		VALUES (?, ?, 1)
		ON CONFLICT(date, kind) DO UPDATE SET
			request_count = request_count + 1
	`, date, kind)

	return err
}

type REQKindStat struct {
	Kind          int
	TotalRequests int64
	LastRequest   time.Time
}

// GetREQKindStats returns overall REQ stats by kind
func (s *Storage) GetREQKindStats(ctx context.Context, limit int) ([]REQKindStat, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT kind, total_requests, last_request
		FROM req_kind_stats
		ORDER BY total_requests DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []REQKindStat
	for rows.Next() {
		var stat REQKindStat
		var lastRequest int64
		if err := rows.Scan(&stat.Kind, &stat.TotalRequests, &lastRequest); err != nil {
			return nil, err
		}
		stat.LastRequest = time.Unix(lastRequest, 0)
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

type REQKindDailyStat struct {
	Date         string
	Kind         int
	RequestCount int64
}

// GetREQKindDailyStats returns REQ stats by kind per day
func (s *Storage) GetREQKindDailyStats(ctx context.Context, days int, kinds []int) ([]REQKindDailyStat, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	startDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	var rows interface {
		Next() bool
		Scan(...interface{}) error
		Close() error
		Err() error
	}
	var err error

	if len(kinds) > 0 {
		query := `
			SELECT date, kind, request_count
			FROM req_kind_stats_daily
			WHERE date >= ?
			ORDER BY date DESC, request_count DESC
		`
		rows, err = dbConn.QueryContext(ctx, query, startDate)
	} else {
		rows, err = dbConn.QueryContext(ctx, `
			SELECT date, kind, request_count
			FROM req_kind_stats_daily
			WHERE date >= ?
			ORDER BY date DESC, request_count DESC
		`, startDate)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []REQKindDailyStat
	for rows.Next() {
		var stat REQKindDailyStat
		if err := rows.Scan(&stat.Date, &stat.Kind, &stat.RequestCount); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

// GetRejectedEventTotals returns total counts for rejected events
func (s *Storage) GetRejectedEventTotals(ctx context.Context) (totalCount int64, uniqueKinds int64, uniquePubkeys int64, err error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, 0, 0, nil
	}

	err = dbConn.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(count), 0), COUNT(DISTINCT kind), COUNT(DISTINCT pubkey)
		FROM rejected_events_by_kind
	`).Scan(&totalCount, &uniqueKinds, &uniquePubkeys)

	return
}

// GetRejectedREQTotals returns total counts for rejected REQs
func (s *Storage) GetRejectedREQTotals(ctx context.Context) (totalCount int64, uniqueKinds int64, err error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, 0, nil
	}

	err = dbConn.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(count), 0), COUNT(DISTINCT kind)
		FROM rejected_req_kinds
	`).Scan(&totalCount, &uniqueKinds)

	return
}

// SetTrustedPubkeys replaces the trusted pubkeys set
func (s *Storage) SetTrustedPubkeys(ctx context.Context, pubkeys []string) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	tx, err := dbConn.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear existing trusted pubkeys
	if _, err := tx.ExecContext(ctx, `DELETE FROM trusted_pubkeys`); err != nil {
		return err
	}

	// Insert new trusted pubkeys
	now := time.Now().Unix()
	for _, pubkey := range pubkeys {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO trusted_pubkeys (pubkey, trusted_at) VALUES (?, ?)
		`, pubkey, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// IsPubkeyTrusted checks if a pubkey is in the trusted set
func (s *Storage) IsPubkeyTrusted(ctx context.Context, pubkey string) bool {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return false
	}

	var count int
	err := dbConn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM trusted_pubkeys WHERE pubkey = ?
	`, pubkey).Scan(&count)

	return err == nil && count > 0
}

// GetTrustedPubkeys returns all trusted pubkeys from the database
func (s *Storage) GetTrustedPubkeys(ctx context.Context) ([]string, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `SELECT pubkey FROM trusted_pubkeys`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pubkeys []string
	for rows.Next() {
		var pubkey string
		if err := rows.Scan(&pubkey); err != nil {
			return nil, err
		}
		pubkeys = append(pubkeys, pubkey)
	}

	return pubkeys, rows.Err()
}

// GetFollowersOfPubkey returns all pubkeys that follow the given pubkey (from their kind:3 events)
func (s *Storage) GetFollowersOfPubkey(ctx context.Context, pubkey string) ([]string, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	// Find the latest kind:3 event per author that has a "p" tag for this pubkey
	query := `
		WITH latest_contact_lists AS (
			SELECT e1.pubkey as follower, e1.tags
			FROM event e1
			INNER JOIN (
				SELECT pubkey, MAX(created_at) as max_created_at
				FROM event
				WHERE kind = 3
				GROUP BY pubkey
			) e2 ON e1.pubkey = e2.pubkey AND e1.created_at = e2.max_created_at
			WHERE e1.kind = 3
		)
		SELECT DISTINCT follower
		FROM latest_contact_lists, json_each(latest_contact_lists.tags) as tag
		WHERE json_extract(tag.value, '$[0]') = 'p'
		  AND json_extract(tag.value, '$[1]') = ?
	`

	rows, err := dbConn.QueryContext(ctx, query, pubkey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var followers []string
	for rows.Next() {
		var follower string
		if err := rows.Scan(&follower); err != nil {
			return nil, err
		}
		followers = append(followers, follower)
	}

	return followers, rows.Err()
}

// Community types for storage
type StoredCommunity struct {
	ID            int
	Size          int
	InternalEdges int
	ExternalEdges int
	Modularity    float64
	TopMembers    []StoredCommunityMember
	DetectedAt    time.Time
}

type StoredCommunityMember struct {
	Pubkey        string `json:"pubkey"`
	Name          string `json:"name"`
	FollowerCount int    `json:"follower_count"`
}

type StoredCommunityEdge struct {
	FromID int
	ToID   int
	Weight int
}

type StoredCommunityGraph struct {
	Communities    []StoredCommunity
	Edges          []StoredCommunityEdge
	TotalNodes     int
	TotalEdges     int
	NumCommunities int
	DetectedAt     time.Time
}

// CommunityGraph interface for saving (matches analytics.CommunityGraph)
type CommunityGraphData interface {
	GetCommunities() []CommunityData
	GetEdges() []EdgeData
	GetTotalNodes() int
	GetTotalEdges() int
}

type CommunityData interface {
	GetID() int
	GetSize() int
	GetMembers() []string
	GetTopMembers() []MemberData
	GetInternalEdges() int
	GetExternalEdges() int
	GetModularity() float64
}

type EdgeData interface {
	GetFromID() int
	GetToID() int
	GetWeight() int
}

type MemberData interface {
	GetPubkey() string
	GetName() string
	GetFollowerCount() int
}

// SaveCommunities saves community detection results
func (s *Storage) SaveCommunities(ctx context.Context, graph interface{}) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	// Type assert to get the data
	type communityGraph struct {
		Communities []struct {
			ID            int
			Members       []string
			Size          int
			TopMembers    []struct {
				Pubkey        string
				Name          string
				FollowerCount int
			}
			InternalEdges int
			ExternalEdges int
			Modularity    float64
		}
		Edges []struct {
			FromID int
			ToID   int
			Weight int
		}
		TotalNodes int
		TotalEdges int
	}

	// Use reflection-free approach with JSON marshaling
	jsonData, err := json.Marshal(graph)
	if err != nil {
		return err
	}

	var cg communityGraph
	if err := json.Unmarshal(jsonData, &cg); err != nil {
		return err
	}

	tx, err := dbConn.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	// Clear old data
	tx.ExecContext(ctx, `DELETE FROM community_members`)
	tx.ExecContext(ctx, `DELETE FROM community_edges`)
	tx.ExecContext(ctx, `DELETE FROM communities`)

	// Insert communities
	for _, com := range cg.Communities {
		topMembersJSON, _ := json.Marshal(com.TopMembers)

		_, err := tx.ExecContext(ctx, `
			INSERT INTO communities (id, size, internal_edges, external_edges, modularity, top_members, detected_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, com.ID, com.Size, com.InternalEdges, com.ExternalEdges, com.Modularity, string(topMembersJSON), now)
		if err != nil {
			return err
		}

		// Insert members
		for _, member := range com.Members {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO community_members (community_id, pubkey) VALUES (?, ?)
			`, com.ID, member)
			if err != nil {
				return err
			}
		}
	}

	// Insert edges
	for _, edge := range cg.Edges {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO community_edges (from_id, to_id, weight) VALUES (?, ?, ?)
		`, edge.FromID, edge.ToID, edge.Weight)
		if err != nil {
			return err
		}
	}

	// Update stats
	_, err = tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO community_stats (id, total_nodes, total_edges, num_communities, detected_at)
		VALUES (1, ?, ?, ?, ?)
	`, cg.TotalNodes, cg.TotalEdges, len(cg.Communities), now)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetCommunityGraph returns the stored community graph
func (s *Storage) GetCommunityGraph(ctx context.Context) (*StoredCommunityGraph, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	result := &StoredCommunityGraph{}

	// Get stats
	var detectedAt int64
	err := dbConn.QueryRowContext(ctx, `
		SELECT total_nodes, total_edges, num_communities, detected_at
		FROM community_stats WHERE id = 1
	`).Scan(&result.TotalNodes, &result.TotalEdges, &result.NumCommunities, &detectedAt)
	if err != nil {
		return nil, nil // No data yet
	}
	result.DetectedAt = time.Unix(detectedAt, 0)

	// Get communities
	rows, err := dbConn.QueryContext(ctx, `
		SELECT id, size, internal_edges, external_edges, modularity, top_members, detected_at
		FROM communities ORDER BY size DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var com StoredCommunity
		var topMembersJSON string
		var detAt int64
		if err := rows.Scan(&com.ID, &com.Size, &com.InternalEdges, &com.ExternalEdges, &com.Modularity, &topMembersJSON, &detAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(topMembersJSON), &com.TopMembers)
		com.DetectedAt = time.Unix(detAt, 0)
		result.Communities = append(result.Communities, com)
	}

	// Get edges
	edgeRows, err := dbConn.QueryContext(ctx, `
		SELECT from_id, to_id, weight FROM community_edges ORDER BY weight DESC
	`)
	if err != nil {
		return nil, err
	}
	defer edgeRows.Close()

	for edgeRows.Next() {
		var edge StoredCommunityEdge
		if err := edgeRows.Scan(&edge.FromID, &edge.ToID, &edge.Weight); err != nil {
			return nil, err
		}
		result.Edges = append(result.Edges, edge)
	}

	return result, nil
}

// GetCommunityMembers returns all members of a community
func (s *Storage) GetCommunityMembers(ctx context.Context, communityID int, limit int) ([]string, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT pubkey FROM community_members WHERE community_id = ? LIMIT ?
	`, communityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []string
	for rows.Next() {
		var pubkey string
		if err := rows.Scan(&pubkey); err != nil {
			return nil, err
		}
		members = append(members, pubkey)
	}

	return members, rows.Err()
}
