package stats

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"time"
)

var statsTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Relay Statistics</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
            background: #0d1117;
            min-height: 100vh;
            padding: 2rem;
            color: #c9d1d9;
        }
        .container { max-width: 1400px; margin: 0 auto; }
        header { margin-bottom: 2rem; border-bottom: 1px solid #21262d; padding-bottom: 1rem; }
        h1 { font-size: 1.5rem; font-weight: 600; color: #f0f6fc; margin-bottom: 0.25rem; }
        .subtitle { font-size: 0.875rem; color: #8b949e; }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
            margin-bottom: 2rem;
        }
        .stat-card {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
        }
        .stat-card:hover { border-color: #30363d; }
        .stat-label {
            font-size: 0.625rem;
            color: #8b949e;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 0.5rem;
        }
        .stat-value {
            font-size: 1.75rem;
            font-weight: 600;
            color: #f0f6fc;
            font-variant-numeric: tabular-nums;
            margin-bottom: 0.25rem;
        }
        .stat-subvalue { font-size: 0.75rem; color: #8b949e; }
        .section {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
            margin-bottom: 1rem;
        }
        .section h2 { font-size: 0.875rem; font-weight: 600; margin-bottom: 1rem; color: #f0f6fc; }
        .kind-list { display: flex; flex-direction: column; gap: 0.25rem; }
        .kind-item {
            background: #0d1117;
            border: 1px solid #21262d;
            padding: 0.5rem 0.75rem;
            border-radius: 6px;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .kind-item:hover { border-color: #30363d; }
        .kind-name { color: #c9d1d9; font-size: 0.75rem; }
        .kind-count { font-size: 0.875rem; font-weight: 600; color: #58a6ff; font-variant-numeric: tabular-nums; }
        .footer {
            text-align: center;
            margin-top: 2rem;
            padding-top: 1rem;
            border-top: 1px solid #21262d;
            font-size: 0.75rem;
        }
        .footer a { color: #58a6ff; text-decoration: none; }
        .footer a:hover { text-decoration: underline; }
        @media (max-width: 768px) {
            body { padding: 1rem; }
            .stats-grid { grid-template-columns: 1fr 1fr; }
            .stat-value { font-size: 1.25rem; }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>purplepag.es</h1>
            <div class="subtitle">Relay Statistics</div>
        </header>

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-label">Uptime</div>
                <div class="stat-value">{{.Uptime}}</div>
            </div>

            <div class="stat-card">
                <div class="stat-label">Total Events</div>
                <div class="stat-value">{{.TotalEvents}}</div>
                <div class="stat-subvalue">{{.AcceptedEvents}} accepted · {{.RejectedEvents}} rejected</div>
            </div>

            <div class="stat-card">
                <div class="stat-label">Connections</div>
                <div class="stat-value">{{.ActiveConnections}}</div>
                <div class="stat-subvalue">{{.TotalConnections}} total</div>
            </div>

            <div class="stat-card">
                <div class="stat-label">Event Types</div>
                <div class="stat-value">{{.UniqueKinds}}</div>
                <div class="stat-subvalue">unique kinds stored</div>
            </div>

            <a href="/relays" style="text-decoration: none; color: inherit;">
                <div class="stat-card" style="cursor: pointer;">
                    <div class="stat-label">Discovered Relays</div>
                    <div class="stat-value">{{.DiscoveredRelays}}</div>
                    <div class="stat-subvalue">click to view →</div>
                </div>
            </a>

            <a href="/stats/analytics" style="text-decoration: none; color: inherit;">
                <div class="stat-card" style="cursor: pointer;">
                    <div class="stat-label">REQ Analytics</div>
                    <div class="stat-value">View</div>
                    <div class="stat-subvalue">pubkey popularity & spam detection →</div>
                </div>
            </a>

            <a href="/stats/dashboard" style="text-decoration: none; color: inherit;">
                <div class="stat-card" style="cursor: pointer;">
                    <div class="stat-label">Usage Dashboard</div>
                    <div class="stat-value">View</div>
                    <div class="stat-subvalue">requests & events served over time →</div>
                </div>
            </a>

            <a href="/stats/rejections" style="text-decoration: none; color: inherit;">
                <div class="stat-card" style="cursor: pointer;">
                    <div class="stat-label">Rejection Stats</div>
                    <div class="stat-value">View</div>
                    <div class="stat-subvalue">rejected events & unsupported REQs →</div>
                </div>
            </a>

            <a href="/stats/communities" style="text-decoration: none; color: inherit;">
                <div class="stat-card" style="cursor: pointer;">
                    <div class="stat-label">Social Graph</div>
                    <div class="stat-value">View</div>
                    <div class="stat-subvalue">community clusters visualization →</div>
                </div>
            </a>
        </div>

        <div class="section">
            <h2>Events by Kind</h2>
            <div class="kind-list">
                {{range .KindStats}}
                <div class="kind-item">
                    <span class="kind-name">Kind {{.Kind}} - {{.Name}}</span>
                    <span class="kind-count">{{.Count}}</span>
                </div>
                {{end}}
            </div>
        </div>

        <div class="footer">
            <p>Powered by <a href="https://khatru.nostr.technology/">khatru</a></p>
        </div>
    </div>
</body>
</html>`

type KindStat struct {
	Kind  int
	Name  string
	Count int64
}

type StatsPageData struct {
	Uptime            string
	TotalEvents       int64
	AcceptedEvents    int64
	RejectedEvents    int64
	ActiveConnections int64
	TotalConnections  int64
	UniqueKinds       int
	KindStats         []KindStat
	DiscoveredRelays  int64
}

var kindNames = map[int]string{
	0:     "Profile",
	3:     "Contacts",
	10000: "Mute List",
	10001: "Pinned Notes",
	10002: "Relay List",
	10003: "Bookmarks",
	10004: "Communities",
	10005: "Public Chats",
	10006: "Blocked Relays",
	10007: "Search Relays",
	10009: "Simple Groups",
	10012: "Relay Feeds",
	10015: "Interests",
	10020: "Media Follows",
	10030: "Emojis",
	10050: "DM Relays",
	10101: "Wiki Authors",
	10102: "Wiki Relays",
	30000: "Follow Sets",
	30002: "Relay Sets",
	30003: "Bookmark Sets",
	30004: "Article Sets",
	30005: "Video Sets",
	30007: "Mute Sets",
	30015: "Interest Sets",
	30030: "Emoji Sets",
	30063: "Release Sets",
	30267: "App Sets",
	31924: "Calendar",
	39089: "Starter Packs",
	39092: "Media Packs",
}

func (s *Stats) HandleStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		uptime := s.GetUptime()
		uptimeStr := formatDuration(uptime)

		storageStats := s.GetStorageStats(ctx)

		var totalEvents int64
		kindStats := make([]KindStat, 0)
		for kind, count := range storageStats {
			totalEvents += count
			if count > 0 {
				name := kindNames[kind]
				if name == "" {
					name = fmt.Sprintf("Kind %d", kind)
				}
				kindStats = append(kindStats, KindStat{
					Kind:  kind,
					Name:  name,
					Count: count,
				})
			}
		}

		sort.Slice(kindStats, func(i, j int) bool {
			return kindStats[i].Count > kindStats[j].Count
		})

		data := StatsPageData{
			Uptime:            uptimeStr,
			TotalEvents:       totalEvents,
			AcceptedEvents:    s.GetAcceptedEvents(),
			RejectedEvents:    s.GetRejectedEvents(),
			ActiveConnections: s.GetActiveConnections(),
			TotalConnections:  s.GetTotalConnections(),
			UniqueKinds:       len(kindStats),
			KindStats:         kindStats,
			DiscoveredRelays:  s.GetDiscoveredRelayCount(ctx),
		}

		tmpl, err := template.New("stats").Parse(statsTemplate)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
