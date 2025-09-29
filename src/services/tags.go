package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/abstract-tutoring/models"
	"github.com/abstract-tutoring/utils"
)

func ClearTagsForCard(cardID, accessToken string) error {
	url := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL") + "/rest/v1/cards_tags?card_id=eq." + cardID

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to clear tags: %s", string(body))
	}

	return nil
}

func FetchTagsForCard(cardID, accessToken string) ([]models.Tag, error) {
	url := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL") +
		"/rest/v1/cards_tags?card_id=eq." + cardID + "&select=tags(id,name)"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var joined []struct {
		Tag struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"tags"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&joined); err != nil {
		return nil, err
	}

	tags := []models.Tag{}
	for _, j := range joined {
		tags = append(tags, j.Tag)
	}

	return tags, nil
}

func UpsertTag(accessToken, tagName string) (int, error) {
	// First, try to GET the tag
	queryURL := fmt.Sprintf("%s/rest/v1/tags?name=eq.%s&select=id", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL"), tagName)

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("apikey", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var found []struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&found); err == nil && len(found) > 0 {
		return found[0].ID, nil
	}

	// Not found — insert it
	postURL := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL") + "/rest/v1/tags?select=id"
	body := fmt.Sprintf(`[{"name": "%s"}]`, tagName)

	req, err = http.NewRequest("POST", postURL, bytes.NewBufferString(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("apikey", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var inserted []struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&inserted); err != nil || len(inserted) == 0 {
		return 0, fmt.Errorf("unable to insert or retrieve tag")
	}

	return inserted[0].ID, nil
}

func LinkTagToCard(accessToken, cardID string, tagID int) error {
	url := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL") + "/rest/v1/cards_tags"
	body := fmt.Sprintf(`[{"card_id": "%s", "tag_id": %d}]`, cardID, tagID)

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	req.Header.Set("apikey", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "resolution=ignore-duplicates")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("link insert failed: %s", string(msg))
	}

	return nil
}

func SortTagsAlphabetically(tags []string) []string {
	normalised := make([]string, len(tags))
	for i, tag := range tags {
		normalised[i] = strings.TrimSpace(strings.ToLower(tag))
	}
	sort.Strings(normalised)
	return normalised
}

// CardMatchesTags checks if the flashcard has at least one tag from the allowed set.
func CardMatchesTags(card models.Flashcard, allowed map[string]bool) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, tag := range card.Tags {
		if allowed[tag.Name] {
			return true
		}
	}
	return false
}

// FilterStudentCardsByTags filters student cards based on allowed tags.
func FilterStudentCardsByTags(
	cards []models.StudentCard,
	allCards map[string]models.Flashcard,
	allowed map[string]bool,
) []models.StudentCard {
	var filtered []models.StudentCard
	for _, sc := range cards {
		card, ok := allCards[sc.CardID]
		if !ok {
			continue
		}
		if CardMatchesTags(card, allowed) {
			filtered = append(filtered, sc)
		}
	}
	return filtered
}
