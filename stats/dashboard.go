package stats

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"

	"github.com/pablof7z/purplepag.es/storage"
)

var (
	dashboardTmpl *template.Template
)

func init() {
	var err error
	dashboardTmpl, err = template.New("dashboard").Parse(dashboardTemplate)
	if err != nil {
		panic(fmt.Sprintf("failed to parse dashboard template: %v", err))
	}
}

var dashboardTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Usage Dashboard</title>
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
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(5, 1fr);
            gap: 1rem;
            margin-bottom: 2rem;
        }
        .stat-card {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
        }
        .stat-card-link {
            text-decoration: none;
            color: inherit;
        }
        .stat-card-clickable {
            cursor: pointer;
            transition: border-color 0.2s;
        }
        .stat-card-clickable:hover {
            border-color: #58a6ff;
        }
        .stat-value-small {
            font-size: 1.5rem;
        }
        .growth-label {
            font-size: 0.75rem;
            color: #8b949e;
            margin-top: 0.5rem;
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
        .back-link {
            display: inline-block;
            margin-bottom: 1rem;
            color: #58a6ff;
            text-decoration: none;
            font-size: 0.875rem;
        }
        .back-link:hover { text-decoration: underline; }
        .toggle-container { display: flex; gap: 0.5rem; margin-bottom: 1rem; }
        .toggle-btn {
            padding: 0.375rem 0.75rem;
            background: #21262d;
            border: 1px solid #30363d;
            border-radius: 6px;
            color: #8b949e;
            cursor: pointer;
            font-size: 0.75rem;
            font-family: inherit;
        }
        .toggle-btn.active {
            background: #388bfd26;
            border-color: #388bfd;
            color: #58a6ff;
        }
        .toggle-btn:hover { border-color: #8b949e; }
        .aggregation-toggle {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            margin-bottom: 1.5rem;
            padding: 0.75rem;
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
        }
        .aggregation-toggle span { font-size: 0.75rem; }
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
        .data-table .ptr { color: #8b949e; font-size: 0.625rem; }
        @media (max-width: 768px) {
            body { padding: 1rem; }
            .stat-value { font-size: 1.5rem; }
            .stats-grid {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <a href="/stats" class="back-link">← Back to Stats</a>

        <header>
            <h1>purplepag.es</h1>
            <div class="subtitle">Usage Dashboard</div>
        </header>

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-label">Today's REQs</div>
                <div class="stat-value">{{.TodayREQs}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Today's Unique IPs</div>
                <div class="stat-value">{{.TodayUniqueIPs}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Today's Events Served</div>
                <div class="stat-value">{{.TodayEventsServed}}</div>
            </div>
            <a href="/stats/storage" class="stat-card-link">
                <div class="stat-card stat-card-clickable">
                    <div class="stat-label">Event Data Size</div>
                    <div class="stat-value stat-value-small">{{.StorageSize}}</div>
                    <div class="growth-label">{{.StorageGrowth}} this month</div>
                </div>
            </a>
        </div>

        <div class="aggregation-toggle">
            <span style="color: #8b949e; margin-right: 1rem;">Aggregation:</span>
            <button class="toggle-btn active" onclick="setAggregation('day')">Daily (30d)</button>
            <button class="toggle-btn" onclick="setAggregation('hour')">Hourly (72h)</button>
        </div>

        <div class="chart-section">
            <h2 id="reqsTitle">REQs per Day</h2>
            <div class="toggle-container">
                <button class="toggle-btn active" onclick="toggleREQsView('total')">Total REQs</button>
                <button class="toggle-btn" onclick="toggleREQsView('unique')">Unique IPs</button>
            </div>
            <div class="chart-container">
                <canvas id="reqsChart"></canvas>
            </div>
        </div>

        <div class="chart-section">
            <h2 id="eventsTitle">Events Served per Day</h2>
            <div class="toggle-container">
                <button class="toggle-btn active" onclick="toggleEventsView('total')">Total</button>
                <button class="toggle-btn" onclick="toggleEventsView('avg')">Avg per IP</button>
            </div>
            <div class="chart-container">
                <canvas id="eventsChart"></canvas>
            </div>
        </div>

        {{if .TopIPs}}
        <div class="section">
            <h2>Top 20 IPs by Events Served</h2>
            <table class="data-table">
                <thead>
                    <tr>
                        <th>IP Address</th>
                        <th>PTR Record</th>
                        <th>REQs</th>
                        <th>Events Served</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .TopIPs}}
                    <tr>
                        <td class="mono">{{.IP}}</td>
                        <td class="ptr">{{.PTR}}</td>
                        <td class="num">{{.TotalREQs}}</td>
                        <td class="num">{{.EventsServed}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{end}}
    </div>

    <script>
        const dailyData = {{.DailyStatsJSON}};
        const hourlyData = {{.HourlyStatsJSON}};

        let currentAggregation = 'day';
        let currentREQsView = 'total';
        let currentEventsView = 'total';

        function getDataForAggregation(agg) {
            if (agg === 'hour') {
                return {
                    labels: hourlyData.map(s => s.Hour ? s.Hour.substring(5) : ''), // "01-15 14" format
                    totalREQs: hourlyData.map(s => s.TotalREQs),
                    uniqueIPs: hourlyData.map(s => s.UniqueIPs),
                    eventsServed: hourlyData.map(s => s.EventsServed),
                    avgEventsPerREQ: hourlyData.map(s => s.TotalREQs > 0 ? Math.round(s.EventsServed / s.TotalREQs) : 0)
                };
            }
            return {
                labels: dailyData.map(s => s.Date),
                totalREQs: dailyData.map(s => s.TotalREQs),
                uniqueIPs: dailyData.map(s => s.UniqueIPs),
                eventsServed: dailyData.map(s => s.EventsServed),
                avgEventsPerREQ: dailyData.map(s => s.TotalREQs > 0 ? Math.round(s.EventsServed / s.TotalREQs) : 0)
            };
        }

        let data = getDataForAggregation('day');

        const chartOptions = {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { display: false }
            },
            scales: {
                x: {
                    grid: { color: '#21262d' },
                    ticks: { color: '#8b949e', maxRotation: 45, minRotation: 45, font: { family: 'monospace', size: 10 } }
                },
                y: {
                    grid: { color: '#21262d' },
                    ticks: { color: '#8b949e', font: { family: 'monospace', size: 10 } },
                    beginAtZero: true
                }
            }
        };

        const reqsCtx = document.getElementById('reqsChart').getContext('2d');
        const reqsChart = new Chart(reqsCtx, {
            type: 'line',
            data: {
                labels: data.labels,
                datasets: [{
                    label: 'Total REQs',
                    data: data.totalREQs,
                    borderColor: '#58a6ff',
                    backgroundColor: 'rgba(88, 166, 255, 0.1)',
                    fill: true,
                    tension: 0.3
                }]
            },
            options: chartOptions
        });

        const eventsCtx = document.getElementById('eventsChart').getContext('2d');
        const eventsChart = new Chart(eventsCtx, {
            type: 'line',
            data: {
                labels: data.labels,
                datasets: [{
                    label: 'Events Served',
                    data: data.eventsServed,
                    borderColor: '#3fb950',
                    backgroundColor: 'rgba(63, 185, 80, 0.1)',
                    fill: true,
                    tension: 0.3
                }]
            },
            options: chartOptions
        });

        function setAggregation(agg) {
            currentAggregation = agg;
            document.querySelectorAll('.aggregation-toggle .toggle-btn').forEach(btn => btn.classList.remove('active'));
            event.target.classList.add('active');

            data = getDataForAggregation(agg);
            const timeLabel = agg === 'hour' ? 'Hour' : 'Day';

            document.getElementById('reqsTitle').textContent = 'REQs per ' + timeLabel;
            document.getElementById('eventsTitle').textContent = 'Events Served per ' + timeLabel;

            reqsChart.data.labels = data.labels;
            eventsChart.data.labels = data.labels;

            updateREQsChart();
            updateEventsChart();
        }

        function updateREQsChart() {
            if (currentREQsView === 'total') {
                reqsChart.data.datasets[0].data = data.totalREQs;
                reqsChart.data.datasets[0].label = 'Total REQs';
            } else {
                reqsChart.data.datasets[0].data = data.uniqueIPs;
                reqsChart.data.datasets[0].label = 'Unique IPs';
            }
            reqsChart.update();
        }

        function updateEventsChart() {
            if (currentEventsView === 'total') {
                eventsChart.data.datasets[0].data = data.eventsServed;
                eventsChart.data.datasets[0].label = 'Events Served';
            } else {
                eventsChart.data.datasets[0].data = data.avgEventsPerREQ;
                eventsChart.data.datasets[0].label = 'Avg Events per REQ';
            }
            eventsChart.update();
        }

        function toggleREQsView(view) {
            currentREQsView = view;
            document.querySelectorAll('.chart-section:nth-child(3) .toggle-btn').forEach(btn => btn.classList.remove('active'));
            event.target.classList.add('active');
            updateREQsChart();
        }

        function toggleEventsView(view) {
            currentEventsView = view;
            document.querySelectorAll('.chart-section:nth-child(4) .toggle-btn').forEach(btn => btn.classList.remove('active'));
            event.target.classList.add('active');
            updateEventsChart();
        }
    </script>
</body>
</html>`

// DashboardHandler handles HTTP requests for the usage dashboard.
type DashboardHandler struct {
	storage *storage.Storage
}

// NewDashboardHandler creates a new dashboard handler with the given storage backend.
func NewDashboardHandler(storage *storage.Storage) *DashboardHandler {
	return &DashboardHandler{storage: storage}
}

// TopIPDisplay represents a single IP address entry in the top IPs table.
type TopIPDisplay struct {
	IP           string
	PTR          string
	TotalREQs    int64
	EventsServed int64
}

// DashboardData contains all data needed to render the dashboard template.
type DashboardData struct {
	TodayREQs         int64
	TodayUniqueIPs    int64
	TodayEventsServed int64
	DailyStatsJSON    template.JS
	HourlyStatsJSON   template.JS
	TopIPs            []TopIPDisplay
	StorageSize       string
	StorageGrowth     string
}

// HandleDashboard returns an HTTP handler function that renders the usage dashboard.
func (h *DashboardHandler) HandleDashboard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		todayStats, err := h.storage.GetTodayStats(ctx)
		if err != nil || todayStats == nil {
			todayStats = &storage.DailyStats{}
		}

		dailyStats, err := h.storage.GetDailyStats(ctx, 30)
		if err != nil {
			dailyStats = []storage.DailyStats{}
		}

		hourlyStats, err := h.storage.GetHourlyStats(ctx, 72) // Last 3 days of hourly data
		if err != nil {
			hourlyStats = []storage.HourlyStats{}
		}

		topIPs, err := h.storage.GetTopIPs(ctx, 20)
		if err != nil {
			topIPs = []storage.TopIP{}
		}

		topIPDisplays := make([]TopIPDisplay, len(topIPs))
		for i, ip := range topIPs {
			ptr := "—"
			names, err := net.LookupAddr(ip.IP)
			if err == nil && len(names) > 0 {
				ptr = names[0]
				if len(ptr) > 0 && ptr[len(ptr)-1] == '.' {
					ptr = ptr[:len(ptr)-1]
				}
			}
			topIPDisplays[i] = TopIPDisplay{
				IP:           ip.IP,
				PTR:          ptr,
				TotalREQs:    ip.TotalREQs,
				EventsServed: ip.EventsServed,
			}
		}

		dailyStatsJSON, _ := json.Marshal(dailyStats)
		hourlyStatsJSON, _ := json.Marshal(hourlyStats)

		// Fetch storage stats
		storageSize := "N/A"
		storageGrowth := "—"
		if currentStorage, err := h.storage.GetCurrentStorageInfo(ctx); err == nil && currentStorage != nil {
			storageSize = FormatBytes(currentStorage.EventTableBytes)

			// Get 30-day growth
			if growth, err := h.storage.GetStorageGrowth(ctx, 30); err == nil && growth != 0 {
				if growth > 0 {
					storageGrowth = fmt.Sprintf("+%.1f%%", growth)
				} else {
					storageGrowth = fmt.Sprintf("%.1f%%", growth)
				}
			}
		}

		data := DashboardData{
			TodayREQs:         todayStats.TotalREQs,
			TodayUniqueIPs:    todayStats.UniqueIPs,
			TodayEventsServed: todayStats.EventsServed,
			DailyStatsJSON:    template.JS(dailyStatsJSON),
			HourlyStatsJSON:   template.JS(hourlyStatsJSON),
			TopIPs:            topIPDisplays,
			StorageSize:       storageSize,
			StorageGrowth:     storageGrowth,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := dashboardTmpl.Execute(w, data); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}
