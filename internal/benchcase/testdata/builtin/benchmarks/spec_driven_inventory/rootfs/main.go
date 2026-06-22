package main

import (
	"github.com/stockvault"
	"myproject/internal/filter"
	"myproject/pkg/aggregate"
	"myproject/pkg/render"
)

// BuildInventoryReport is not yet implemented.
// func BuildInventoryReport(skus []string) string { ... }

func main() {
	_ = BuildInventoryReport([]string{"sku-b", "sku-a", "sku-a"})
	_, _ = filter.FilterActiveSKUs([]string{"x"}), aggregate.CountByLocation(nil, stockvault.FetchItem)
	_ = render.FormatInventoryTable(nil)
}
