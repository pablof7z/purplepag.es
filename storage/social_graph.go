package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

type MutedPubkey struct {
	Pubkey       string
	MuteCount    int64
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
	Pubkey      string
	NetChange   int64
	Gained      int64
	Lost        int64
}

// GetMostMutedPubkeys returns pubkeys that appear most frequently in kind 10000 mute lists
func (s *Storage) GetMostMutedPubkeys(ctx context.Context, limit int) ([]MutedPubkey, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	// Get all mute list events (kind 10000)
	rows, err := dbConn.QueryContext(ctx, `SELECT tags FROM event WHERE kind = 10000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Count mutes per pubkey
	muteCounts := make(map[string]int64)
	for rows.Next() {
		var tagsJSON string
		if err := rows.Scan(&tagsJSON); err != nil {
			continue
		}
		var tags [][]string
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			continue
		}
		for _, tag := range tags {
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
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	// Get all contact lists (kind 3)
	rows, err := dbConn.QueryContext(ctx, `SELECT tags FROM event WHERE kind = 3`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	followerCounts := make(map[string]int64)
	for rows.Next() {
		var tagsJSON string
		if err := rows.Scan(&tagsJSON); err != nil {
			continue
		}
		var tags [][]string
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			continue
		}
		for _, tag := range tags {
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
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `SELECT tags FROM event WHERE kind = 10015`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	interestCounts := make(map[string]int64)
	for rows.Next() {
		var tagsJSON string
		if err := rows.Scan(&tagsJSON); err != nil {
			continue
		}
		var tags [][]string
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			continue
		}
		for _, tag := range tags {
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
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `SELECT tags FROM event WHERE kind = 10004`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	communityCounts := make(map[string]int64)
	for rows.Next() {
		var tagsJSON string
		if err := rows.Scan(&tagsJSON); err != nil {
			continue
		}
		var tags [][]string
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			continue
		}
		for _, tag := range tags {
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
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `SELECT tags FROM event WHERE kind = 3`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	followerCounts := make(map[string]int64)
	for rows.Next() {
		var tagsJSON string
		if err := rows.Scan(&tagsJSON); err != nil {
			continue
		}
		var tags [][]string
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			continue
		}
		for _, tag := range tags {
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
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil, nil
	}

	// Get historical kind 3 events
	rows, err := dbConn.QueryContext(ctx, `
		SELECT pubkey, tags
		FROM event_history
		WHERE kind = 3
	`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	// Track old follows per pubkey (aggregated)
	oldFollows := make(map[string]map[string]bool)
	for rows.Next() {
		var pubkey, tagsJSON string
		if err := rows.Scan(&pubkey, &tagsJSON); err != nil {
			continue
		}
		var tags [][]string
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			continue
		}

		if oldFollows[pubkey] == nil {
			oldFollows[pubkey] = make(map[string]bool)
		}
		for _, tag := range tags {
			if len(tag) >= 2 && tag[0] == "p" {
				oldFollows[pubkey][tag[1]] = true
			}
		}
	}

	// Get current kind 3 events
	currentRows, err := dbConn.QueryContext(ctx, `SELECT pubkey, tags FROM event WHERE kind = 3`)
	if err != nil {
		return nil, nil, err
	}
	defer currentRows.Close()

	currentFollows := make(map[string]map[string]bool)
	for currentRows.Next() {
		var pubkey, tagsJSON string
		if err := currentRows.Scan(&pubkey, &tagsJSON); err != nil {
			continue
		}
		var tags [][]string
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			continue
		}

		currentFollows[pubkey] = make(map[string]bool)
		for _, tag := range tags {
			if len(tag) >= 2 && tag[0] == "p" {
				currentFollows[pubkey][tag[1]] = true
			}
		}
	}

	// Calculate changes per followed pubkey
	changes := make(map[string]*FollowerTrend)

	// Process users who have history
	for pubkey, oldSet := range oldFollows {
		currentSet := currentFollows[pubkey]
		if currentSet == nil {
			currentSet = make(map[string]bool)
		}

		// Find gained (in current but not in old)
		for followed := range currentSet {
			if !oldSet[followed] {
				if changes[followed] == nil {
					changes[followed] = &FollowerTrend{Pubkey: followed}
				}
				changes[followed].Gained++
			}
		}

		// Find lost (in old but not in current)
		for followed := range oldSet {
			if !currentSet[followed] {
				if changes[followed] == nil {
					changes[followed] = &FollowerTrend{Pubkey: followed}
				}
				changes[followed].Lost++
			}
		}
	}

	// Calculate net change
	for _, trend := range changes {
		trend.NetChange = int64(trend.Gained) - int64(trend.Lost)
	}

	// Convert to slices
	allTrends := make([]FollowerTrend, 0, len(changes))
	for _, trend := range changes {
		allTrends = append(allTrends, *trend)
	}

	// Sort for rising (highest net gain)
	sort.Slice(allTrends, func(i, j int) bool {
		return allTrends[i].NetChange > allTrends[j].NetChange
	})

	rising = make([]FollowerTrend, 0, limit)
	for i := 0; i < len(allTrends) && i < limit; i++ {
		if allTrends[i].NetChange > 0 {
			rising = append(rising, allTrends[i])
		}
	}

	// Sort for falling (lowest net gain / highest loss)
	sort.Slice(allTrends, func(i, j int) bool {
		return allTrends[i].NetChange < allTrends[j].NetChange
	})

	falling = make([]FollowerTrend, 0, limit)
	for i := 0; i < len(allTrends) && i < limit; i++ {
		if allTrends[i].NetChange < 0 {
			falling = append(falling, allTrends[i])
		}
	}

	return rising, falling, nil
}

// GetFollowerCount returns the number of followers for a specific pubkey
func (s *Storage) GetFollowerCount(ctx context.Context, pubkey string) (int64, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, nil
	}

	var count int64

	// Use JSONB containment operator for fast counting
	err := dbConn.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM event
		WHERE kind = 3
		AND tags @> $1::jsonb
	`, fmt.Sprintf(`[["p","%s"]]`, pubkey)).Scan(&count)

	return count, err
}

// GetSocialGraphStats returns summary statistics
func (s *Storage) GetSocialGraphStats(ctx context.Context) (muteListCount, interestListCount, communityListCount, contactListCount int64, err error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, 0, 0, 0, nil
	}

	err = dbConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM event WHERE kind = 10000`).Scan(&muteListCount)
	if err != nil {
		return
	}
	err = dbConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM event WHERE kind = 10015`).Scan(&interestListCount)
	if err != nil {
		return
	}
	err = dbConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM event WHERE kind = 10004`).Scan(&communityListCount)
	if err != nil {
		return
	}
	err = dbConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM event WHERE kind = 3`).Scan(&contactListCount)
	return
}
