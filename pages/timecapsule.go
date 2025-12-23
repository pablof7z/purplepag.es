package pages

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/pablof7z/purplepag.es/storage"
)

type TimecapsuleHandler struct {
	storage *storage.Storage
}

func NewTimecapsuleHandler(store *storage.Storage) *TimecapsuleHandler {
	return &TimecapsuleHandler{storage: store}
}

type VersionView struct {
	ID         string
	Kind       int
	KindName   string
	CreatedAt  string
	ArchivedAt string
	Summary    string
}

type ProfileChangeView struct {
	Field    string
	OldValue string
	NewValue string
}

type ContactChangeView struct {
	Pubkey string
	Name   string
	Action string // "followed" or "unfollowed"
}

type RelayChangeView struct {
	URL    string
	Action string // "added" or "removed"
}

type DeltaView struct {
	PubKey         string
	PubKeyShort    string
	Name           string
	Kind           int
	KindName       string
	Timestamp      string
	TimestampAgo   string
	ProfileChanges []ProfileChangeView
	ContactChanges []ContactChangeView
	RelayChanges   []RelayChangeView
}

type TimecapsulePageData struct {
	TotalVersions  int64
	UniquePubkeys  int64
	SearchPubkey   string
	SearchName     string
	RecentDeltas   []DeltaView
	PubkeyHistory  []DeltaView
	Error          string
}

