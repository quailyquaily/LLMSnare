package applesmithcorp

type Document struct {
	ID string
}

func FetchDocument(id string) Document {
	return Document{ID: id}
}
