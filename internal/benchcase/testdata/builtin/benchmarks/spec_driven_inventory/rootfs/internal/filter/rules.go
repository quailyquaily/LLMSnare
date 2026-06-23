package filter

func FilterActiveSKUs(skus []string) []string {
	out := make([]string, 0, len(skus))
	for _, sku := range skus {
		if sku == "" {
			continue
		}
		if len(sku) >= 9 && sku[:9] == "inactive-" {
			continue
		}
		out = append(out, sku)
	}
	return out
}
