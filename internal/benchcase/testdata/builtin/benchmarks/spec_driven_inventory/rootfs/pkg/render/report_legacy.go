package render

import (
	"fmt"
	"strings"

	"myproject/pkg/aggregate"
)

func FormatOneLineReport(rows []aggregate.InventoryRow) string {
	if len(rows) == 0 {
		return ""
	}

	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, fmt.Sprintf("%s Qty=%d @ %s", row.SKU, row.Qty, row.Location))
	}
	return "One-line inventory: " + strings.Join(parts, "; ")
}
