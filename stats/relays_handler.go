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
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 2rem;
            color: #fff;
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
        }

        header {
            text-align: center;
            margin-bottom: 3rem;
        }

        h1 {
            font-size: 3rem;
            margin-bottom: 0.5rem;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
        }

        .subtitle {
            font-size: 1.2rem;
            opacity: 0.9;
        }

        .back-link {
            display: inline-block;
            margin-bottom: 2rem;
            color: #fff;
            text-decoration: none;
            opacity: 0.8;
            transition: opacity 0.2s;
        }

        .back-link:hover {
            opacity: 1;
        }

        .table-container {
            background: rgba(255, 255, 255, 0.15);
            backdrop-filter: blur(10px);
            border-radius: 16px;
            padding: 2rem;
            border: 1px solid rgba(255, 255, 255, 0.2);
            overflow-x: auto;
        }

        table {
            width: 100%;
            border-collapse: separate;
            border-spacing: 0;
        }

        thead th {
            background: rgba(255, 255, 255, 0.1);
            padding: 1rem;
            text-align: left;
            font-weight: 600;
            text-transform: uppercase;
            font-size: 0.85rem;
            letter-spacing: 0.5px;
            border-bottom: 2px solid rgba(255, 255, 255, 0.2);
        }

        thead th:first-child {
            border-top-left-radius: 8px;
        }

        thead th:last-child {
            border-top-right-radius: 8px;
        }

        tbody tr {
            transition: background 0.2s;
        }

        tbody tr:hover {
            background: rgba(255, 255, 255, 0.05);
        }

        tbody td {
            padding: 1rem;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }

        .relay-url {
            font-family: 'SF Mono', 'Monaco', 'Cascadia Code', 'Courier New', monospace;
            font-size: 0.9rem;
        }

        .relay-url a {
            color: #a78bfa;
            text-decoration: none;
            transition: color 0.2s;
        }

        .relay-url a:hover {
            color: #c4b5fd;
        }

        .time-ago {
            opacity: 0.8;
            font-size: 0.9rem;
        }

        .success-rate {
            font-weight: 600;
        }

        .success-rate.high {
            color: #86efac;
        }

        .success-rate.medium {
            color: #fde047;
        }

        .success-rate.low {
            color: #fca5a5;
        }

        .status {
            display: inline-block;
            padding: 0.25rem 0.75rem;
            border-radius: 12px;
            font-size: 0.8rem;
            font-weight: 600;
        }

        .status.active {
            background: rgba(134, 239, 172, 0.2);
            color: #86efac;
        }

        .status.inactive {
            background: rgba(252, 165, 165, 0.2);
            color: #fca5a5;
        }

        .events-count {
            font-weight: 700;
            font-variant-numeric: tabular-nums;
        }

        .no-relays {
            text-align: center;
            padding: 3rem;
            opacity: 0.7;
        }

        @media (max-width: 768px) {
            body {
                padding: 1rem;
            }

            h1 {
                font-size: 2rem;
            }

            .table-container {
                padding: 1rem;
            }

            table {
                font-size: 0.85rem;
            }

            thead th, tbody td {
                padding: 0.75rem 0.5rem;
            }
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
