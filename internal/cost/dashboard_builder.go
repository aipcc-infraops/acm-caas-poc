package cost

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	DashboardName    = "acmlab-cost-tracking"
	CostRulesGroup   = "acmlab-cost-estimation"
	CostMetricPrefix = "acmlab_cost"
)

func CostMetricsAllowlist() []string {
	return []string{
		"kube_node_status_capacity",
		"kube_node_info",
		"kube_node_labels",
	}
}

func CostRecordingRulesYAML(fallbackPricePerCPUHr float64) string {
	var sb strings.Builder
	sb.WriteString("groups:\n")
	sb.WriteString(fmt.Sprintf("- name: %s\n", CostRulesGroup))
	sb.WriteString("  interval: 5m\n")
	sb.WriteString("  rules:\n")

	sb.WriteString("  - record: acmlab_cost:cluster_cpu_cores:sum\n")
	sb.WriteString("    expr: |\n")
	sb.WriteString("      sum by (cluster) (\n")
	sb.WriteString("        kube_node_status_capacity{resource=\"cpu\",unit=\"core\"}\n")
	sb.WriteString("        * on(node,cluster) group_left()\n")
	sb.WriteString("        kube_node_labels{label_node_role_kubernetes_io_worker=\"\"}\n")
	sb.WriteString("      )\n")

	sb.WriteString("  - record: acmlab_cost:cluster_memory_gib:sum\n")
	sb.WriteString("    expr: |\n")
	sb.WriteString("      sum by (cluster) (\n")
	sb.WriteString("        kube_node_status_capacity{resource=\"memory\",unit=\"byte\"}\n")
	sb.WriteString("        * on(node,cluster) group_left()\n")
	sb.WriteString("        kube_node_labels{label_node_role_kubernetes_io_worker=\"\"}\n")
	sb.WriteString("      ) / 1073741824\n")

	sb.WriteString("  - record: acmlab_cost:cluster_worker_nodes:sum\n")
	sb.WriteString("    expr: |\n")
	sb.WriteString("      count by (cluster) (\n")
	sb.WriteString("        kube_node_labels{label_node_role_kubernetes_io_worker=\"\"}\n")
	sb.WriteString("      )\n")

	sb.WriteString("  - record: acmlab_cost:cluster_daily_estimate:sum\n")
	sb.WriteString("    expr: |\n")
	sb.WriteString(fmt.Sprintf("      acmlab_cost:cluster_cpu_cores:sum * %.3f * 24\n", fallbackPricePerCPUHr))

	sb.WriteString("  - record: acmlab_cost:cluster_monthly_estimate:sum\n")
	sb.WriteString("    expr: |\n")
	sb.WriteString("      acmlab_cost:cluster_daily_estimate:sum * 30\n")

	return sb.String()
}

func CostDashboardJSON() string {
	dashboard := map[string]interface{}{
		"annotations": map[string]interface{}{
			"list": []interface{}{},
		},
		"editable":     true,
		"graphTooltip": 1,
		"id":           nil,
		"links":        []interface{}{},
		"panels":       costDashboardPanels(),
		"schemaVersion": 39,
		"tags":          []interface{}{"acmlab", "cost", "fleet"},
		"templating": map[string]interface{}{
			"list": []interface{}{},
		},
		"time": map[string]interface{}{
			"from": "now-24h",
			"to":   "now",
		},
		"title": "ACMlab Fleet Cost Tracking",
		"uid":   "acmlab-cost-tracking",
	}
	data, _ := json.MarshalIndent(dashboard, "", "  ")
	return string(data)
}

