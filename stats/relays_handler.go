package stats

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"time"
)

var relaysTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Discovered Relays</title>
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
        .back-link { display: inline-block; margin-bottom: 1rem; color: #58a6ff; text-decoration: none; font-size: 0.875rem; }
        .back-link:hover { text-decoration: underline; }
        .table-container {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
            overflow-x: auto;
        }
        table { width: 100%; border-collapse: collapse; }
        thead th {
            padding: 0.5rem;
            text-align: left;
            font-weight: 600;
            text-transform: uppercase;
            font-size: 0.625rem;
            color: #8b949e;
            border-bottom: 1px solid #21262d;
        }
        tbody tr:hover { background: #1c2128; }
        tbody td { padding: 0.5rem; border-bottom: 1px solid #21262d; font-size: 0.75rem; }
        .relay-url a { color: #58a6ff; text-decoration: none; }
        .relay-url a:hover { text-decoration: underline; }
        .time-ago { color: #8b949e; }
        .success-rate { font-weight: 600; }
        .success-rate.high { color: #3fb950; }
        .success-rate.medium { color: #d29922; }
        .success-rate.low { color: #f85149; }
        .status {
            display: inline-block;
            padding: 0.125rem 0.5rem;
            border-radius: 4px;
            font-size: 0.625rem;
            font-weight: 600;
        }
        .status.active { background: #238636; color: #fff; }
        .status.inactive { background: #21262d; color: #8b949e; }
        .events-count { font-weight: 600; font-variant-numeric: tabular-nums; color: #f0f6fc; }
        .no-relays { text-align: center; padding: 2rem; color: #8b949e; }
        @media (max-width: 768px) {
            body { padding: 1rem; }
            thead th, tbody td { padding: 0.375rem; }
        }
    </style>
</head>
<body>
    <div class="container">
        <a href="/stats" class="back-link">← Back to Stats</a>

        <header>
            <h1>Discovered Relays</h1>
            <div class="subtitle">{{.TotalCount}} relays discovered from kind:10002 events</div>
        </header>

        {{if .Relays}}
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>Relay URL</th>
                        <th>Pubkeys</th>
                        <th>First Seen</th>
                        <th>Last Sync</th>
                        <th>Success Rate</th>
                        <th>Events</th>
                        <th>Status</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Relays}}
                    <tr>
                        <td class="relay-url"><a href="{{.URL}}" target="_blank" rel="noopener">{{.URL}}</a></td>
                        <td class="events-count">{{.PubkeyCount}}</td>
                        <td class="time-ago">{{.FirstSeenAgo}}</td>
                        <td class="time-ago">{{.LastSyncAgo}}</td>
                        <td class="success-rate {{.SuccessRateClass}}">{{.SuccessRate}}</td>
                        <td class="events-count">{{.EventsContributed}}</td>
                        <td><span class="status {{.StatusClass}}">{{.StatusText}}</span></td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{else}}
        <div class="table-container">
            <div class="no-relays">
                <p>No relays discovered yet. Kind:10002 events will be scanned for relay URLs.</p>
            </div>
        </div>
        {{end}}
    </div>
</body>
</html>`

type RelayInfo struct {
	URL               string
	FirstSeenAgo      string
	LastSyncAgo       string
	SuccessRate       string
	SuccessRateClass  string
	EventsContributed int64
	PubkeyCount       int64
	StatusClass       string
	StatusText        string
}

type RelaysPageData struct {
	TotalCount int
	Relays     []RelayInfo
}

func (s *Stats) HandleRelays() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		relays, err := s.storage.GetRelayStats(ctx)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		relayInfos := make([]RelayInfo, 0, len(relays))
		now := time.Now()

		for _, relay := range relays {
			var successRate float64
			if relay.SyncAttempts > 0 {
				successRate = (float64(relay.SyncSuccesses) / float64(relay.SyncAttempts)) * 100
			}

			successRateStr := fmt.Sprintf("%.1f%%", successRate)
			if relay.SyncAttempts == 0 {
				successRateStr = "—"
			}

			successRateClass := "low"
			if successRate >= 80 {
				successRateClass = "high"
			} else if successRate >= 50 {
				successRateClass = "medium"
			}

			statusClass := "inactive"
			statusText := "Inactive"
			if relay.IsActive {
				statusClass = "active"
				statusText = "Active"
			}

			lastSyncAgo := "never"
			if !relay.LastSync.IsZero() && relay.LastSync.Unix() > 0 {
				lastSyncAgo = formatTimeAgo(now.Sub(relay.LastSync))
			}

			relayInfos = append(relayInfos, RelayInfo{
				URL:               relay.URL,
				FirstSeenAgo:      formatTimeAgo(now.Sub(relay.FirstSeen)),
				LastSyncAgo:       lastSyncAgo,
				SuccessRate:       successRateStr,
				SuccessRateClass:  successRateClass,
				EventsContributed: relay.EventsContributed,
				PubkeyCount:       relay.PubkeyCount,
				StatusClass:       statusClass,
				StatusText:        statusText,
			})
		}

		data := RelaysPageData{
			TotalCount: len(relayInfos),
			Relays:     relayInfos,
		}

		tmpl, err := template.New("relays").Parse(relaysTemplate)
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

func formatTimeAgo(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
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
	if months < 12 {
		return fmt.Sprintf("%d months ago", months)
	}
	years := months / 12
	if years == 1 {
		return "1 year ago"
	}
	return fmt.Sprintf("%d years ago", years)
}
