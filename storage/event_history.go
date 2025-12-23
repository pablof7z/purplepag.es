package storage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// EventVersion represents a historical version of an event
type EventVersion struct {
	ID        string
	PubKey    string
	Kind      int
	CreatedAt nostr.Timestamp
	Content   string
	Tags      nostr.Tags
	ArchivedAt time.Time
}

// ProfileDelta represents changes between profile versions
type ProfileDelta struct {
	PubKey      string
	OldVersion  *EventVersion
	NewVersion  *EventVersion
	Timestamp   time.Time
	Changes     []ProfileChange
}

type ProfileChange struct {
	Field    string
	OldValue string
	NewValue string
}

// ContactsDelta represents changes between contact list versions
type ContactsDelta struct {
	PubKey     string
	OldVersion *EventVersion
	NewVersion *EventVersion
	Timestamp  time.Time
	Added      []string // pubkeys added
	Removed    []string // pubkeys removed
}

// RelaysDelta represents changes between relay list versions
type RelaysDelta struct {
	PubKey     string
	OldVersion *EventVersion
	NewVersion *EventVersion
	Timestamp  time.Time
	Added      []string // relays added
	Removed    []string // relays removed
}

func (s *Storage) InitEventHistorySchema() error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	schema := `
	CREATE TABLE IF NOT EXISTS event_history (
		id TEXT NOT NULL,
		pubkey TEXT NOT NULL,
		kind INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		content TEXT NOT NULL,
		tags TEXT NOT NULL,
		sig TEXT NOT NULL,
		archived_at INTEGER NOT NULL,
		PRIMARY KEY (id)
	);

	CREATE INDEX IF NOT EXISTS idx_event_history_pubkey_kind ON event_history(pubkey, kind);
	CREATE INDEX IF NOT EXISTS idx_event_history_kind_created ON event_history(kind, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_event_history_archived ON event_history(archived_at DESC);
	`

	_, err := dbConn.Exec(schema)
	return err
}

// ArchiveEvent saves an event to history before it gets replaced
func (s *Storage) ArchiveEvent(ctx context.Context, evt *nostr.Event) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	tagsJSON, err := json.Marshal(evt.Tags)
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	_, err = dbConn.ExecContext(ctx, s.rebind(`
		INSERT INTO event_history (id, pubkey, kind, created_at, content, tags, sig, archived_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`), evt.ID, evt.PubKey, evt.Kind, evt.CreatedAt, evt.Content, string(tagsJSON), evt.Sig, now)

	return err
}

// GetEventHistory returns all historical versions of events for a pubkey and kind
func (s *Storage) GetEventHistory(ctx context.Context, pubkey string, kind int, limit int) ([]EventVersion, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT id, pubkey, kind, created_at, content, tags, archived_at
		FROM event_history
		WHERE pubkey = ? AND kind = ?
		ORDER BY created_at DESC
		LIMIT ?
	`), pubkey, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []EventVersion
	for rows.Next() {
		var v EventVersion
		var tagsJSON string
		var archivedAt int64
		if err := rows.Scan(&v.ID, &v.PubKey, &v.Kind, &v.CreatedAt, &v.Content, &tagsJSON, &archivedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(tagsJSON), &v.Tags)
		v.ArchivedAt = time.Unix(archivedAt, 0)
		versions = append(versions, v)
	}

	return versions, rows.Err()
}

// GetAllEventHistory returns all historical events for a pubkey (all kinds)
func (s *Storage) GetAllEventHistory(ctx context.Context, pubkey string, limit int) ([]EventVersion, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT id, pubkey, kind, created_at, content, tags, archived_at
		FROM event_history
		WHERE pubkey = ?
		ORDER BY created_at DESC
		LIMIT ?
	`), pubkey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []EventVersion
	for rows.Next() {
		var v EventVersion
		var tagsJSON string
		var archivedAt int64
		if err := rows.Scan(&v.ID, &v.PubKey, &v.Kind, &v.CreatedAt, &v.Content, &tagsJSON, &archivedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(tagsJSON), &v.Tags)
		v.ArchivedAt = time.Unix(archivedAt, 0)
		versions = append(versions, v)
	}

	return versions, rows.Err()
}

