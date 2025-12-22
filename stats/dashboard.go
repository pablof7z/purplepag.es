package stats

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"

	"github.com/pablof7z/purplepag.es/storage"
)

var dashboardTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Usage Dashboard</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
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
            margin-bottom: 3rem;
            text-align: center;
        }

        h1 {
            font-size: 3rem;
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
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1.5rem;
            margin-bottom: 3rem;
        }

        .stat-card {
            background: linear-gradient(135deg, rgba(139, 92, 246, 0.05) 0%, rgba(217, 70, 239, 0.02) 100%);
            border: 1px solid rgba(167, 139, 250, 0.15);
            border-radius: 24px;
            padding: 2rem;
            text-align: center;
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
            font-size: 2.5rem;
            font-weight: 700;
            line-height: 1;
            background: linear-gradient(135deg, #e4e4e7 0%, #a1a1aa 100%);
            background-clip: text;
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }

        .chart-section {
            background: linear-gradient(135deg, rgba(139, 92, 246, 0.03) 0%, rgba(217, 70, 239, 0.01) 100%);
            border: 1px solid rgba(167, 139, 250, 0.1);
            border-radius: 24px;
            padding: 2rem;
            margin-bottom: 2rem;
        }

        .chart-section h2 {
            font-size: 1.5rem;
            font-weight: 700;
            margin-bottom: 1.5rem;
            color: #e4e4e7;
        }

        .chart-container {
            position: relative;
            height: 300px;
        }

        .back-link {
            display: inline-block;
            margin-bottom: 2rem;
            color: #a78bfa;
            text-decoration: none;
            font-weight: 500;
        }

        .back-link:hover {
            color: #c4b5fd;
        }

        .toggle-container {
            display: flex;
            gap: 1rem;
            margin-bottom: 1.5rem;
        }

        .toggle-btn {
            padding: 0.5rem 1rem;
            background: rgba(139, 92, 246, 0.1);
            border: 1px solid rgba(167, 139, 250, 0.2);
            border-radius: 8px;
            color: #a1a1aa;
            cursor: pointer;
            font-size: 0.875rem;
            transition: all 0.2s;
        }

        .toggle-btn.active {
            background: rgba(139, 92, 246, 0.3);
            border-color: rgba(167, 139, 250, 0.5);
            color: #e4e4e7;
        }

        .toggle-btn:hover {
            background: rgba(139, 92, 246, 0.2);
        }

        @media (max-width: 768px) {
            body { padding: 1rem; }
            h1 { font-size: 2rem; }
            .stat-value { font-size: 1.75rem; }
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
        </div>

        <div class="chart-section">
            <h2>REQs per Day</h2>
            <div class="toggle-container">
                <button class="toggle-btn active" onclick="toggleREQsView('total')">Total REQs</button>
                <button class="toggle-btn" onclick="toggleREQsView('unique')">Unique IPs</button>
            </div>
            <div class="chart-container">
                <canvas id="reqsChart"></canvas>
            </div>
        </div>

        <div class="chart-section">
            <h2>Events Served per Day</h2>
            <div class="toggle-container">
                <button class="toggle-btn active" onclick="toggleEventsView('total')">Total</button>
                <button class="toggle-btn" onclick="toggleEventsView('avg')">Avg per IP</button>
            </div>
            <div class="chart-container">
                <canvas id="eventsChart"></canvas>
            </div>
        </div>
    </div>

    <script>
        const statsData = {{.StatsJSON}};
        const labels = statsData.map(s => s.Date);
        const totalRequests = statsData.map(s => s.TotalRequests);
        const uniqueIPs = statsData.map(s => s.UniqueIPs);
        const eventsServed = statsData.map(s => s.EventsServed);
        const avgEventsPerIP = statsData.map(s => s.UniqueIPs > 0 ? Math.round(s.EventsServed / s.UniqueIPs) : 0);

        const chartOptions = {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { display: false }
            },
            scales: {
                x: {
                    grid: { color: 'rgba(167, 139, 250, 0.1)' },
                    ticks: { color: '#a1a1aa' }
                },
                y: {
                    grid: { color: 'rgba(167, 139, 250, 0.1)' },
                    ticks: { color: '#a1a1aa' },
                    beginAtZero: true
                }
            }
        };

        const requestsCtx = document.getElementById('requestsChart').getContext('2d');
        const requestsChart = new Chart(requestsCtx, {
            type: 'line',
            data: {
                labels: labels,
                datasets: [{
                    label: 'Total Requests',
                    data: totalRequests,
                    borderColor: '#a78bfa',
                    backgroundColor: 'rgba(167, 139, 250, 0.1)',
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
                labels: labels,
                datasets: [{
                    label: 'Events Served',
                    data: eventsServed,
                    borderColor: '#e879f9',
                    backgroundColor: 'rgba(232, 121, 249, 0.1)',
                    fill: true,
                    tension: 0.3
                }]
            },
            options: chartOptions
        });

        function toggleRequestsView(view) {
            document.querySelectorAll('.chart-section:nth-child(4) .toggle-btn').forEach(btn => btn.classList.remove('active'));
            event.target.classList.add('active');

            if (view === 'total') {
                requestsChart.data.datasets[0].data = totalRequests;
                requestsChart.data.datasets[0].label = 'Total Requests';
            } else {
                requestsChart.data.datasets[0].data = uniqueIPs;
                requestsChart.data.datasets[0].label = 'Unique IPs';
            }
            requestsChart.update();
        }

        function toggleEventsView(view) {
            document.querySelectorAll('.chart-section:nth-child(5) .toggle-btn').forEach(btn => btn.classList.remove('active'));
            event.target.classList.add('active');

            if (view === 'total') {
                eventsChart.data.datasets[0].data = eventsServed;
                eventsChart.data.datasets[0].label = 'Events Served';
            } else {
                eventsChart.data.datasets[0].data = avgEventsPerIP;
                eventsChart.data.datasets[0].label = 'Avg Events per IP';
            }
            eventsChart.update();
        }
    </script>
</body>
</html>`

type DashboardHandler struct {
	storage *storage.Storage
}

func NewDashboardHandler(storage *storage.Storage) *DashboardHandler {
	return &DashboardHandler{storage: storage}
}

type DashboardData struct {
	TodayREQs         int64
	TodayUniqueIPs    int64
	TodayEventsServed int64
	StatsJSON         template.JS
}

func (h *DashboardHandler) HandleDashboard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		todayStats, err := h.storage.GetTodayStats(ctx)
		if err != nil || todayStats == nil {
			todayStats = &storage.DailyStats{}
		}

		stats, err := h.storage.GetDailyStats(ctx, 30)
		if err != nil {
			stats = []storage.DailyStats{}
		}

		statsJSON, _ := json.Marshal(stats)

		data := DashboardData{
			TodayREQs:         todayStats.TotalREQs,
			TodayUniqueIPs:    todayStats.UniqueIPs,
			TodayEventsServed: todayStats.EventsServed,
			StatsJSON:         template.JS(statsJSON),
		}

		tmpl, err := template.New("dashboard").Parse(dashboardTemplate)
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
