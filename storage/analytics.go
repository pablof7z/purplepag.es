package storage

import (
	"context"
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
