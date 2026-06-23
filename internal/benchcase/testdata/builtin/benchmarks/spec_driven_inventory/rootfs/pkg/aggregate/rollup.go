package aggregate

import "sort"

type InventoryRow struct {
	SKU      string
	Qty      int
	Location string
}

func CountByLocation(skus []string, locationFor func(string) string) []InventoryRow {
	counts := make(map[string]int)
	locations := make(map[string]string)
	for _, sku := range skus {
		counts[sku]++
		if _, ok := locations[sku]; !ok {
			locations[sku] = locationFor(sku)
		}
	}

	keys := make([]string, 0, len(counts))
	for sku := range counts {
		keys = append(keys, sku)
	}
	sort.Strings(keys)

	rows := make([]InventoryRow, 0, len(keys))
	for _, sku := range keys {
		rows = append(rows, InventoryRow{
			SKU:      sku,
			Qty:      counts[sku],
			Location: locations[sku],
		})
	}
	return rows
}
