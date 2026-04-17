package utils

import "strings"

func FormatItemsLine(ids []string) string {
	return "Items: " + strings.Join(ids, ", ")
}
