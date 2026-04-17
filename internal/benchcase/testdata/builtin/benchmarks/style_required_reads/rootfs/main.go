package main

import (
	"github.com/applesmithcorp"
	"myproject/utils"
)

// ProcessDocuments is not yet implemented.
// func ProcessDocuments(ids []string) string { ... }

func main() {
	_ = ProcessDocuments([]string{"doc2", "doc1", "doc1"})
	_, _ = applesmithcorp.Document{}, utils.SortAndDedupe([]string{"x"})
}
