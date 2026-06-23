package filter

import "strings"

func FilterSKUs(skus []string) []string {
	out := make([]string, 0, len(skus))
	for _, sku := range skus {
		if strings.TrimSpace(sku) == "" {
			continue
		}
		out = append(out, sku)
	}
	return out
}
