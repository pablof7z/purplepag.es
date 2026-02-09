package storage

import (
	"context"
	"sort"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func (s *Storage) latestEventsByPubkey(ctx context.Context, kind int) (map[string]*nostr.Event, error) {
	latest := make(map[string]*nostr.Event)
	if err := s.ScanEvents(ctx, nostr.Filter{Kinds: []int{kind}}, 0, func(evt *nostr.Event) error {
		if existing, ok := latest[evt.PubKey]; !ok || isNewerEvent(existing, evt) {
			latest[evt.PubKey] = evt
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return latest, nil
}

func isNewerEvent(previous, next *nostr.Event) bool {
	if previous == nil {
		return true
	}
	if next == nil {
		return false
	}
	if previous.CreatedAt < next.CreatedAt {
		return true
	}
	if previous.CreatedAt == next.CreatedAt && previous.ID > next.ID {
		return true
	}
	return false
}

// LatestEventsByPubkey returns the latest event for each pubkey for the given kind.
func (s *Storage) LatestEventsByPubkey(ctx context.Context, kind int) (map[string]*nostr.Event, error) {
	return s.latestEventsByPubkey(ctx, kind)
}

func tagsAsStrings(tags nostr.Tags) [][]string {
	if tags == nil {
		return nil
	}
	results := make([][]string, 0, len(tags))
	for _, tag := range tags {
		results = append(results, tag)
	}
	return results
}

type MutedPubkey struct {
	Pubkey        string
	MuteCount     int64
	FollowerCount int64
}

type InterestRank struct {
	Interest string
	Count    int64
}

type CommunityRank struct {
	Community string
	Count     int64
}

type FollowerCount struct {
	Pubkey        string
	FollowerCount int64
}

type FollowerTrend struct {
	Pubkey    string
	NetChange int64
	Gained    int64
	Lost      int64
}

// GetMostMutedPubkeys returns pubkeys that appear most frequently in kind 10000 mute lists
func (s *Storage) GetMostMutedPubkeys(ctx context.Context, limit int) ([]MutedPubkey, error) {
	if cached, err := s.GetCachedMostMuted(ctx, limit); err == nil {
		return cached, nil
	}

	latest, err := s.latestEventsByPubkey(ctx, 10000)
	if err != nil {
		return nil, err
	}

	// Count mutes per pubkey
	muteCounts := make(map[string]int64)
	for _, evt := range latest {
		for _, tag := range tagsAsStrings(evt.Tags) {
			if len(tag) >= 2 && tag[0] == "p" {
				muteCounts[tag[1]]++
			}
		}
	}

	// Get follower counts for muted pubkeys
	followerCounts, _ := s.getFollowerCountsForPubkeys(ctx, muteCounts)

	// Convert to slice and sort
	results := make([]MutedPubkey, 0, len(muteCounts))
	for pk, count := range muteCounts {
		results = append(results, MutedPubkey{
			Pubkey:        pk,
			MuteCount:     count,
			FollowerCount: followerCounts[pk],
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].MuteCount > results[j].MuteCount
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// getFollowerCountsForPubkeys counts how many kind 3 events have each pubkey in their "p" tags
func (s *Storage) getFollowerCountsForPubkeys(ctx context.Context, pubkeys map[string]int64) (map[string]int64, error) {
	latest, err := s.latestEventsByPubkey(ctx, 3)
	if err != nil {
		return nil, err
	}
	followerCounts := make(map[string]int64)
	for _, evt := range latest {
		for _, tag := range tagsAsStrings(evt.Tags) {
			if len(tag) >= 2 && tag[0] == "p" {
				if _, exists := pubkeys[tag[1]]; exists {
					followerCounts[tag[1]]++
				}
			}
		}
	}

	return followerCounts, nil
}

// GetInterestRankings returns the most common interests from kind 10015 events
func (s *Storage) GetInterestRankings(ctx context.Context, limit int) ([]InterestRank, error) {
	if cached, err := s.GetCachedTopInterests(ctx, limit); err == nil {
		return cached, nil
	}

	latest, err := s.latestEventsByPubkey(ctx, 10015)
	if err != nil {
		return nil, err
	}

	interestCounts := make(map[string]int64)
	for _, evt := range latest {
		for _, tag := range tagsAsStrings(evt.Tags) {
			if len(tag) >= 2 && tag[0] == "t" {
				interestCounts[tag[1]]++
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
		return results[i].Count > results[j].Count
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetCommunityRankings returns the most popular communities from kind 10004 events
func (s *Storage) GetCommunityRankings(ctx context.Context, limit int) ([]CommunityRank, error) {
	latest, err := s.latestEventsByPubkey(ctx, 10004)
	if err != nil {
		return nil, err
	}

	communityCounts := make(map[string]int64)
	for _, evt := range latest {
		for _, tag := range tagsAsStrings(evt.Tags) {
			if len(tag) >= 2 && tag[0] == "a" {
				communityCounts[tag[1]]++
			}
		}
	}

	results := make([]CommunityRank, 0, len(communityCounts))
	for community, count := range communityCounts {
		results = append(results, CommunityRank{
			Community: community,
			Count:     count,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Count > results[j].Count
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetTopFollowed returns pubkeys with the most followers from kind 3 events
func (s *Storage) GetTopFollowed(ctx context.Context, limit int) ([]FollowerCount, error) {
	if cached, _, err := s.GetCachedFollowerCountsPage(ctx, limit, 0); err == nil {
		return cached, nil
	}

	latest, err := s.latestEventsByPubkey(ctx, 3)
	if err != nil {
		return nil, err
	}

	followerCounts := make(map[string]int64)
	for _, evt := range latest {
		for _, tag := range tagsAsStrings(evt.Tags) {
			if len(tag) >= 2 && tag[0] == "p" {
				followerCounts[tag[1]]++
			}
		}
	}

	results := make([]FollowerCount, 0, len(followerCounts))
	for pk, count := range followerCounts {
		results = append(results, FollowerCount{
			Pubkey:        pk,
			FollowerCount: count,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FollowerCount > results[j].FollowerCount
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetFollowerTrends calculates who gained/lost the most followers based on event_history
func (s *Storage) GetFollowerTrends(ctx context.Context, limit int) (rising []FollowerTrend, falling []FollowerTrend, err error) {
	if cachedRising, cachedFalling, cachedErr := s.GetCachedFollowerTrends(ctx, limit); cachedErr == nil {
		return cachedRising, cachedFalling, nil
	}
	return s.getFollowerTrendsFromChangeLog(ctx, limit)
}

func (s *Storage) getFollowerTrendsFromChangeLog(ctx context.Context, limit int) (rising []FollowerTrend, falling []FollowerTrend, err error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil, nil
	}

	dayStart := time.Now().UTC().Truncate(24 * time.Hour)
	windowStart := dayStart.Add(-time.Duration(trendWindowDays-1) * 24 * time.Hour).Unix()

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT pubkey, SUM(gained) AS gained, SUM(lost) AS lost
		FROM follower_trend_changes
		WHERE day >= ?
		GROUP BY pubkey
	`), windowStart)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	trends := make([]FollowerTrend, 0)
	for rows.Next() {
		var trend FollowerTrend
		if err := rows.Scan(&trend.Pubkey, &trend.Gained, &trend.Lost); err != nil {
			return nil, nil, err
		}
		trend.NetChange = trend.Gained - trend.Lost
		trends = append(trends, trend)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	rising, falling = rankFollowerTrends(trends, limit)
	return rising, falling, nil
}

// GetFollowerCount returns the number of followers for a specific pubkey
func (s *Storage) GetFollowerCount(ctx context.Context, pubkey string) (int64, error) {
	count, ok, err := s.GetCachedFollowerCount(ctx, pubkey)
	if err != nil {
		return 0, err
	}
	if ok {
		return count, nil
	}
	return 0, nil
}

// GetSocialGraphStats returns summary statistics
func (s *Storage) GetSocialGraphStats(ctx context.Context) (muteListCount, interestListCount, communityListCount, contactListCount int64, err error) {
	if cached, cacheErr := s.GetCachedSocialCounts(ctx); cacheErr == nil {
		return cached.MuteListCount, cached.InterestListCount, cached.CommunityListCount, cached.ContactListCount, nil
	}

	return 0, 0, 0, 0, nil
}