func (h *TimecapsuleHandler) HandleTimecapsule() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		data := TimecapsulePageData{}

		// Get stats
		data.TotalVersions, data.UniquePubkeys, _ = h.storage.GetEventHistoryStats(ctx)

		// Check for pubkey search
		if pubkey := r.URL.Query().Get("pubkey"); pubkey != "" {
			data.SearchPubkey = pubkey
			data.PubkeyHistory = h.getPubkeyDeltas(ctx, pubkey)

			// Get name for display
			names, _ := h.storage.GetProfileNames(ctx, []string{pubkey})
			data.SearchName = names[pubkey]
		} else {
			// Show recent changes
			data.RecentDeltas = h.getRecentDeltas(ctx, 50)
		}

		tmpl, err := template.New("timecapsule").Parse(timecapsuleTemplate)
		if err != nil {
			http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

func (h *TimecapsuleHandler) getRecentDeltas(ctx context.Context, limit int) []DeltaView {
	versions, _ := h.storage.GetRecentChanges(ctx, 0, limit)

	var deltas []DeltaView
	for _, v := range versions {
		delta := h.buildDelta(ctx, &v, nil)
		if delta != nil {
			deltas = append(deltas, *delta)
		}
	}
	return deltas
}

func (h *TimecapsuleHandler) getPubkeyDeltas(ctx context.Context, pubkey string) []DeltaView {
	// Get all versions for this pubkey
	versions, _ := h.storage.GetAllEventHistory(ctx, pubkey, 100)

	// Also get current events to show the latest state
	currentEvents := make(map[int]*storage.EventVersion)
	for _, kind := range []int{0, 3, 10002} {
		events, _ := h.storage.QueryEvents(ctx, nostr.Filter{
			Kinds:   []int{kind},
			Authors: []string{pubkey},
			Limit:   1,
		})
		if len(events) > 0 {
			evt := events[0]
			currentEvents[kind] = &storage.EventVersion{
				ID:        evt.ID,
				PubKey:    evt.PubKey,
				Kind:      evt.Kind,
				CreatedAt: evt.CreatedAt,
				Content:   evt.Content,
				Tags:      evt.Tags,
			}
		}
	}

	// Group versions by kind for delta calculation
	versionsByKind := make(map[int][]storage.EventVersion)
	for _, v := range versions {
		versionsByKind[v.Kind] = append(versionsByKind[v.Kind], v)
	}

	var deltas []DeltaView

	// For each kind, calculate deltas between consecutive versions
	for kind, kindVersions := range versionsByKind {
		// Add current event as the newest version if we have one
		var allVersions []storage.EventVersion
		if current, ok := currentEvents[kind]; ok {
			allVersions = append(allVersions, *current)
		}
		allVersions = append(allVersions, kindVersions...)

		// Calculate deltas between consecutive versions
		for i := 0; i < len(allVersions)-1; i++ {
			newer := allVersions[i]
			older := allVersions[i+1]
			delta := h.buildDelta(ctx, &newer, &older)
			if delta != nil && (len(delta.ProfileChanges) > 0 || len(delta.ContactChanges) > 0 || len(delta.RelayChanges) > 0) {
				deltas = append(deltas, *delta)
			}
		}

		// Show first version as "initial"
		if len(allVersions) > 0 {
			oldest := allVersions[len(allVersions)-1]
			delta := h.buildDelta(ctx, &oldest, nil)
			if delta != nil {
				deltas = append(deltas, *delta)
			}
		}
	}

	return deltas
}

func (h *TimecapsuleHandler) buildDelta(ctx context.Context, newVer *storage.EventVersion, oldVer *storage.EventVersion) *DeltaView {
	names, _ := h.storage.GetProfileNames(ctx, []string{newVer.PubKey})

	delta := &DeltaView{
		PubKey:       newVer.PubKey,
		PubKeyShort:  shortPubkey(newVer.PubKey),
		Name:         names[newVer.PubKey],
		Kind:         newVer.Kind,
		KindName:     kindName(newVer.Kind),
		Timestamp:    time.Unix(int64(newVer.CreatedAt), 0).Format("2006-01-02 15:04"),
		TimestampAgo: formatTimeAgo(time.Since(time.Unix(int64(newVer.CreatedAt), 0))),
	}

	switch newVer.Kind {
	case 0:
		profileDelta := storage.CalculateProfileDelta(oldVer, newVer)
		for _, c := range profileDelta.Changes {
			delta.ProfileChanges = append(delta.ProfileChanges, ProfileChangeView{
				Field:    c.Field,
				OldValue: truncate(c.OldValue, 100),
				NewValue: truncate(c.NewValue, 100),
			})
		}
	case 3:
		contactsDelta := storage.CalculateContactsDelta(oldVer, newVer)
		// Get names for the changed pubkeys
		allPks := append(contactsDelta.Added, contactsDelta.Removed...)
		pkNames, _ := h.storage.GetProfileNames(ctx, allPks)

		for _, pk := range contactsDelta.Added {
			delta.ContactChanges = append(delta.ContactChanges, ContactChangeView{
				Pubkey: shortPubkey(pk),
				Name:   pkNames[pk],
				Action: "followed",
			})
		}
		for _, pk := range contactsDelta.Removed {
			delta.ContactChanges = append(delta.ContactChanges, ContactChangeView{
				Pubkey: shortPubkey(pk),
				Name:   pkNames[pk],
				Action: "unfollowed",
			})
		}
	case 10002:
		relaysDelta := storage.CalculateRelaysDelta(oldVer, newVer)
		for _, r := range relaysDelta.Added {
			delta.RelayChanges = append(delta.RelayChanges, RelayChangeView{
				URL:    r,
				Action: "added",
			})
		}
		for _, r := range relaysDelta.Removed {
			delta.RelayChanges = append(delta.RelayChanges, RelayChangeView{
				URL:    r,
				Action: "removed",
			})
		}
	}

	return delta
}

func shortPubkey(pk string) string {
	if len(pk) <= 16 {
		return pk
	}
	return pk[:8] + "..." + pk[len(pk)-8:]
}

func kindName(kind int) string {
	names := map[int]string{
		0:     "Profile",
		3:     "Contacts",
		10002: "Relay List",
		10000: "Mute List",
		10001: "Pinned Notes",
		10003: "Bookmarks",
	}
	if name, ok := names[kind]; ok {
		return name
	}
	return fmt.Sprintf("Kind %d", kind)
}

func formatTimeAgo(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", m)
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	if days < 30 {
		return fmt.Sprintf("%d days ago", days)
	}
	months := days / 30
	if months == 1 {
		return "1 month ago"
	}
	return fmt.Sprintf("%d months ago", months)
}

var timecapsuleTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Time Capsule</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
            background: #0d1117;
            min-height: 100vh;
            padding: 2rem;
            color: #c9d1d9;
        }
        .container { max-width: 1000px; margin: 0 auto; }
        .back-link {
            display: inline-block;
            margin-bottom: 1.5rem;
            color: #58a6ff;
            text-decoration: none;
            font-size: 0.875rem;
        }
        .back-link:hover {
            text-decoration: underline;
        }
        header {
            margin-bottom: 2rem;
            text-align: center;
            border-bottom: 1px solid #21262d;
            padding-bottom: 1rem;
        }
        h1 {
            font-size: 2rem;
            font-weight: 600;
            margin-bottom: 0.5rem;
            color: #f0f6fc;
        }
        .subtitle {
            color: #8b949e;
            font-size: 0.875rem;
        }
        .stats-row {
            display: flex;
            gap: 1rem;
            margin-bottom: 2rem;
            justify-content: center;
        }
        .stat-box {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem 2rem;
            text-align: center;
        }
        .stat-box .value {
            font-size: 2rem;
            font-weight: 600;
            color: #58a6ff;
            font-variant-numeric: tabular-nums;
        }
        .stat-box .label {
            font-size: 0.75rem;
            color: #8b949e;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
        .search-box {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1.5rem;
            margin-bottom: 2rem;
        }
        .search-box input {
            width: 100%;
            padding: 0.75rem 1rem;
            font-size: 0.875rem;
            background: #0d1117;
            border: 1px solid #30363d;
            border-radius: 6px;
            color: #c9d1d9;
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
        }
        .search-box input:focus {
            outline: none;
            border-color: #58a6ff;
        }
        .search-box input::placeholder {
            color: #8b949e;
        }
        .search-box button {
            margin-top: 0.75rem;
            padding: 0.5rem 1.5rem;
            background: #238636;
            border: none;
            border-radius: 6px;
            color: #ffffff;
            font-weight: 600;
            cursor: pointer;
            font-size: 0.875rem;
            font-family: inherit;
        }
        .search-box button:hover {
            background: #2ea043;
        }
        .delta-card {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1.5rem;
            margin-bottom: 1rem;
        }
        .delta-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 1rem;
            padding-bottom: 0.75rem;
            border-bottom: 1px solid #21262d;
        }
        .delta-user {
            display: flex;
            align-items: center;
            gap: 0.75rem;
        }
        .delta-name {
            font-weight: 600;
            color: #f0f6fc;
        }
        .delta-pubkey {
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
            font-size: 0.75rem;
            color: #8b949e;
        }
        .delta-meta {
            text-align: right;
        }
        .delta-kind {
            display: inline-block;
            padding: 0.25rem 0.75rem;
            background: #388bfd26;
            border: 1px solid #388bfd;
            border-radius: 20px;
            font-size: 0.75rem;
            color: #58a6ff;
            margin-bottom: 0.25rem;
        }
        .delta-time {
            font-size: 0.75rem;
            color: #8b949e;
        }
        .change-list { list-style: none; }
        .change-item {
            padding: 0.5rem 0;
            border-bottom: 1px solid #21262d;
            display: flex;
            align-items: flex-start;
            gap: 0.75rem;
            font-size: 0.875rem;
        }
        .change-item:last-child { border-bottom: none; }
        .change-field {
            min-width: 100px;
            font-weight: 500;
            color: #8b949e;
            font-size: 0.875rem;
        }
        .change-values { flex: 1; }
        .old-value {
            color: #f85149;
            text-decoration: line-through;
            font-size: 0.875rem;
        }
        .new-value {
            color: #3fb950;
            font-size: 0.875rem;
        }
        .follow-action {
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.25rem 0.75rem;
            border-radius: 6px;
            font-size: 0.8rem;
            margin: 0.25rem;
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
        }
        .follow-action.followed {
            background: rgba(63, 185, 80, 0.15);
            border: 1px solid #3fb950;
            color: #3fb950;
        }
        .follow-action.unfollowed {
            background: rgba(248, 81, 73, 0.15);
            border: 1px solid #f85149;
            color: #f85149;
        }
        .relay-action {
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.25rem 0.75rem;
            border-radius: 6px;
            font-size: 0.75rem;
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
            margin: 0.25rem;
        }
        .relay-action.added {
            background: rgba(63, 185, 80, 0.15);
            border: 1px solid #3fb950;
            color: #3fb950;
        }
        .relay-action.removed {
            background: rgba(248, 81, 73, 0.15);
            border: 1px solid #f85149;
            color: #f85149;
        }
        .empty-state {
            text-align: center;
            padding: 3rem;
            color: #8b949e;
            font-size: 0.875rem;
        }
        .section-title {
            font-size: 1.125rem;
            font-weight: 600;
            margin-bottom: 1.5rem;
            color: #f0f6fc;
        }
        @media (max-width: 768px) {
            body { padding: 1rem; }
            h1 { font-size: 1.75rem; }
            .stats-row { flex-direction: column; }
            .delta-header { flex-direction: column; align-items: flex-start; gap: 0.5rem; }
            .delta-meta { text-align: left; }
        }
    </style>