func costDashboardPanels() []interface{} {
	return []interface{}{
		statPanel(1, "Fleet Monthly Estimate", 0, 0, 6, 4,
			"sum(acmlab_cost:cluster_monthly_estimate:sum)",
			"$", "currencyUSD"),
		statPanel(2, "Fleet Daily Estimate", 6, 0, 6, 4,
			"sum(acmlab_cost:cluster_daily_estimate:sum)",
			"$", "currencyUSD"),
		statPanel(3, "Total Worker Nodes", 12, 0, 6, 4,
			"sum(acmlab_cost:cluster_worker_nodes:sum)",
			"", "short"),
		statPanel(4, "Total CPU Cores", 18, 0, 6, 4,
			"sum(acmlab_cost:cluster_cpu_cores:sum)",
			"", "short"),
		tablePanel(5, "Cost per Cluster", 0, 4, 24, 8, []tableQuery{
			{expr: "acmlab_cost:cluster_monthly_estimate:sum", legend: "Monthly $"},
			{expr: "acmlab_cost:cluster_daily_estimate:sum", legend: "Daily $"},
			{expr: "acmlab_cost:cluster_worker_nodes:sum", legend: "Nodes"},
			{expr: "acmlab_cost:cluster_cpu_cores:sum", legend: "CPU Cores"},
			{expr: "acmlab_cost:cluster_memory_gib:sum", legend: "Memory GiB"},
		}),
		timeseriesPanel(6, "Daily Cost Trend per Cluster", 0, 12, 24, 8,
			"acmlab_cost:cluster_daily_estimate:sum", "{{cluster}}"),
		barGaugePanel(7, "Monthly Estimate by Cluster", 0, 20, 24, 6,
			"acmlab_cost:cluster_monthly_estimate:sum"),
	}
}

func statPanel(id int, title string, x, y, w, h int, expr, prefix, unit string) map[string]interface{} {
	p := map[string]interface{}{
		"id":    id,
		"type":  "stat",
		"title": title,
		"gridPos": map[string]interface{}{
			"h": h, "w": w, "x": x, "y": y,
		},
		"targets": []interface{}{
			map[string]interface{}{
				"expr":    expr,
				"refId":   "A",
				"instant": true,
			},
		},
		"fieldConfig": map[string]interface{}{
			"defaults": map[string]interface{}{
				"unit": unit,
			},
		},
	}
	if prefix != "" {
		p["options"] = map[string]interface{}{
			"textMode":    "value",
			"graphMode":   "none",
			"reduceOptions": map[string]interface{}{
				"calcs": []interface{}{"lastNotNull"},
			},
		}
	}
	return p
}

type tableQuery struct {
	expr   string
	legend string
}

func tablePanel(id int, title string, x, y, w, h int, queries []tableQuery) map[string]interface{} {
	targets := make([]interface{}, 0, len(queries))
	for i, q := range queries {
		targets = append(targets, map[string]interface{}{
			"expr":    q.expr,
			"refId":   string(rune('A' + i)),
			"instant": true,
			"legendFormat": q.legend,
			"format":  "table",
		})
	}
	return map[string]interface{}{
		"id":    id,
		"type":  "table",
		"title": title,
		"gridPos": map[string]interface{}{
			"h": h, "w": w, "x": x, "y": y,
		},
		"targets": targets,
		"transformations": []interface{}{
			map[string]interface{}{
				"id": "merge",
			},
		},
	}
}

func timeseriesPanel(id int, title string, x, y, w, h int, expr, legend string) map[string]interface{} {
	return map[string]interface{}{
		"id":    id,
		"type":  "timeseries",
		"title": title,
		"gridPos": map[string]interface{}{
			"h": h, "w": w, "x": x, "y": y,
		},
		"targets": []interface{}{
			map[string]interface{}{
				"expr":         expr,
				"refId":        "A",
				"legendFormat": legend,
			},
		},
		"fieldConfig": map[string]interface{}{
			"defaults": map[string]interface{}{
				"unit":  "currencyUSD",
				"custom": map[string]interface{}{
					"fillOpacity": 10,
					"lineWidth":   2,
				},
			},
		},
	}
}

func barGaugePanel(id int, title string, x, y, w, h int, expr string) map[string]interface{} {
	return map[string]interface{}{
		"id":    id,
		"type":  "bargauge",
		"title": title,
		"gridPos": map[string]interface{}{
			"h": h, "w": w, "x": x, "y": y,
		},
		"targets": []interface{}{
			map[string]interface{}{
				"expr":         expr,
				"refId":        "A",
				"instant":      true,
				"legendFormat": "{{cluster}}",
			},
		},
		"fieldConfig": map[string]interface{}{
			"defaults": map[string]interface{}{
				"unit": "currencyUSD",
			},
		},
		"options": map[string]interface{}{
			"orientation":   "horizontal",
			"displayMode":   "gradient",
			"reduceOptions": map[string]interface{}{
				"calcs": []interface{}{"lastNotNull"},
			},
		},
	}
}
