package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nbd-wtf/go-nostr"
	"github.com/pablof7z/purplepag.es/internal/relayutil"
)

const (
	defaultMutedLimit    = 20
	defaultInterestLimit = 20
	defaultTrendLimit    = 10
	trendWindowDays      = 30
)

// RefreshDerivedStats recomputes cached analytics data from LMDB and stores it in SQLite.
func (s *Storage) RefreshDerivedStats(ctx context.Context) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	now := time.Now().Unix()

	// Social list counts (full event counts).
	muteCount, err := s.CountEvents(ctx, nostr.Filter{Kinds: []int{10000}})
	if err != nil {
		return fmt.Errorf("count mute lists: %w", err)
	}
	interestCount, err := s.CountEvents(ctx, nostr.Filter{Kinds: []int{10015}})
	if err != nil {
		return fmt.Errorf("count interest lists: %w", err)
	}
	communityCount, err := s.CountEvents(ctx, nostr.Filter{Kinds: []int{10004}})
	if err != nil {
		return fmt.Errorf("count community lists: %w", err)
	}
	contactCount, err := s.CountEvents(ctx, nostr.Filter{Kinds: []int{3}})
	if err != nil {
		return fmt.Errorf("count contact lists: %w", err)
	}

	// Latest contact lists for follower counts.
	contactLists, err := s.latestEventsByPubkey(ctx, 3)
	if err != nil {
		return fmt.Errorf("scan contact lists: %w", err)
	}
	followSets := buildFollowSets(contactLists)
	followerCounts := buildFollowerCounts(followSets)

	// Latest mute lists for most-muted.
	muteLists, err := s.latestEventsByPubkey(ctx, 10000)
	if err != nil {
		return fmt.Errorf("scan mute lists: %w", err)
	}
	mostMuted := buildMostMuted(muteLists, followerCounts, defaultMutedLimit)

	// Latest interest lists.
	interestLists, err := s.latestEventsByPubkey(ctx, 10015)
	if err != nil {
		return fmt.Errorf("scan interest lists: %w", err)
	}
	topInterests := buildTopInterests(interestLists, defaultInterestLimit)

	// Latest relay lists for discovered relays.
	relayLists, err := s.latestEventsByPubkey(ctx, 10002)
	if err != nil {
		return fmt.Errorf("scan relay lists: %w", err)
	}
	relayStats := buildRelayStats(relayLists)

	tx, err := dbConn.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rising, falling, err := s.refreshFollowerTrends(ctx, tx, followSets, time.Unix(now, 0), defaultTrendLimit)
	if err != nil {
		return err
	}

	// Social counts.
	if _, err := tx.ExecContext(ctx, s.rebind(`
		INSERT INTO cached_social_counts (
			id, updated_at, mute_list_count, interest_list_count, community_list_count, contact_list_count
		) VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			updated_at = excluded.updated_at,
			mute_list_count = excluded.mute_list_count,
			interest_list_count = excluded.interest_list_count,
			community_list_count = excluded.community_list_count,
			contact_list_count = excluded.contact_list_count
	`), now, muteCount, interestCount, communityCount, contactCount); err != nil {
		return err
	}

	// Follower counts.
	if _, err := tx.ExecContext(ctx, `DELETE FROM follower_counts`); err != nil {
		return err
	}
	stmtFollower, err := tx.PreparexContext(ctx, s.rebind(`
		INSERT INTO follower_counts (pubkey, follower_count, updated_at)
		VALUES (?, ?, ?)
	`))
	if err != nil {
		return err
	}
	for pubkey, count := range followerCounts {
		if _, err := stmtFollower.ExecContext(ctx, pubkey, count, now); err != nil {
			stmtFollower.Close()
			return err
		}
	}
	if err := stmtFollower.Close(); err != nil {
		return err
	}

	// Most muted.
	if _, err := tx.ExecContext(ctx, `DELETE FROM cached_most_muted`); err != nil {
		return err
	}
	stmtMuted, err := tx.PreparexContext(ctx, s.rebind(`
		INSERT INTO cached_most_muted (rank, pubkey, mute_count, follower_count)
		VALUES (?, ?, ?, ?)
	`))
	if err != nil {
		return err
	}
	for i, item := range mostMuted {
		if _, err := stmtMuted.ExecContext(ctx, i+1, item.Pubkey, item.MuteCount, item.FollowerCount); err != nil {
			stmtMuted.Close()
			return err
		}
	}
	if err := stmtMuted.Close(); err != nil {
		return err
	}

	// Top interests.
	if _, err := tx.ExecContext(ctx, `DELETE FROM cached_top_interests`); err != nil {
		return err
	}
	stmtInterest, err := tx.PreparexContext(ctx, s.rebind(`
		INSERT INTO cached_top_interests (rank, interest, interest_count)
		VALUES (?, ?, ?)
	`))
	if err != nil {
		return err
	}
	for i, item := range topInterests {
		if _, err := stmtInterest.ExecContext(ctx, i+1, item.Interest, item.Count); err != nil {
			stmtInterest.Close()
			return err
		}
	}
	if err := stmtInterest.Close(); err != nil {
		return err
	}

	// Follower trends.
	if _, err := tx.ExecContext(ctx, `DELETE FROM cached_follower_trends`); err != nil {
		return err
	}
	stmtTrend, err := tx.PreparexContext(ctx, s.rebind(`
		INSERT INTO cached_follower_trends (direction, rank, pubkey, net_change, gained, lost)
		VALUES (?, ?, ?, ?, ?, ?)
	`))
	if err != nil {
		return err
	}
	for i, item := range rising {
		if _, err := stmtTrend.ExecContext(ctx, "rising", i+1, item.Pubkey, item.NetChange, item.Gained, item.Lost); err != nil {
			stmtTrend.Close()
			return err
		}
	}
	for i, item := range falling {
		if _, err := stmtTrend.ExecContext(ctx, "falling", i+1, item.Pubkey, item.NetChange, item.Gained, item.Lost); err != nil {
			stmtTrend.Close()
			return err
		}
	}
	if err := stmtTrend.Close(); err != nil {
		return err
	}

	// Relay stats.
	if _, err := tx.ExecContext(ctx, `DELETE FROM cached_relay_stats`); err != nil {
		return err
	}
	stmtRelay, err := tx.PreparexContext(ctx, s.rebind(`
		INSERT INTO cached_relay_stats (rank, url, pubkey_count)
		VALUES (?, ?, ?)
	`))
	if err != nil {
		return err
	}
	for i, item := range relayStats {
		if _, err := stmtRelay.ExecContext(ctx, i+1, item.URL, item.PubkeyCount); err != nil {
			stmtRelay.Close()
			return err
		}
	}
	if err := stmtRelay.Close(); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Cache event counts by kind
	if err := s.refreshEventCountsCache(ctx); err != nil {
		return fmt.Errorf("refresh event counts cache: %w", err)
	}

	return nil
}