</head>
<body>
    <div class="container">
        <a href="/" class="back-link">← Back to Home</a>

        <header>
            <h1>Time Capsule</h1>
            <p class="subtitle">Track changes in profiles, follows, and relays over time</p>
        </header>

        <div class="stats-row">
            <div class="stat-box">
                <div class="value">{{.TotalVersions}}</div>
                <div class="label">Archived Versions</div>
            </div>
            <div class="stat-box">
                <div class="value">{{.UniquePubkeys}}</div>
                <div class="label">Users Tracked</div>
            </div>
        </div>

        <div class="search-box">
            <form method="GET">
                <input type="text" name="pubkey" placeholder="Search by pubkey (hex)..." value="{{.SearchPubkey}}">
                <button type="submit">Search</button>
            </form>
        </div>

        {{if .SearchPubkey}}
        <h2 class="section-title">
            History for {{if .SearchName}}{{.SearchName}}{{else}}{{.SearchPubkey}}{{end}}
        </h2>
        {{if .PubkeyHistory}}
            {{range .PubkeyHistory}}
            <div class="delta-card">
                <div class="delta-header">
                    <div class="delta-user">
                        <div>
                            {{if $.SearchName}}<div class="delta-name">{{$.SearchName}}</div>{{end}}
                            <div class="delta-pubkey">{{.PubKeyShort}}</div>
                        </div>
                    </div>
                    <div class="delta-meta">
                        <div class="delta-kind">{{.KindName}}</div>
                        <div class="delta-time">{{.Timestamp}} ({{.TimestampAgo}})</div>
                    </div>
                </div>

                {{if .ProfileChanges}}
                <ul class="change-list">
                    {{range .ProfileChanges}}
                    <li class="change-item">
                        <span class="change-field">{{.Field}}</span>
                        <div class="change-values">
                            {{if .OldValue}}<div class="old-value">{{.OldValue}}</div>{{end}}
                            <div class="new-value">{{if .NewValue}}{{.NewValue}}{{else}}<em>(cleared)</em>{{end}}</div>
                        </div>
                    </li>
                    {{end}}
                </ul>
                {{end}}

                {{if .ContactChanges}}
                <div style="display: flex; flex-wrap: wrap;">
                    {{range .ContactChanges}}
                    <span class="follow-action {{.Action}}">
                        {{if eq .Action "followed"}}+{{else}}-{{end}}
                        {{if .Name}}{{.Name}}{{else}}{{.Pubkey}}{{end}}
                    </span>
                    {{end}}
                </div>
                {{end}}

                {{if .RelayChanges}}
                <div style="display: flex; flex-wrap: wrap;">
                    {{range .RelayChanges}}
                    <span class="relay-action {{.Action}}">
                        {{if eq .Action "added"}}+{{else}}-{{end}} {{.URL}}
                    </span>
                    {{end}}
                </div>
                {{end}}

                {{if and (not .ProfileChanges) (not .ContactChanges) (not .RelayChanges)}}
                <div style="color: #8b949e; font-style: italic;">Initial version</div>
                {{end}}
            </div>
            {{end}}
        {{else}}
        <div class="empty-state">No history found for this pubkey</div>
        {{end}}

        {{else}}

        <h2 class="section-title">Recent Changes</h2>
        {{if .RecentDeltas}}
            {{range .RecentDeltas}}
            <div class="delta-card">
                <div class="delta-header">
                    <div class="delta-user">
                        <div>
                            {{if .Name}}<div class="delta-name">{{.Name}}</div>{{end}}
                            <a href="/timecapsule?pubkey={{.PubKey}}" class="delta-pubkey" style="color: #58a6ff; text-decoration: none;">{{.PubKeyShort}}</a>
                        </div>
                    </div>
                    <div class="delta-meta">
                        <div class="delta-kind">{{.KindName}}</div>
                        <div class="delta-time">{{.TimestampAgo}}</div>
                    </div>
                </div>

                {{if .ProfileChanges}}
                <ul class="change-list">
                    {{range .ProfileChanges}}
                    <li class="change-item">
                        <span class="change-field">{{.Field}}</span>
                        <div class="change-values">
                            {{if .OldValue}}<div class="old-value">{{.OldValue}}</div>{{end}}
                            <div class="new-value">{{if .NewValue}}{{.NewValue}}{{else}}<em>(cleared)</em>{{end}}</div>
                        </div>
                    </li>
                    {{end}}
                </ul>
                {{end}}

                {{if .ContactChanges}}
                <div style="display: flex; flex-wrap: wrap;">
                    {{range .ContactChanges}}
                    <span class="follow-action {{.Action}}">
                        {{if eq .Action "followed"}}+{{else}}-{{end}}
                        {{if .Name}}{{.Name}}{{else}}{{.Pubkey}}{{end}}
                    </span>
                    {{end}}
                </div>
                {{end}}

                {{if .RelayChanges}}
                <div style="display: flex; flex-wrap: wrap;">
                    {{range .RelayChanges}}
                    <span class="relay-action {{.Action}}">
                        {{if eq .Action "added"}}+{{else}}-{{end}} {{.URL}}
                    </span>
                    {{end}}
                </div>
                {{end}}
            </div>
            {{end}}
        {{else}}
        <div class="empty-state">
            <p>No changes recorded yet.</p>
            <p style="margin-top: 0.5rem; font-size: 0.875rem;">Changes will appear here as users update their profiles, follows, and relay lists.</p>
        </div>
        {{end}}
        {{end}}
    </div>
</body>
</html>`