// GetRecentChanges returns recent archived events across all pubkeys
func (s *Storage) GetRecentChanges(ctx context.Context, kind int, limit int) ([]EventVersion, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	var rows interface {
		Next() bool
		Scan(...interface{}) error
		Close() error
		Err() error
	}
	var err error

	if kind > 0 {
		rows, err = dbConn.QueryContext(ctx, s.rebind(`
			SELECT id, pubkey, kind, created_at, content, tags, archived_at
			FROM event_history
			WHERE kind = ?
			ORDER BY archived_at DESC
			LIMIT ?
		`), kind, limit)
	} else {
		rows, err = dbConn.QueryContext(ctx, s.rebind(`
			SELECT id, pubkey, kind, created_at, content, tags, archived_at
			FROM event_history
			ORDER BY archived_at DESC
			LIMIT ?
		`), limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []EventVersion
	for rows.Next() {
		var v EventVersion
		var tagsJSON string
		var archivedAt int64
		if err := rows.Scan(&v.ID, &v.PubKey, &v.Kind, &v.CreatedAt, &v.Content, &tagsJSON, &archivedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(tagsJSON), &v.Tags)
		v.ArchivedAt = time.Unix(archivedAt, 0)
		versions = append(versions, v)
	}

	return versions, rows.Err()
}

// GetEventHistoryStats returns stats about archived events
func (s *Storage) GetEventHistoryStats(ctx context.Context) (totalVersions int64, uniquePubkeys int64, err error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, 0, nil
	}

	err = dbConn.QueryRowContext(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT pubkey)
		FROM event_history
	`).Scan(&totalVersions, &uniquePubkeys)

	return
}

// CalculateProfileDelta computes differences between two profile versions
func CalculateProfileDelta(oldVer, newVer *EventVersion) *ProfileDelta {
	delta := &ProfileDelta{
		PubKey:     newVer.PubKey,
		OldVersion: oldVer,
		NewVersion: newVer,
		Timestamp:  time.Unix(int64(newVer.CreatedAt), 0),
		Changes:    []ProfileChange{},
	}

	var oldProfile, newProfile struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		About       string `json:"about"`
		Picture     string `json:"picture"`
		Banner      string `json:"banner"`
		Nip05       string `json:"nip05"`
		Lud16       string `json:"lud16"`
		Website     string `json:"website"`
	}

	if oldVer != nil {
		json.Unmarshal([]byte(oldVer.Content), &oldProfile)
	}
	json.Unmarshal([]byte(newVer.Content), &newProfile)

	// Check each field for changes
	checkField := func(field, oldVal, newVal string) {
		if oldVal != newVal {
			delta.Changes = append(delta.Changes, ProfileChange{
				Field:    field,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}

	checkField("name", oldProfile.Name, newProfile.Name)
	checkField("display_name", oldProfile.DisplayName, newProfile.DisplayName)
	checkField("about", oldProfile.About, newProfile.About)
	checkField("picture", oldProfile.Picture, newProfile.Picture)
	checkField("banner", oldProfile.Banner, newProfile.Banner)
	checkField("nip05", oldProfile.Nip05, newProfile.Nip05)
	checkField("lud16", oldProfile.Lud16, newProfile.Lud16)
	checkField("website", oldProfile.Website, newProfile.Website)

	return delta
}

// CalculateContactsDelta computes follow/unfollow changes
func CalculateContactsDelta(oldVer, newVer *EventVersion) *ContactsDelta {
	delta := &ContactsDelta{
		PubKey:     newVer.PubKey,
		OldVersion: oldVer,
		NewVersion: newVer,
		Timestamp:  time.Unix(int64(newVer.CreatedAt), 0),
		Added:      []string{},
		Removed:    []string{},
	}

	oldFollows := make(map[string]bool)
	newFollows := make(map[string]bool)

	if oldVer != nil {
		for _, tag := range oldVer.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				oldFollows[tag[1]] = true
			}
		}
	}

	for _, tag := range newVer.Tags {
		if len(tag) >= 2 && tag[0] == "p" {
			newFollows[tag[1]] = true
		}
	}

	// Find added
	for pk := range newFollows {
		if !oldFollows[pk] {
			delta.Added = append(delta.Added, pk)
		}
	}

	// Find removed
	for pk := range oldFollows {
		if !newFollows[pk] {
			delta.Removed = append(delta.Removed, pk)
		}
	}

	return delta
}

// CalculateRelaysDelta computes relay list changes
func CalculateRelaysDelta(oldVer, newVer *EventVersion) *RelaysDelta {
	delta := &RelaysDelta{
		PubKey:     newVer.PubKey,
		OldVersion: oldVer,
		NewVersion: newVer,
		Timestamp:  time.Unix(int64(newVer.CreatedAt), 0),
		Added:      []string{},
		Removed:    []string{},
	}

	oldRelays := make(map[string]bool)
	newRelays := make(map[string]bool)

	if oldVer != nil {
		for _, tag := range oldVer.Tags {
			if len(tag) >= 2 && tag[0] == "r" {
				oldRelays[tag[1]] = true
			}
		}
	}

	for _, tag := range newVer.Tags {
		if len(tag) >= 2 && tag[0] == "r" {
			newRelays[tag[1]] = true
		}
	}

	// Find added
	for relay := range newRelays {
		if !oldRelays[relay] {
			delta.Added = append(delta.Added, relay)
		}
	}

	// Find removed
	for relay := range oldRelays {
		if !newRelays[relay] {
			delta.Removed = append(delta.Removed, relay)
		}
	}

	return delta
}

// GetPubkeysWithHistory returns pubkeys that have archived versions
func (s *Storage) GetPubkeysWithHistory(ctx context.Context, limit int) ([]string, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT DISTINCT pubkey
		FROM event_history
		ORDER BY (SELECT MAX(archived_at) FROM event_history eh WHERE eh.pubkey = event_history.pubkey) DESC
		LIMIT ?
	`), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pubkeys []string
	for rows.Next() {
		var pk string
		if err := rows.Scan(&pk); err != nil {
			return nil, err
		}
		pubkeys = append(pubkeys, pk)
	}

	return pubkeys, rows.Err()
}
