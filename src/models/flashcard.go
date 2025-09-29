package models

type Asset struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Alt  string `json:"alt"`
}

type Flashcard struct {
	ID        string  `json:"id"`
	Front     Content `json:"front"`
	Back      Content `json:"back"`
	Assets    []Asset `json:"assets"`
	CreatedBy string  `json:"created_by"`
	Tags      []Tag   `json:"tags"`
}

type Content struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type FlashcardTag struct {
	CardID string `json:"card_id"`
	TagID  int    `json:"tag_id"`
}

type StudentCard struct {
	CardID string `json:"card_id"`
	Status int    `json:"status"`
	Due    int64  `json:"due"`
}

type CardDueStats struct {
	InProgressDue int
	ReviewDue     int
	NewAvailable  int
}

const MaxNewCardsPerDay = "20"
