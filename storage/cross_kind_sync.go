package storage

import (
	"context"
	"fmt"
	"strings"
)

// GetPubkeysMissingKind returns pubkeys that have sourceKind but NOT targetKind
// Limited to `limit` results for batched processing
func (s *Storage) GetPubkeysMissingKind(ctx context.Context, sourceKind, targetKind int, limit int) ([]string, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	query := `
		SELECT DISTINCT e1.pubkey
		FROM event e1
		WHERE e1.kind = $1
		AND NOT EXISTS (
			SELECT 1 FROM event e2
			WHERE e2.pubkey = e1.pubkey
			AND e2.kind = $2
		)
		LIMIT $3
	`

	rows, err := dbConn.QueryContext(ctx, query, sourceKind, targetKind, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query missing kinds: %w", err)
	}
	defer rows.Close()

	var pubkeys []string
	for rows.Next() {
		var pk string
		if err := rows.Scan(&pk); err != nil {
			continue
		}
		pubkeys = append(pubkeys, pk)
	}
	return pubkeys, rows.Err()
}

// CheckPubkeyHasKinds checks which of the specified kinds each pubkey has
// Returns map[pubkey]map[kind]bool
func (s *Storage) CheckPubkeyHasKinds(ctx context.Context, pubkeys []string, kinds []int) (map[string]map[int]bool, error) {
	result := make(map[string]map[int]bool)
	for _, pk := range pubkeys {
		result[pk] = make(map[int]bool)
	}

	if len(pubkeys) == 0 || len(kinds) == 0 {
		return result, nil
	}

	dbConn := s.getDBConn()
	if dbConn == nil {
		return result, nil
	}

	// Build pubkey placeholders
	pubkeyPlaceholders := make([]string, len(pubkeys))
	args := make([]interface{}, 0, len(pubkeys)+len(kinds))
	for i, pk := range pubkeys {
		pubkeyPlaceholders[i] = fmt.Sprintf("$%d", i+1)
		args = append(args, pk)
	}

	// Build kind placeholders
	kindPlaceholders := make([]string, len(kinds))
	for i, k := range kinds {
		kindPlaceholders[i] = fmt.Sprintf("$%d", len(pubkeys)+i+1)
		args = append(args, k)
	}

	query := fmt.Sprintf(`
		SELECT DISTINCT pubkey, kind
		FROM event
		WHERE pubkey IN (%s) AND kind IN (%s)
	`, strings.Join(pubkeyPlaceholders, ","), strings.Join(kindPlaceholders, ","))

	rows, err := dbConn.QueryContext(ctx, query, args...)
	if err != nil {
		return result, fmt.Errorf("failed to check pubkey kinds: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var pubkey string
		var kind int
		if err := rows.Scan(&pubkey, &kind); err != nil {
			continue
		}
		if result[pubkey] == nil {
			result[pubkey] = make(map[int]bool)
		}
		result[pubkey][kind] = true
	}

	return result, rows.Err()
}
