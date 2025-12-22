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
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: #0a0a0f;
            min-height: 100vh;
            padding: 2rem;
            color: #e4e4e7;
            position: relative;
            overflow-x: hidden;
        }

        body::before {
            content: '';
            position: fixed;
            top: -50%;
            left: -50%;
            width: 200%;
            height: 200%;
            background: radial-gradient(circle at 30% 20%, rgba(139, 92, 246, 0.08) 0%, transparent 50%),
                        radial-gradient(circle at 70% 80%, rgba(217, 70, 239, 0.06) 0%, transparent 50%);
            animation: drift 30s ease-in-out infinite;
            pointer-events: none;
        }

        @keyframes drift {
            0%, 100% { transform: translate(0, 0) rotate(0deg); }
            33% { transform: translate(-5%, 5%) rotate(5deg); }
            66% { transform: translate(5%, -5%) rotate(-5deg); }
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
            position: relative;
            z-index: 1;
        }

        header {
            margin-bottom: 4rem;
            text-align: center;
        }

        h1 {
            font-size: 3.5rem;
            font-weight: 700;
            margin-bottom: 0.5rem;
            background: linear-gradient(135deg, #a78bfa 0%, #e879f9 50%, #a78bfa 100%);
            background-size: 200% 100%;
            background-clip: text;
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            animation: shimmer 8s ease-in-out infinite;
            letter-spacing: -0.02em;
        }

        @keyframes shimmer {
            0%, 100% { background-position: 0% 50%; }
            50% { background-position: 100% 50%; }
        }

        .subtitle {
            font-size: 1rem;
            font-weight: 500;
            color: #a1a1aa;
            text-transform: uppercase;
            letter-spacing: 0.15em;
        }

        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 1.5rem;
            margin-bottom: 3rem;
        }

        .stat-card {
            background: linear-gradient(135deg, rgba(139, 92, 246, 0.05) 0%, rgba(217, 70, 239, 0.02) 100%);
            border: 1px solid rgba(167, 139, 250, 0.15);
            border-radius: 24px;
            padding: 2rem;
            position: relative;
            overflow: hidden;
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
        }

        .stat-card::before {
            content: '';
            position: absolute;
            top: 0;
            left: 0;
            right: 0;
            height: 1px;
            background: linear-gradient(90deg, transparent, rgba(167, 139, 250, 0.4), transparent);
            opacity: 0;
            transition: opacity 0.3s;
        }

        .stat-card:hover {
            border-color: rgba(167, 139, 250, 0.3);
            transform: translateY(-2px);
            box-shadow: 0 20px 40px rgba(139, 92, 246, 0.1);
        }

        .stat-card:hover::before {
            opacity: 1;
        }

        .stat-label {
            font-size: 0.75rem;
            font-weight: 600;
            color: #a1a1aa;
            text-transform: uppercase;
            letter-spacing: 0.1em;
            margin-bottom: 1rem;
        }

        .stat-value {
            font-size: 3.5rem;
            font-weight: 700;
            line-height: 1;
            background: linear-gradient(135deg, #e4e4e7 0%, #a1a1aa 100%);
            background-clip: text;
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            margin-bottom: 0.5rem;
        }

        .stat-subvalue {
            font-size: 0.875rem;
            color: #71717a;
            font-weight: 500;
        }

        .section {
            background: linear-gradient(135deg, rgba(139, 92, 246, 0.03) 0%, rgba(217, 70, 239, 0.01) 100%);
            border: 1px solid rgba(167, 139, 250, 0.1);
            border-radius: 24px;
            padding: 2.5rem;
            margin-bottom: 2rem;
        }

        .section h2 {
            font-size: 1.5rem;
            font-weight: 700;
            margin-bottom: 2rem;
            color: #e4e4e7;
            letter-spacing: -0.01em;
        }

        .kind-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
            gap: 1rem;
        }

        .kind-item {
            background: rgba(139, 92, 246, 0.04);
            border: 1px solid rgba(167, 139, 250, 0.08);
            padding: 1.25rem 1.5rem;
            border-radius: 16px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            transition: all 0.2s;
        }

        .kind-item:hover {
            background: rgba(139, 92, 246, 0.08);
            border-color: rgba(167, 139, 250, 0.2);
        }

        .kind-name {
            font-weight: 500;
            color: #d4d4d8;
            font-size: 0.9rem;
        }

        .kind-count {
            font-size: 1.5rem;
            font-weight: 700;
            color: #a78bfa;
            font-variant-numeric: tabular-nums;
        }

        .relay-list {
            list-style: none;
            margin-top: 1.5rem;
            display: grid;
            gap: 0.75rem;
        }

        .relay-list li {
            font-family: 'SF Mono', 'Monaco', 'Cascadia Code', 'Courier New', monospace;
            font-size: 0.875rem;
            color: #a1a1aa;
            padding: 1rem 1.5rem;
            background: rgba(139, 92, 246, 0.03);
            border: 1px solid rgba(167, 139, 250, 0.08);
            border-radius: 12px;
            transition: all 0.2s;
        }

        .relay-list li:hover {
            background: rgba(139, 92, 246, 0.06);
            border-color: rgba(167, 139, 250, 0.15);
            color: #d4d4d8;
        }

        .relay-list li:before {
            content: "→";
            margin-right: 0.75rem;
            color: #a78bfa;
        }

        .sync-time {
            color: #a1a1aa;
            font-size: 0.9rem;
            margin-bottom: 1.5rem;
        }

        .footer {
            text-align: center;
            margin-top: 4rem;
            padding-top: 2rem;
            border-top: 1px solid rgba(167, 139, 250, 0.1);
        }

        .footer a {
            color: #a78bfa;
            text-decoration: none;
            font-weight: 500;
            transition: color 0.2s;
        }

        .footer a:hover {
            color: #c4b5fd;
        }

        @media (max-width: 768px) {
            body {
                padding: 1.5rem;
            }

            h1 {
                font-size: 2.5rem;
            }

            .stats-grid {
                grid-template-columns: 1fr;
            }

            .kind-grid {
                grid-template-columns: 1fr;
            }

            .section {
                padding: 1.5rem;
            }

            .stat-value {
                font-size: 2.5rem;
            }
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
        </div>

        <div class="section">
            <h2>Events by Kind</h2>
            <div class="kind-grid">
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
