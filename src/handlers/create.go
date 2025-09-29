package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/abstract-tutoring/services"
	"github.com/abstract-tutoring/utils"
)

// CreateCardPage renders the form for creating a new flashcard
func CreateCardPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tmpl, err := template.ParseFiles("./frontend/templates/create.html")
	if err != nil {
		log.Println("Template parse error:", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	// Always provide empty values for consistency
	tmpl.Execute(w, struct {
		Front        string
		Back         string
		ErrorMessage string
		Success      bool
	}{})
}

func CreateCardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	rawFront := r.FormValue("front")
	rawBack := r.FormValue("back")
	rawTags := r.FormValue("tags")

	front, err := services.SanitiseAndValidate(rawFront)
	if err != nil {
		renderCreateFormWithError(w, "No HTML Allowed (Front)", rawFront, rawBack)
		return
	}

	back, err := services.SanitiseAndValidate(rawBack)
	if err != nil {
		renderCreateFormWithError(w, "No HTML Allowed (Back)", rawFront, rawBack)
		return
	}

	tags := []string{}
	if rawTags != "" {
		for _, t := range strings.Split(rawTags, ",") {
			tClean, err := services.SanitiseAndValidate(strings.ToLower(strings.TrimSpace(t)))
			if err == nil && tClean != "" {
				tags = append(tags, tClean)
			}
		}
	}

	userCookie, err := r.Cookie("user_id")
	if err != nil || userCookie.Value == "" {
		http.Error(w, "User not logged in", http.StatusUnauthorized)
		return
	}
	userId := userCookie.Value

	accessCookie, err := r.Cookie("access_token")
	if err != nil || accessCookie.Value == "" {
		http.Error(w, "Missing access token", http.StatusUnauthorized)
		return
	}
	accessToken := accessCookie.Value

	studentId, err := fetchStudentIdByUserId(userId, accessToken)
	if err != nil || studentId == "" {
		log.Println("Error fetching student ID:", err)
		clearSessionCookies(w, r)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	cardID, err := generateSequentialCardID(userId, studentId, accessToken)
	if err != nil {
		log.Println("Card ID generation failed:", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	newCard := map[string]interface{}{
		"id":         cardID,
		"front":      map[string]string{"type": "rich_text", "content": front},
		"back":       map[string]string{"type": "rich_text", "content": back},
		"assets":     []interface{}{},
		"created_by": userId,
	}

	if err := insertFlashcard(accessToken, newCard); err != nil {
		log.Println("Flashcard insert failed:", err)
		http.Error(w, "Failed to create flashcard", http.StatusInternalServerError)
		return
	}

	if err := assignCardToStudent(studentId, cardID, accessToken); err != nil {
		log.Println("Card assignment failed:", err)
		http.Error(w, "Failed to assign card to student", http.StatusInternalServerError)
		return
	}

	// 🔗 Insert tag rows into Supabase
	for _, tag := range tags {
		tagID, err := services.UpsertTag(accessToken, tag)
		if err != nil {
			log.Println("Tag insert failed:", err)
			continue
		}
		err = services.LinkTagToCard(accessToken, cardID, tagID)
		if err != nil {
			log.Println("Failed to link tag to card:", err)
		}
	}

	// Success
	tmpl, err := template.ParseFiles("./frontend/templates/create.html")
	if err != nil {
		log.Println("Template parse error:", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	tmpl.Execute(w, struct {
		Front        string
		Back         string
		ErrorMessage string
		Success      bool
	}{
		Front:        "",
		Back:         "",
		ErrorMessage: "",
		Success:      true,
	})
}

// generateSequentialCardID produces a unique card ID like "card_harvey_000001"
func generateSequentialCardID(userId, studentId, accessToken string) (string, error) {
	url := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL") +
		"/rest/v1/cards?select=id&created_by=eq." + userId

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("request creation failed: %w", err)
	}
	req.Header.Set("apikey", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to fetch IDs: %s", string(body))
	}

	var result []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode failed: %w", err)
	}

	prefix := "card_" + studentId + "_"
	maxNum := 0

	for _, r := range result {
		if strings.HasPrefix(r.ID, prefix) {
			suffix := strings.TrimPrefix(r.ID, prefix)
			if n, err := strconv.Atoi(suffix); err == nil && n > maxNum {
				maxNum = n
			}
		}
	}

	newID := fmt.Sprintf("card_%s_%06d", studentId, maxNum+1)
	// newID := fmt.Sprintf("aveeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeerylongwordthatwouldntpossiblyfitononeline")
	return newID, nil
}

// insertFlashcard sends a new flashcard to Supabase
func insertFlashcard(accessToken string, card map[string]interface{}) error {
	url := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL") + "/rest/v1/cards"
	body, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("failed to marshal card: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create insert request: %w", err)
	}
	req.Header.Set("apikey", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("flashcard insert request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("flashcard insert failed: %s", string(msg))
	}

	return nil
}

// assignCardToStudent links a flashcard to a student
func assignCardToStudent(studentId, cardId, accessToken string) error {
	url := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL") + "/rest/v1/students_cards"
	body, err := json.Marshal(map[string]interface{}{
		"student_id": studentId,
		"card_id":    cardId,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal assignment: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create assignment request: %w", err)
	}
	req.Header.Set("apikey", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("assignment request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("assignment failed: %s", string(msg))
	}

	return nil
}

func renderCreateFormWithError(w http.ResponseWriter, msg, front, back string) {
	tmpl, err := template.ParseFiles("./frontend/templates/create.html")
	if err != nil {
		log.Println("Template parse error:", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	tmpl.Execute(w, struct {
		Front        string
		Back         string
		ErrorMessage string
		Success      bool
	}{
		Front:        front,
		Back:         back,
		ErrorMessage: msg,
		Success:      false,
	})
}
