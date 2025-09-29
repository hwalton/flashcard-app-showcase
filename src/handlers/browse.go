package handlers

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/abstract-tutoring/services"
	"github.com/abstract-tutoring/utils"
)

func ServeBrowsePage(w http.ResponseWriter, r *http.Request) {
	userCookie, err := r.Cookie("user_id")
	if err != nil || userCookie.Value == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	userId := userCookie.Value

	accessTokenCookie, err := r.Cookie("access_token")
	if err != nil || accessTokenCookie.Value == "" {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	accessToken := accessTokenCookie.Value

	supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
	apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")

	studentId, err := fetchStudentId(r, userId, supabaseUrl, apiKey)
	if err != nil || studentId == "" {
		// No mapping: show browse page with empty cards, but keep search bar etc.
		data := struct {
			StudentID  string
			Flashcards []interface{}
			Query      string
			HasMore    bool
			NextOffset int
		}{
			StudentID:  "",
			Flashcards: []interface{}{},
			Query:      "",
			HasMore:    false,
			NextOffset: 0,
		}

		tmpl, err := template.ParseFiles(
			"./frontend/templates/browse.html",
			"./frontend/templates/partials/flashcard-item.html",
		)
		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, data)
		return
	}

	studentCards, err := fetchStudentCards(w, r, studentId, supabaseUrl, apiKey)
	if err != nil {
		http.Error(w, "Could not load student cards", http.StatusInternalServerError)
		return
	}

	allCards, err := services.LoadCardsJSON(accessToken)
	if err != nil {
		http.Error(w, "Could not load card content", http.StatusInternalServerError)
		return
	}

	query := strings.ToLower(r.URL.Query().Get("query"))
	offsetStr := r.URL.Query().Get("offset")
	offset, _ := strconv.Atoi(offsetStr)
	pageSize := 25

	type FlashcardPreview struct {
		ID         string
		Front      string
		Back       string
		IsOwner    bool
		Tags       []string
		Status     int
		StatusText string // ← added
	}

	var filtered []FlashcardPreview
	for _, c := range studentCards {
		content, ok := allCards[c.CardID]
		if !ok {
			continue
		}

		preview := FlashcardPreview{
			ID:         c.CardID,
			Front:      content.Front.Content,
			Back:       content.Back.Content,
			IsOwner:    content.CreatedBy == userId,
			Status:     c.Status,
			StatusText: statusToText(c.Status), // ← added
		}

		var tags []string
		for _, tag := range content.Tags {
			tags = append(tags, tag.Name)
		}
		preview.Tags = services.SortTagsAlphabetically(tags)

		// Match query if present
		if query == "" ||
			strings.Contains(strings.ToLower(preview.ID), query) ||
			strings.Contains(strings.ToLower(preview.Front), query) ||
			strings.Contains(strings.ToLower(preview.Back), query) ||
			tagMatch(preview.Tags, query) ||
			strings.Contains(strings.ToLower(preview.StatusText), query) { // ← added
			filtered = append(filtered, preview)
		}
	}

	end := offset + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	hasMore := end < len(filtered)

	data := struct {
		StudentID  string
		Flashcards []FlashcardPreview
		Query      string
		HasMore    bool
		NextOffset int
	}{
		StudentID:  studentId,
		Flashcards: filtered[offset:end],
		Query:      query,
		HasMore:    hasMore,
		NextOffset: end,
	}

	if r.Header.Get("HX-Request") != "" {
		tmpl, err := template.ParseFiles(
			"./frontend/templates/partials/browse-more.html",
			"./frontend/templates/partials/flashcard-item.html",
		)
		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, data)
		return
	}

	tmpl, err := template.ParseFiles(
		"./frontend/templates/browse.html",
		"./frontend/templates/partials/flashcard-item.html",
	)

	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, data)
}

func UnlinkCardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	cardID := r.FormValue("card_id")
	if cardID == "" {
		http.Error(w, "Missing card ID", http.StatusBadRequest)
		return
	}

	userId, err := getCookieValue(r, "user_id")
	if err != nil {
		http.Error(w, "Not logged in", http.StatusUnauthorized)
		return
	}

	accessToken, err := r.Cookie("access_token")
	if err != nil {
		http.Error(w, "No access token", http.StatusUnauthorized)
		return
	}

	studentId, err := fetchStudentId(r, userId, utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL"), utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	if err != nil || studentId == "" {
		http.Error(w, "Failed to resolve student ID", http.StatusInternalServerError)
		return
	}

	req, _ := http.NewRequest("DELETE",
		utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")+"/rest/v1/students_cards?student_id=eq."+studentId+"&card_id=eq."+cardID,
		nil)
	req.Header.Set("apikey", utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Authorization", "Bearer "+accessToken.Value)

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode >= 300 {
		http.Error(w, "Failed to unlink card", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/browse", http.StatusSeeOther)
}

func ServeConfirmDeleteButton(w http.ResponseWriter, r *http.Request) {
	cardID := r.URL.Query().Get("card_id")
	if cardID == "" {
		http.Error(w, "Missing card_id", http.StatusBadRequest)
		return
	}

	tmpl, err := template.ParseFiles("./frontend/templates/partials/confirm-delete-button-compact.html")
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	data := struct {
		CardID string
	}{
		CardID: cardID,
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, data)
}

func tagMatch(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

func statusToText(status int) string {
	switch status {
	case 0:
		return "new"
	case 1, 2, 3:
		return "in progress"
	case 4, 5, 6:
		return "consolidating"
	default:
		return "unknown"
	}
}
