package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/abstract-tutoring/models"
	"github.com/abstract-tutoring/utils"
)

func LoadCardsJSON(accessToken string) (map[string]models.Flashcard, error) {
	supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
	apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")

	url := supabaseUrl + "/rest/v1/cards?select=id,front,back,assets,created_by,cards_tags(tags(id,name))"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", apiKey)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var cardList []struct {
		models.Flashcard
		CardsTags []struct {
			Tags models.Tag `json:"tags"`
		} `json:"cards_tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cardList); err != nil {
		return nil, err
	}

	cards := make(map[string]models.Flashcard)
	for _, wrapped := range cardList {
		card := wrapped.Flashcard
		for _, rel := range wrapped.CardsTags {
			card.Tags = append(card.Tags, rel.Tags)
		}
		cards[card.ID] = card
	}

	return cards, nil
}

func FetchFlashcard(cardID, accessToken string) (models.Flashcard, error) {
	url := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL") + "/rest/v1/cards?id=eq." + cardID + "&select=id,front,back,created_by"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("apikey", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode >= 300 {
		return models.Flashcard{}, fmt.Errorf("Fetch failed")
	}
	defer resp.Body.Close()

	var cards []models.Flashcard
	json.NewDecoder(resp.Body).Decode(&cards)
	if len(cards) == 0 {
		return models.Flashcard{}, nil
	}
	return cards[0], nil
}

func UpdateFlashcard(cardID, newFront, newBack, accessToken string) error {
	payload := map[string]interface{}{
		"front": map[string]string{
			"type":    "rich_text",
			"content": newFront,
		},
		"back": map[string]string{
			"type":    "rich_text",
			"content": newBack,
		},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")+"/rest/v1/cards?id=eq."+cardID, bytes.NewReader(body))
	req.Header.Set("apikey", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode >= 300 {
		return fmt.Errorf("update failed")
	}
	return nil
}

func CountDueCards(cards []models.StudentCard, now int64) models.CardDueStats {
	var stats models.CardDueStats
	for _, c := range cards {
		if c.Status == 1 || c.Status == 2 || c.Status == 3 {
			stats.InProgressDue++
		}
		if (c.Status == 4 || c.Status == 5 || c.Status == 6) && c.Due <= now {
			stats.ReviewDue++
		}
		if c.Status == 0 && c.Due <= now {
			stats.NewAvailable++
		}
	}
	return stats
}
