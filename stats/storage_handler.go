package stats

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"

	"github.com/pablof7z/purplepag.es/storage"
)

var (
	storageTmpl *template.Template
)

func init() {
	var err error
	storageTmpl, err = template.New("storage").Parse(storageTemplate)
	if err != nil {
		panic(fmt.Sprintf("failed to parse storage template: %v", err))
	}
}

var storageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Storage Analytics</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
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
        .back-link {
            display: inline-block;
            margin-bottom: 1rem;
            color: #58a6ff;
            text-decoration: none;
            font-size: 0.875rem;
        }
        .back-link:hover { text-decoration: underline; }
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
        .stat-label {
            font-size: 0.75rem;
            color: #8b949e;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 0.5rem;
        }
        .stat-value {
            font-size: 2rem;
            font-weight: 600;
            color: #f0f6fc;
            font-variant-numeric: tabular-nums;
        }
        .chart-section {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1.5rem;
            margin-bottom: 1rem;
        }
        .chart-section h2 {
            font-size: 0.875rem;
            font-weight: 600;
            margin-bottom: 1rem;
            color: #f0f6fc;
        }
        .chart-container { position: relative; height: 300px; }
        .section {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
            margin-bottom: 1rem;
        }
        .section h2 { font-size: 0.875rem; font-weight: 600; margin-bottom: 1rem; color: #f0f6fc; }
        .data-table { width: 100%; border-collapse: collapse; }
        .data-table th, .data-table td { padding: 0.5rem; text-align: left; border-bottom: 1px solid #21262d; }
        .data-table th { color: #8b949e; font-weight: 600; font-size: 0.625rem; text-transform: uppercase; }
        .data-table td { font-size: 0.75rem; }
        .data-table .num { font-variant-numeric: tabular-nums; color: #58a6ff; font-weight: 600; }
        .data-table .mono { color: #c9d1d9; }
        .no-data {
            text-align: center;
            padding: 2rem;
            color: #8b949e;
            font-size: 0.875rem;
        }
        @media (max-width: 768px) {
            body { padding: 1rem; }
            .stat-value { font-size: 1.5rem; }
        }
    </style>
</head>
<body>
    <div class="container">
        <a href="/stats/dashboard" class="back-link">← Back to Dashboard</a>

        <header>
            <h1>purplepag.es</h1>
            <div class="subtitle">Event Storage Analytics</div>
        </header>

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-label">Current Size</div>
                <div class="stat-value">{{.CurrentSize}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Total Events</div>
                <div class="stat-value">{{.EventCount}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Avg Bytes/Event</div>
                <div class="stat-value">{{.BytesPerEvent}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">30-Day Growth</div>
                <div class="stat-value">{{.Growth}}</div>
            </div>
        </div>

        {{if .HasData}}
        <div class="chart-section">
            <h2>Event Data Size (30 Days)</h2>
            <div class="chart-container">
                <canvas id="storageChart"></canvas>
            </div>
        </div>

        <div class="section">
            <h2>Daily Breakdown</h2>
            <table class="data-table">
                <thead>
                    <tr>
                        <th>Date</th>
                        <th>Size</th>
                        <th>Event Count</th>
                        <th>Bytes/Event</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .DailyStats}}
                    <tr>
                        <td class="mono">{{.Date}}</td>
                        <td class="num">{{.SizeFormatted}}</td>
                        <td class="num">{{.EventCount}}</td>
                        <td class="num">{{.BytesPerEventFormatted}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{else}}
        <div class="no-data">
            <p>Collecting storage data...</p>
            <p style="margin-top: 0.5rem; font-size: 0.75rem;">Daily snapshots will appear here once data collection begins.</p>
        </div>
        {{end}}
    </div>

    {{if .HasData}}
    <script>
        const storageData = {{.StorageDataJSON}};

        const ctx = document.getElementById('storageChart').getContext('2d');
        new Chart(ctx, {
            type: 'line',
            data: {
                labels: storageData.labels,
                datasets: [{
                    label: 'Storage Size (bytes)',
                    data: storageData.sizes,
                    borderColor: '#58a6ff',
                    backgroundColor: 'rgba(88, 166, 255, 0.1)',
                    fill: true,
                    tension: 0.3
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                const bytes = context.parsed.y;
                                const formatted = formatBytes(bytes);
                                return 'Size: ' + formatted;
                            }
                        }
                    }
                },
                scales: {
                    x: {
                        grid: { color: '#21262d' },
                        ticks: { color: '#8b949e', maxRotation: 45, minRotation: 45, font: { family: 'monospace', size: 10 } }
                    },
                    y: {
                        grid: { color: '#21262d' },
                        ticks: {
                            color: '#8b949e',
                            font: { family: 'monospace', size: 10 },
                            callback: function(value) {
                                return formatBytes(value);
                            }
                        },
                        beginAtZero: true
                    }
                }
            }
        });

        function formatBytes(bytes) {
            const KB = 1024;
            const MB = 1024 * KB;
            const GB = 1024 * MB;

            if (bytes >= GB) {
                return (bytes / GB).toFixed(2) + ' GB';
            } else if (bytes >= MB) {
                return (bytes / MB).toFixed(2) + ' MB';
            } else if (bytes >= KB) {
                return (bytes / KB).toFixed(2) + ' KB';
            } else {
                return bytes + ' B';
            }
        }
    </script>
    {{end}}
</body>
</html>`

// StorageHandler handles HTTP requests for storage analytics.
type StorageHandler struct {
	storage *storage.Storage
}

// NewStorageHandler creates a new storage analytics handler with the given storage backend.
func NewStorageHandler(storage *storage.Storage) *StorageHandler {
	return &StorageHandler{storage: storage}
}

// DailyStatDisplay represents a single day's storage statistics for display.
type DailyStatDisplay struct {
	Date                   string
	SizeFormatted          string
	EventCount             int64
	BytesPerEventFormatted string
}

// StoragePageData contains all data needed to render the storage analytics template.
type StoragePageData struct {
	CurrentSize      string
	EventCount       string
	BytesPerEvent    string
	Growth           string
	HasData          bool
	DailyStats       []DailyStatDisplay
	StorageDataJSON  template.JS
}

// HandleStorage returns an HTTP handler function that renders the storage analytics page.
func (h *StorageHandler) HandleStorage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get current storage info
		currentInfo, err := h.storage.GetCurrentStorageInfo(ctx)
		if err != nil {
			currentInfo = &storage.DailyStorageStats{}
		}

		// Get 30-day historical data
		dailyStats, err := h.storage.GetDailyStorageStats(ctx, 30)
		if err != nil {
			dailyStats = []storage.DailyStorageStats{}
		}

		// Get growth percentage
		growth, err := h.storage.GetStorageGrowth(ctx, 30)
		if err != nil {
			growth = 0
		}

		// Format current stats
		currentSize := FormatBytes(currentInfo.EventTableBytes)
		eventCount := FormatNumber(currentInfo.EventCount)
		bytesPerEvent := FormatBytes(currentInfo.BytesPerEvent)

		growthStr := "—"
		if growth != 0 {
			if growth > 0 {
				growthStr = fmt.Sprintf("+%.1f%%", growth)
			} else {
				growthStr = fmt.Sprintf("%.1f%%", growth)
			}
		}

		// Format daily stats for display
		dailyStatsDisplay := make([]DailyStatDisplay, len(dailyStats))
		labels := make([]string, len(dailyStats))
		sizes := make([]int64, len(dailyStats))

		for i, stat := range dailyStats {
			dailyStatsDisplay[i] = DailyStatDisplay{
				Date:                   stat.Date,
				SizeFormatted:          FormatBytes(stat.EventTableBytes),
				EventCount:             stat.EventCount,
				BytesPerEventFormatted: FormatBytes(stat.BytesPerEvent),
			}
			labels[i] = stat.Date
			sizes[i] = stat.EventTableBytes
		}

		// Prepare chart data
		chartData := map[string]interface{}{
			"labels": labels,
			"sizes":  sizes,
		}
		chartDataJSON, _ := json.Marshal(chartData)

		data := StoragePageData{
			CurrentSize:     currentSize,
			EventCount:      eventCount,
			BytesPerEvent:   bytesPerEvent,
			Growth:          growthStr,
			HasData:         len(dailyStats) > 0,
			DailyStats:      dailyStatsDisplay,
			StorageDataJSON: template.JS(chartDataJSON),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := storageTmpl.Execute(w, data); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}
