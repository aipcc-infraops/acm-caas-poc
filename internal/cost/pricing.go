package cost

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type InstancePricing struct {
	Type       string  `json:"type"`
	CPUCores   int     `json:"cpuCores"`
	MemoryGiB  int     `json:"memoryGiB"`
	PricePerHr float64 `json:"pricePerHr"`
}

type nodeData struct {
	InstanceType string
	CPU          string
	Memory       string
}

var DefaultPricing = map[string]InstancePricing{
	"bx2-4x16":       {Type: "bx2-4x16", CPUCores: 4, MemoryGiB: 16, PricePerHr: 0.192},
	"bx2-8x32":       {Type: "bx2-8x32", CPUCores: 8, MemoryGiB: 32, PricePerHr: 0.384},
	"bx2-16x64":      {Type: "bx2-16x64", CPUCores: 16, MemoryGiB: 64, PricePerHr: 0.768},
	"m5.xlarge":       {Type: "m5.xlarge", CPUCores: 4, MemoryGiB: 16, PricePerHr: 0.192},
	"m5.2xlarge":      {Type: "m5.2xlarge", CPUCores: 8, MemoryGiB: 32, PricePerHr: 0.384},
	"m5.4xlarge":      {Type: "m5.4xlarge", CPUCores: 16, MemoryGiB: 64, PricePerHr: 0.768},
	"n2-standard-4":   {Type: "n2-standard-4", CPUCores: 4, MemoryGiB: 16, PricePerHr: 0.194},
	"n2-standard-8":   {Type: "n2-standard-8", CPUCores: 8, MemoryGiB: 32, PricePerHr: 0.388},
	"Standard_D4s_v3": {Type: "Standard_D4s_v3", CPUCores: 4, MemoryGiB: 16, PricePerHr: 0.192},
	"Standard_D8s_v3": {Type: "Standard_D8s_v3", CPUCores: 8, MemoryGiB: 32, PricePerHr: 0.384},
}

var FallbackPricePerCPUHr = 0.048

func calculateClusterCost(name string, nodes []nodeData, days int) ClusterCost {
	hours := float64(days) * 24
	var totalCPU int
	var totalMemGiB float64
	var estimatedCost float64
	var primaryType string

	for _, n := range nodes {
		cpu := parseCPU(n.CPU)
		totalCPU += cpu
		totalMemGiB += parseMemoryGiB(n.Memory)

		if p, ok := DefaultPricing[n.InstanceType]; ok {
			estimatedCost += p.PricePerHr * hours
			if primaryType == "" {
				primaryType = n.InstanceType
			}
		} else {
			estimatedCost += float64(cpu) * FallbackPricePerCPUHr * hours
		}
	}

	return ClusterCost{
		Name:           name,
		Nodes:          len(nodes),
		CPUCores:       totalCPU,
		MemoryGiB:      totalMemGiB,
		InstanceType:   primaryType,
		DailyEstimate:  estimatedCost / float64(days),
		PeriodEstimate: estimatedCost,
		Days:           days,
	}
}

func calculateTenantCost(cluster, namespace string, cpuHours, memGiBHours float64, days int) TenantCost {
	return TenantCost{
		Cluster:        cluster,
		Namespace:      namespace,
		CPUHours:       cpuHours,
		MemoryGiBHours: memGiBHours,
		Days:           days,
	}
}

func formatCostCSV(report CostReport) string {
	var sb strings.Builder
	sb.WriteString("name,nodes,cpuCores,memoryGiB,dailyEstimate,periodEstimate,days\n")
	for _, c := range report.Clusters {
		sb.WriteString(fmt.Sprintf("%s,%d,%d,%.1f,%.2f,%.2f,%d\n",
			c.Name, c.Nodes, c.CPUCores, c.MemoryGiB, c.DailyEstimate, c.PeriodEstimate, c.Days))
	}
	sb.WriteString(fmt.Sprintf("TOTAL,,,,%.2f,%.2f,%d\n", report.TotalCost/float64(report.Days), report.TotalCost, report.Days))
	return sb.String()
}

func formatCostJSON(report CostReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

func parseCPU(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func parseMemoryGiB(memKi string) float64 {
	s := strings.TrimSuffix(memKi, "Ki")
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val / (1024 * 1024)
}
