package storage

import (
	"context"
	"database/sql"
	"time"
)

type SocialCounts struct {
	UpdatedAt          time.Time
	MuteListCount      int64
	InterestListCount  int64
	CommunityListCount int64
	ContactListCount   int64
}

func (s *Storage) GetCachedSocialCounts(ctx context.Context) (SocialCounts, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return SocialCounts{}, nil
	}

	var counts SocialCounts
	var updatedAt int64
	err := dbConn.QueryRowContext(ctx, `
		SELECT updated_at, mute_list_count, interest_list_count, community_list_count, contact_list_count
		FROM cached_social_counts
		WHERE id = 1
	`).Scan(&updatedAt, &counts.MuteListCount, &counts.InterestListCount, &counts.CommunityListCount, &counts.ContactListCount)
	if err == sql.ErrNoRows {
		return SocialCounts{}, nil
	}
	if err != nil {
		return SocialCounts{}, err
	}
	counts.UpdatedAt = time.Unix(updatedAt, 0)
	return counts, nil
}

func (s *Storage) GetCachedMostMuted(ctx context.Context, limit int) ([]MutedPubkey, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT pubkey, mute_count, follower_count
		FROM cached_most_muted
		ORDER BY rank ASC
		LIMIT ?
	`), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []MutedPubkey
	for rows.Next() {
		var item MutedPubkey
		if err := rows.Scan(&item.Pubkey, &item.MuteCount, &item.FollowerCount); err != nil {
			return nil, err
		}
		results = append(results, item)
	}

	return results, rows.Err()
}

func (s *Storage) GetCachedTopInterests(ctx context.Context, limit int) ([]InterestRank, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT interest, interest_count
		FROM cached_top_interests
		ORDER BY rank ASC
		LIMIT ?
	`), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []InterestRank
	for rows.Next() {
		var item InterestRank
		if err := rows.Scan(&item.Interest, &item.Count); err != nil {
			return nil, err
		}
		results = append(results, item)
	}

	return results, rows.Err()
}

func (s *Storage) GetCachedFollowerTrends(ctx context.Context, limit int) (rising []FollowerTrend, falling []FollowerTrend, err error) {
	rising, err = s.getCachedFollowerTrendsByDirection(ctx, "rising", limit)
	if err != nil {
		return nil, nil, err
	}
	falling, err = s.getCachedFollowerTrendsByDirection(ctx, "falling", limit)
	if err != nil {
		return nil, nil, err
	}
	return rising, falling, nil
}

func (s *Storage) getCachedFollowerTrendsByDirection(ctx context.Context, direction string, limit int) ([]FollowerTrend, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT pubkey, net_change, gained, lost
		FROM cached_follower_trends
		WHERE direction = ?
		ORDER BY rank ASC
		LIMIT ?
	`), direction, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []FollowerTrend
	for rows.Next() {
		var item FollowerTrend
		if err := rows.Scan(&item.Pubkey, &item.NetChange, &item.Gained, &item.Lost); err != nil {
			return nil, err
		}
		results = append(results, item)
	}

	return results, rows.Err()
}

func (s *Storage) GetCachedFollowerCount(ctx context.Context, pubkey string) (int64, bool, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, false, nil
	}

	var count int64
	err := dbConn.QueryRowContext(ctx, s.rebind(`
		SELECT follower_count
		FROM follower_counts
		WHERE pubkey = ?
	`), pubkey).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return count, true, nil
}

func (s *Storage) GetCachedFollowerCountsPage(ctx context.Context, limit, offset int) ([]FollowerCount, int, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 50
	}

	var total int
	if err := dbConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM follower_counts`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT pubkey, follower_count
		FROM follower_counts
		ORDER BY follower_count DESC, pubkey ASC
		LIMIT ? OFFSET ?
	`), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	results := make([]FollowerCount, 0)
	for rows.Next() {
		var item FollowerCount
		if err := rows.Scan(&item.Pubkey, &item.FollowerCount); err != nil {
			return nil, 0, err
		}
		results = append(results, item)
	}

	return results, total, rows.Err()
}

func (s *Storage) GetCachedFollowerCounts(ctx context.Context, minFollowers int) (map[string]int, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT pubkey, follower_count
		FROM follower_counts
		WHERE follower_count >= ?
	`), minFollowers)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[string]int)
	for rows.Next() {
		var pubkey string
		var count int
		if err := rows.Scan(&pubkey, &count); err != nil {
			return nil, err
		}
		results[pubkey] = count
	}

	return results, rows.Err()
}

func (s *Storage) GetCachedRelayStats(ctx context.Context) ([]DiscoveredRelay, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `
		SELECT url, pubkey_count
		FROM cached_relay_stats
		ORDER BY rank ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DiscoveredRelay
	for rows.Next() {
		var item DiscoveredRelay
		if err := rows.Scan(&item.URL, &item.PubkeyCount); err != nil {
			return nil, err
		}
		results = append(results, item)
	}

	return results, rows.Err()
}