func buildFollowSets(latest map[string]*nostr.Event) map[string]map[string]struct{} {
	follows := make(map[string]map[string]struct{}, len(latest))
	for pubkey, evt := range latest {
		seen := make(map[string]struct{})
		for _, tag := range evt.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				followed := tag[1]
				if followed == "" {
					continue
				}
				seen[followed] = struct{}{}
			}
		}
		follows[pubkey] = seen
	}
	return follows
}

func buildFollowerCounts(followSets map[string]map[string]struct{}) map[string]int64 {
	counts := make(map[string]int64)
	for _, set := range followSets {
		for pubkey := range set {
			counts[pubkey]++
		}
	}
	return counts
}

func buildMostMuted(latest map[string]*nostr.Event, followerCounts map[string]int64, limit int) []MutedPubkey {
	muteCounts := make(map[string]int64)
	for _, evt := range latest {
		seen := make(map[string]struct{})
		for _, tag := range evt.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				pubkey := tag[1]
				if pubkey == "" {
					continue
				}
				if _, ok := seen[pubkey]; ok {
					continue
				}
				seen[pubkey] = struct{}{}
				muteCounts[pubkey]++
			}
		}
	}

	results := make([]MutedPubkey, 0, len(muteCounts))
	for pubkey, count := range muteCounts {
		results = append(results, MutedPubkey{
			Pubkey:        pubkey,
			MuteCount:     count,
			FollowerCount: followerCounts[pubkey],
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].MuteCount == results[j].MuteCount {
			return results[i].Pubkey < results[j].Pubkey
		}
		return results[i].MuteCount > results[j].MuteCount
	})

	if limit <= 0 {
		limit = defaultMutedLimit
	}
	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

