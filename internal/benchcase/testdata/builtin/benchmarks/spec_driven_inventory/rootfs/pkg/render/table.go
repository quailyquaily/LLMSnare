package render

import (
	"fmt"
	"strings"

	"myproject/pkg/aggregate"
)

func FormatInventoryTable(rows []aggregate.InventoryRow) string {
	if len(rows) == 0 {
		return "Inventory Report\n(no items)"
	}

	var b strings.Builder
	b.WriteString("Inventory Report\n")
	b.WriteString("SKU | Qty | Location\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "%s | %d | %s\n", row.SKU, row.Qty, row.Location)
	}
	return strings.TrimSuffix(b.String(), "\n")
}
