package utils

import "strings"

func FormatDocumentSummary(ids []string) string {
	if len(ids) == 0 {
		return "Summary:\n- (no documents)"
	}

	var b strings.Builder
	b.WriteString("Summary:")
	for _, id := range ids {
		b.WriteString("\n- ")
		b.WriteString(id)
	}
	return b.String()
}
