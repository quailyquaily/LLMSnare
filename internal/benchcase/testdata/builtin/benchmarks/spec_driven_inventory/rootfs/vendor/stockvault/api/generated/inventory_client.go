package stockvault

func FetchItem(sku string) Item {
	locations := map[string]string{
		"sku-a": "shelf-a",
		"sku-b": "shelf-b",
	}
	location := locations[sku]
	if location == "" {
		location = "unknown"
	}
	return Item{SKU: sku, Location: location}
}
