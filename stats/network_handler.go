package stats

import (
	"context"
	"html/template"
	"net/http"

	"github.com/pablof7z/purplepag.es/storage"
)

type NetworkHandler struct {
	storage *storage.Storage
}

func NewNetworkHandler(store *storage.Storage) *NetworkHandler {
	return &NetworkHandler{storage: store}
}

type NetworkPageData struct {
	HourlyStats []storage.HourlyStats
	DailyStats  []storage.DailyStats
}

func (h *NetworkHandler) HandleNetwork() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		hourlyStats, _ := h.storage.GetHourlyStats(ctx, 48)
		dailyStats, _ := h.storage.GetDailyStats(ctx, 30)

		data := NetworkPageData{
			HourlyStats: hourlyStats,
			DailyStats:  dailyStats,
		}

		tmpl, err := template.New("network").Parse(networkTemplate)
		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

var networkTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Network Connections</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }

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

        @keyframes shimmer {
            0%, 100% { background-position: 0% 50%; }
            50% { background-position: 100% 50%; }
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

        .subtitle {
            font-size: 1rem;
            font-weight: 500;
            color: #a1a1aa;
            text-transform: uppercase;
            letter-spacing: 0.15em;
        }

        .back-link {
            display: inline-block;
            margin-bottom: 2rem;
            color: #a78bfa;
            text-decoration: none;
            font-weight: 500;
            transition: color 0.2s;
        }

        .back-link:hover { color: #c4b5fd; }

        .section {
            background: linear-gradient(135deg, rgba(139, 92, 246, 0.03) 0%, rgba(217, 70, 239, 0.01) 100%);
            border: 1px solid rgba(167, 139, 250, 0.1);
            border-radius: 24px;
            padding: 2rem;
            margin-bottom: 2rem;
        }

        .section h2 {
            font-size: 1.25rem;
            font-weight: 700;
            margin-bottom: 0.5rem;
            color: #e4e4e7;
        }

        .section .description {
            font-size: 0.875rem;
            color: #a1a1aa;
            margin-bottom: 1.5rem;
        }

        .chart-container {
            width: 100%;
            height: 400px;
            background: rgba(10, 10, 15, 0.5);
            border-radius: 16px;
            overflow: hidden;
        }

        .legend {
            display: flex;
            justify-content: center;
            gap: 2rem;
            margin-top: 1rem;
            flex-wrap: wrap;
        }

        .legend-item {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            font-size: 0.875rem;
            color: #a1a1aa;
        }

        .legend-color {
            width: 20px;
            height: 3px;
            border-radius: 2px;
        }

        .tooltip {
            position: absolute;
            background: rgba(10, 10, 15, 0.95);
            border: 1px solid rgba(167, 139, 250, 0.3);
            border-radius: 8px;
            padding: 0.75rem;
            font-size: 0.75rem;
            pointer-events: none;
            opacity: 0;
            transition: opacity 0.2s;
            z-index: 100;
        }

        .tooltip-date {
            font-weight: 700;
            color: #e4e4e7;
            margin-bottom: 0.5rem;
        }

        .tooltip-row {
            display: flex;
            justify-content: space-between;
            gap: 1rem;
            margin-bottom: 0.25rem;
        }

        .tooltip-label {
            color: #a1a1aa;
        }

        .tooltip-value {
            font-weight: 600;
            font-variant-numeric: tabular-nums;
        }

        @media (max-width: 768px) {
            body { padding: 1.5rem; }
            h1 { font-size: 2rem; }
            .chart-container { height: 300px; }
        }
    </style>
</head>
<body>
    <div class="container">
        <a href="/stats" class="back-link">← Back to Stats</a>
        <header>
            <h1>purplepag.es</h1>
            <div class="subtitle">Network Connections Dashboard</div>
        </header>

        <div class="section">
            <h2>Hourly Metrics (Last 48 Hours)</h2>
            <div class="description">Recent network activity with hourly granularity</div>
            <div id="hourly-chart" class="chart-container"></div>
            <div class="legend">
                <div class="legend-item">
                    <div class="legend-color" style="background: #a78bfa;"></div>
                    <span>Unique IPs</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color" style="background: #e879f9;"></div>
                    <span>Total Requests</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color" style="background: #60a5fa;"></div>
                    <span>Events Served</span>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Daily Metrics (Last 30 Days)</h2>
            <div class="description">Historical trends with daily aggregation</div>
            <div id="daily-chart" class="chart-container"></div>
            <div class="legend">
                <div class="legend-item">
                    <div class="legend-color" style="background: #a78bfa;"></div>
                    <span>Unique IPs</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color" style="background: #e879f9;"></div>
                    <span>Total Requests</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color" style="background: #60a5fa;"></div>
                    <span>Events Served</span>
                </div>
            </div>
        </div>
    </div>

    <script src="https://d3js.org/d3.v7.min.js"></script>
    <script>
        const hourlyData = [
            {{range .HourlyStats}}
            {time: "{{.Hour}}", uniqueIPs: {{.UniqueIPs}}, totalREQs: {{.TotalREQs}}, eventsServed: {{.EventsServed}}},
            {{end}}
        ];

        const dailyData = [
            {{range .DailyStats}}
            {time: "{{.Date}}", uniqueIPs: {{.UniqueIPs}}, totalREQs: {{.TotalREQs}}, eventsServed: {{.EventsServed}}},
            {{end}}
        ];

        function createChart(containerId, data, isHourly) {
            const container = document.getElementById(containerId);
            const margin = {top: 20, right: 20, bottom: 40, left: 60};
            const width = container.clientWidth - margin.left - margin.right;
            const height = container.clientHeight - margin.top - margin.bottom;

            const svg = d3.select('#' + containerId)
                .append('svg')
                .attr('width', container.clientWidth)
                .attr('height', container.clientHeight)
                .append('g')
                .attr('transform', 'translate(' + margin.left + ',' + margin.top + ')');

            data.forEach(d => {
                d.parsedTime = isHourly ? new Date(d.time.replace(' ', 'T') + ':00:00') : new Date(d.time);
            });

            const x = d3.scaleTime()
                .domain(d3.extent(data, d => d.parsedTime))
                .range([0, width]);

            const maxUniqueIPs = d3.max(data, d => d.uniqueIPs);
            const maxTotalREQs = d3.max(data, d => d.totalREQs);
            const maxEventsServed = d3.max(data, d => d.eventsServed);
            const globalMax = Math.max(maxUniqueIPs, maxTotalREQs, maxEventsServed);

            const y = d3.scaleLinear()
                .domain([0, globalMax * 1.1])
                .range([height, 0]);

            svg.append('g')
                .attr('transform', 'translate(0,' + height + ')')
                .call(d3.axisBottom(x).ticks(isHourly ? 8 : 10))
                .selectAll('text')
                .style('fill', '#a1a1aa')
                .style('font-size', '11px');

            svg.append('g')
                .call(d3.axisLeft(y).ticks(6))
                .selectAll('text')
                .style('fill', '#a1a1aa')
                .style('font-size', '11px');

            svg.selectAll('.domain, .tick line')
                .style('stroke', 'rgba(167, 139, 250, 0.2)');

            const lineUniqueIPs = d3.line()
                .x(d => x(d.parsedTime))
                .y(d => y(d.uniqueIPs))
                .curve(d3.curveMonotoneX);

            const lineTotalREQs = d3.line()
                .x(d => x(d.parsedTime))
                .y(d => y(d.totalREQs))
                .curve(d3.curveMonotoneX);

            const lineEventsServed = d3.line()
                .x(d => x(d.parsedTime))
                .y(d => y(d.eventsServed))
                .curve(d3.curveMonotoneX);

            svg.append('path')
                .datum(data)
                .attr('fill', 'none')
                .attr('stroke', '#a78bfa')
                .attr('stroke-width', 2)
                .attr('d', lineUniqueIPs);

            svg.append('path')
                .datum(data)
                .attr('fill', 'none')
                .attr('stroke', '#e879f9')
                .attr('stroke-width', 2)
                .attr('d', lineTotalREQs);

            svg.append('path')
                .datum(data)
                .attr('fill', 'none')
                .attr('stroke', '#60a5fa')
                .attr('stroke-width', 2)
                .attr('d', lineEventsServed);

            const tooltip = d3.select('body')
                .append('div')
                .attr('class', 'tooltip');

            const focus = svg.append('g')
                .style('display', 'none');

            focus.append('circle')
                .attr('r', 4)
                .attr('fill', '#a78bfa')
                .attr('stroke', '#e4e4e7')
                .attr('stroke-width', 2);

            const bisect = d3.bisector(d => d.parsedTime).left;

            svg.append('rect')
                .attr('width', width)
                .attr('height', height)
                .style('fill', 'none')
                .style('pointer-events', 'all')
                .on('mouseover', () => {
                    focus.style('display', null);
                    tooltip.style('opacity', 1);
                })
                .on('mouseout', () => {
                    focus.style('display', 'none');
                    tooltip.style('opacity', 0);
                })
                .on('mousemove', function(event) {
                    const x0 = x.invert(d3.pointer(event)[0]);
                    const i = bisect(data, x0, 1);
                    const d0 = data[i - 1];
                    const d1 = data[i];
                    const d = d1 && (x0 - d0.parsedTime > d1.parsedTime - x0) ? d1 : d0;

                    if (d) {
                        focus.attr('transform', 'translate(' + x(d.parsedTime) + ',' + y(d.uniqueIPs) + ')');

                        const formatTime = isHourly ?
                            d3.timeFormat('%Y-%m-%d %H:00') :
                            d3.timeFormat('%Y-%m-%d');

                        tooltip
                            .html(
                                '<div class="tooltip-date">' + formatTime(d.parsedTime) + '</div>' +
                                '<div class="tooltip-row"><span class="tooltip-label">Unique IPs:</span><span class="tooltip-value" style="color:#a78bfa">' + d.uniqueIPs.toLocaleString() + '</span></div>' +
                                '<div class="tooltip-row"><span class="tooltip-label">Total Requests:</span><span class="tooltip-value" style="color:#e879f9">' + d.totalREQs.toLocaleString() + '</span></div>' +
                                '<div class="tooltip-row"><span class="tooltip-label">Events Served:</span><span class="tooltip-value" style="color:#60a5fa">' + d.eventsServed.toLocaleString() + '</span></div>'
                            )
                            .style('left', (event.pageX + 15) + 'px')
                            .style('top', (event.pageY - 15) + 'px');
                    }
                });
        }

        if (hourlyData.length > 0) {
            createChart('hourly-chart', hourlyData, true);
        }

        if (dailyData.length > 0) {
            createChart('daily-chart', dailyData, false);
        }
    </script>
</body>
</html>`