func buildTopInterests(latest map[string]*nostr.Event, limit int) []InterestRank {
	interestCounts := make(map[string]int64)
	for _, evt := range latest {
		seen := make(map[string]struct{})
		for _, tag := range evt.Tags {
			if len(tag) >= 2 && tag[0] == "t" {
				interest := tag[1]
				if interest == "" {
					continue
				}
				if _, ok := seen[interest]; ok {
					continue
				}
				seen[interest] = struct{}{}
				interestCounts[interest]++
			}
		}
	}

	results := make([]InterestRank, 0, len(interestCounts))
	for interest, count := range interestCounts {
		results = append(results, InterestRank{
			Interest: interest,
			Count:    count,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Count == results[j].Count {
			return results[i].Interest < results[j].Interest
		}
		return results[i].Count > results[j].Count
	})

	if limit <= 0 {
		limit = defaultInterestLimit
	}
	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

func buildRelayStats(latest map[string]*nostr.Event) []DiscoveredRelay {
	relayCounts := make(map[string]int64)
	for _, evt := range latest {
		seen := make(map[string]struct{})
		for _, tag := range evt.Tags {
			if len(tag) < 2 || tag[0] != "r" {
				continue
			}
			normalized, err := relayutil.NormalizeRelayURL(tag[1])
			if err != nil {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			relayCounts[normalized]++
		}
	}

	results := make([]DiscoveredRelay, 0, len(relayCounts))
	for url, count := range relayCounts {
		results = append(results, DiscoveredRelay{
			URL:         url,
			PubkeyCount: count,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].PubkeyCount == results[j].PubkeyCount {
			return results[i].URL < results[j].URL
		}
		return results[i].PubkeyCount > results[j].PubkeyCount
	})

	return results
}

func (s *Storage) refreshFollowerTrends(ctx context.Context, tx *sqlx.Tx, followSets map[string]map[string]struct{}, now time.Time, limit int) (rising []FollowerTrend, falling []FollowerTrend, err error) {
	dayStart := now.UTC().Truncate(24 * time.Hour)
	windowStart := dayStart.Add(-time.Duration(trendWindowDays-1) * 24 * time.Hour).Unix()

	var exists int
	checkErr := tx.QueryRowContext(ctx, `SELECT 1 FROM follower_edges LIMIT 1`).Scan(&exists)
	if checkErr != nil && checkErr != sql.ErrNoRows {
		return nil, nil, checkErr
	}

	if checkErr == sql.ErrNoRows {
		if err := s.insertFollowerEdges(ctx, tx, followSets); err != nil {
			return nil, nil, err
		}
	} else {
		changes, err := s.collectFollowerEdgeChanges(ctx, tx, followSets)
		if err != nil {
			return nil, nil, err
		}
		if err := s.upsertFollowerTrendChanges(ctx, tx, dayStart.Unix(), changes); err != nil {
			return nil, nil, err
		}
	}

	if _, err := tx.ExecContext(ctx, s.rebind(`
		DELETE FROM follower_trend_changes
		WHERE day < ?
	`), windowStart); err != nil {
		return nil, nil, err
	}

	trends, err := s.loadFollowerTrendWindow(ctx, tx, windowStart)
	if err != nil {
		return nil, nil, err
	}

	rising, falling = rankFollowerTrends(trends, limit)
	return rising, falling, nil
}

func (s *Storage) insertFollowerEdges(ctx context.Context, tx *sqlx.Tx, followSets map[string]map[string]struct{}) error {
	stmt, err := tx.PreparexContext(ctx, s.rebind(`
		INSERT OR IGNORE INTO follower_edges (follower, followed)
		VALUES (?, ?)
	`))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for follower, follows := range followSets {
		for followed := range follows {
			if _, err := stmt.ExecContext(ctx, follower, followed); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Storage) collectFollowerEdgeChanges(ctx context.Context, tx *sqlx.Tx, followSets map[string]map[string]struct{}) (map[string]*FollowerTrend, error) {
	changes := make(map[string]*FollowerTrend)

	stmtSelect, err := tx.PreparexContext(ctx, s.rebind(`
		SELECT followed
		FROM follower_edges
		WHERE follower = ?
	`))
	if err != nil {
		return nil, err
	}
	defer stmtSelect.Close()

	stmtInsert, err := tx.PreparexContext(ctx, s.rebind(`
		INSERT OR IGNORE INTO follower_edges (follower, followed)
		VALUES (?, ?)
	`))
	if err != nil {
		return nil, err
	}
	defer stmtInsert.Close()

	stmtDelete, err := tx.PreparexContext(ctx, s.rebind(`
		DELETE FROM follower_edges
		WHERE follower = ? AND followed = ?
	`))
	if err != nil {
		return nil, err
	}
	defer stmtDelete.Close()

	for follower, newSet := range followSets {
		oldSet := make(map[string]struct{})
		rows, err := stmtSelect.QueryxContext(ctx, follower)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var followed string
			if err := rows.Scan(&followed); err != nil {
				rows.Close()
				return nil, err
			}
			oldSet[followed] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()

		for followed := range newSet {
			if _, ok := oldSet[followed]; ok {
				continue
			}
			trend := changes[followed]
			if trend == nil {
				trend = &FollowerTrend{Pubkey: followed}
				changes[followed] = trend
			}
			trend.Gained++
			if _, err := stmtInsert.ExecContext(ctx, follower, followed); err != nil {
				return nil, err
			}
		}

		for followed := range oldSet {
			if _, ok := newSet[followed]; ok {
				continue
			}
			trend := changes[followed]
			if trend == nil {
				trend = &FollowerTrend{Pubkey: followed}
				changes[followed] = trend
			}
			trend.Lost++
			if _, err := stmtDelete.ExecContext(ctx, follower, followed); err != nil {
				return nil, err
			}
		}
	}

	return changes, nil
}

func (s *Storage) upsertFollowerTrendChanges(ctx context.Context, tx *sqlx.Tx, day int64, changes map[string]*FollowerTrend) error {
	if len(changes) == 0 {
		return nil
	}

	stmt, err := tx.PreparexContext(ctx, s.rebind(`
		INSERT INTO follower_trend_changes (day, pubkey, gained, lost)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(day, pubkey) DO UPDATE SET
			gained = gained + excluded.gained,
			lost = lost + excluded.lost
	`))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, trend := range changes {
		if trend.Gained == 0 && trend.Lost == 0 {
			continue
		}
		if _, err := stmt.ExecContext(ctx, day, trend.Pubkey, trend.Gained, trend.Lost); err != nil {
			return err
		}
	}

	return nil
}

func (s *Storage) loadFollowerTrendWindow(ctx context.Context, tx *sqlx.Tx, sinceDay int64) ([]FollowerTrend, error) {
	rows, err := tx.QueryxContext(ctx, s.rebind(`
		SELECT pubkey, SUM(gained) AS gained, SUM(lost) AS lost
		FROM follower_trend_changes
		WHERE day >= ?
		GROUP BY pubkey
	`), sinceDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]FollowerTrend, 0)
	for rows.Next() {
		var trend FollowerTrend
		if err := rows.Scan(&trend.Pubkey, &trend.Gained, &trend.Lost); err != nil {
			return nil, err
		}
		trend.NetChange = trend.Gained - trend.Lost
		results = append(results, trend)
	}

	return results, rows.Err()
}

func rankFollowerTrends(trends []FollowerTrend, limit int) (rising []FollowerTrend, falling []FollowerTrend) {
	if limit <= 0 {
		limit = defaultTrendLimit
	}

	sort.Slice(trends, func(i, j int) bool {
		if trends[i].NetChange == trends[j].NetChange {
			return trends[i].Pubkey < trends[j].Pubkey
		}
		return trends[i].NetChange > trends[j].NetChange
	})
	for _, trend := range trends {
		if trend.NetChange > 0 {
			rising = append(rising, trend)
			if len(rising) >= limit {
				break
			}
		}
	}

	sort.Slice(trends, func(i, j int) bool {
		if trends[i].NetChange == trends[j].NetChange {
			return trends[i].Pubkey < trends[j].Pubkey
		}
		return trends[i].NetChange < trends[j].NetChange
	})
	for _, trend := range trends {
		if trend.NetChange < 0 {
			falling = append(falling, trend)
			if len(falling) >= limit {
				break
			}
		}
	}

	return rising, falling
}

// refreshEventCountsCache scans all events and caches counts by kind
func (s *Storage) refreshEventCountsCache(ctx context.Context) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	// Scan LMDB directly to count events by kind (this is slow but only runs hourly)
	kindCounts := make(map[int]int64)
	if err := s.ScanEvents(ctx, nostr.Filter{}, 0, func(evt *nostr.Event) error {
		kindCounts[evt.Kind]++
		return nil
	}); err != nil {
		return fmt.Errorf("scan events for counts: %w", err)
	}

	now := time.Now().Unix()

	// Begin transaction
	tx, err := dbConn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear old cached counts
	if _, err := tx.ExecContext(ctx, "DELETE FROM cached_event_counts"); err != nil {
		return err
	}

	// Insert new counts
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO cached_event_counts (kind, event_count, updated_at)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	var totalCount int64
	for kind, count := range kindCounts {
		totalCount += count
		if _, err := stmt.ExecContext(ctx, kind, count, now); err != nil {
			return err
		}
	}

	// Also cache the total count for fast access
	if _, err := tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO cached_total_count (id, total_events, updated_at)
		VALUES (1, ?, ?)
	`, totalCount, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}
