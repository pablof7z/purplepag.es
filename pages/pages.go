package pages

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"

	"github.com/nbd-wtf/go-nostr"
	"github.com/pablof7z/purplepag.es/storage"
)

type Handler struct {
	storage *storage.Storage
}

func NewHandler(store *storage.Storage) *Handler {
	return &Handler{storage: store}
}

type Profile struct {
	Pubkey        string
	Name          string
	DisplayName   string
	Picture       string
	About         string
	Nip05         string
	FollowerCount int
	FollowingCount int
	Npub          string
}

var rankingsFuncs = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	"mul": func(a, b int) int { return a * b },
	"slice": func(s string, start, end int) string {
		if len(s) == 0 {
			return ""
		}
		if start >= len(s) {
			return string(s[0])
		}
		return s[start:end]
	},
}

func (h *Handler) HandleRankings(w http.ResponseWriter, r *http.Request) {
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 50
	offset := (page - 1) * limit

	contactLists, err := h.storage.QueryEvents(context.Background(), nostr.Filter{
		Kinds: []int{3},
	})
	if err != nil {
		http.Error(w, "Failed to query contact lists", http.StatusInternalServerError)
		return
	}

	followerCounts := make(map[string]int)
	latestContactList := make(map[string]*nostr.Event)

	for _, evt := range contactLists {
		if existing, ok := latestContactList[evt.PubKey]; !ok || evt.CreatedAt > existing.CreatedAt {
			latestContactList[evt.PubKey] = evt
		}
	}

	for _, evt := range latestContactList {
		for _, tag := range evt.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				pubkey := tag[1]
				followerCounts[pubkey]++
			}
		}
	}

	type pubkeyCount struct {
		pubkey string
		count  int
	}

	ranked := make([]pubkeyCount, 0, len(followerCounts))
	for pubkey, count := range followerCounts {
		ranked = append(ranked, pubkeyCount{pubkey, count})
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].count > ranked[j].count
	})

	total := len(ranked)
	totalPages := (total + limit - 1) / limit

	if offset >= total {
		offset = 0
		page = 1
	}

	end := offset + limit
	if end > total {
		end = total
	}

	topPubkeys := ranked[offset:end]

	profiles := make([]Profile, 0, len(topPubkeys))
	for _, pc := range topPubkeys {
		profile := h.getProfile(pc.pubkey)
		profile.FollowerCount = pc.count
		profile.Npub = convertToNpub(pc.pubkey)
		profiles = append(profiles, profile)
	}

	data := struct {
		Profiles    []Profile
		Page        int
		TotalPages  int
		HasPrev     bool
		HasNext     bool
		Total       int
	}{
		Profiles:   profiles,
		Page:       page,
		TotalPages: totalPages,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
		Total:      total,
	}

	tmpl := template.Must(template.New("rankings").Funcs(rankingsFuncs).Parse(rankingsTemplate))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
}

func (h *Handler) HandleProfile(w http.ResponseWriter, r *http.Request) {
	pubkey := r.URL.Query().Get("pubkey")
	if pubkey == "" {
		http.Error(w, "Missing pubkey parameter", http.StatusBadRequest)
		return
	}

	profile := h.getProfile(pubkey)
	profile.Npub = convertToNpub(pubkey)

	contactLists, _ := h.storage.QueryEvents(context.Background(), nostr.Filter{
		Kinds:   []int{3},
		Authors: []string{pubkey},
	})

	following := make([]Profile, 0)
	if len(contactLists) > 0 {
		var latest *nostr.Event
		for _, evt := range contactLists {
			if latest == nil || evt.CreatedAt > latest.CreatedAt {
				latest = evt
			}
		}

		for _, tag := range latest.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				fpubkey := tag[1]
				fp := h.getProfile(fpubkey)
				fp.Npub = convertToNpub(fpubkey)
				following = append(following, fp)
			}
		}
		profile.FollowingCount = len(following)
	}

	// Get follower count from storage
	followerCount, _ := h.storage.GetFollowerCount(context.Background(), pubkey)
	profile.FollowerCount = int(followerCount)

	data := struct {
		Profile   Profile
		Following []Profile
	}{
		Profile:   profile,
		Following: following,
	}

	tmpl := template.Must(template.New("profile").Funcs(rankingsFuncs).Parse(profileTemplate))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
}

func (h *Handler) getProfile(pubkey string) Profile {
	events, err := h.storage.QueryEvents(context.Background(), nostr.Filter{
		Kinds:   []int{0},
		Authors: []string{pubkey},
	})

	profile := Profile{
		Pubkey: pubkey,
		Name:   pubkey[:16] + "...",
	}

	if err != nil || len(events) == 0 {
		return profile
	}

	var latest *nostr.Event
	for _, evt := range events {
		if latest == nil || evt.CreatedAt > latest.CreatedAt {
			latest = evt
		}
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(latest.Content), &metadata); err != nil {
		return profile
	}

	if name, ok := metadata["name"].(string); ok && name != "" {
		profile.Name = name
	}
	if displayName, ok := metadata["display_name"].(string); ok {
		profile.DisplayName = displayName
	}
	if picture, ok := metadata["picture"].(string); ok {
		profile.Picture = picture
	}
	if about, ok := metadata["about"].(string); ok {
		profile.About = about
	}
	if nip05, ok := metadata["nip05"].(string); ok {
		profile.Nip05 = nip05
	}

	return profile
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func convertToNpub(hex string) string {
	if len(hex) != 64 {
		return hex
	}
	return fmt.Sprintf("npub1%s", hex[:8])
}
