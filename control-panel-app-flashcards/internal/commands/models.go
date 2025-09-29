package commands

type FlashcardSide struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Caption string `json:"caption,omitempty"`
}

type Asset struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Alt  string `json:"alt,omitempty"`
}

type Flashcard struct {
	ID      string        `json:"id"`
	Default bool          `json:"default,omitempty"`
	Front   FlashcardSide `json:"front"`
	Back    FlashcardSide `json:"back"`
	Assets  []Asset       `json:"assets"`
	Tags    []string      `json:"tags"`
}
