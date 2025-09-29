package handlers

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/abstract-tutoring/services"
)

func EditCardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	cardID := r.FormValue("card_id")
	rawFront := r.FormValue("front")
	rawBack := r.FormValue("back")
	rawTags := r.FormValue("tags")

	userID, err := getCookieValue(r, "user_id")
	if err != nil {
		http.Error(w, "Not logged in", http.StatusUnauthorized)
		return
	}
	accessToken, err := r.Cookie("access_token")
	if err != nil {
		http.Error(w, "No access token", http.StatusUnauthorized)
		return
	}

	card, err := services.FetchFlashcard(cardID, accessToken.Value)
	if err != nil || card.ID == "" {
		http.Error(w, "Card not found", http.StatusNotFound)
		return
	}
	if card.CreatedBy != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Sanitize front/back
	sanitisedFront, err := services.SanitiseAndValidate(rawFront)
	if err != nil {
		renderEditFormWithError(w, cardID, rawFront, rawBack, "No HTML Allowed (Front)")
		return
	}
	sanitisedBack, err := services.SanitiseAndValidate(rawBack)
	if err != nil {
		renderEditFormWithError(w, cardID, rawFront, rawBack, "No HTML Allowed (Back)")
		return
	}

	// Update card content
	err = services.UpdateFlashcard(cardID, sanitisedFront, sanitisedBack, accessToken.Value)
	if err != nil {
		http.Error(w, "Failed to update", http.StatusInternalServerError)
		return
	}

	// Parse and sanitise tags
	tags := []string{}
	if rawTags != "" {
		for _, t := range strings.Split(rawTags, ",") {
			tClean, err := services.SanitiseAndValidate(strings.ToLower(strings.TrimSpace(t)))
			if err == nil && tClean != "" {
				tags = append(tags, tClean)
			}
		}
	}

	// Remove old tag links
	err = services.ClearTagsForCard(cardID, accessToken.Value)
	if err != nil {
		http.Error(w, "Failed to clear old tags", http.StatusInternalServerError)
		return
	}

	// Re-link new tags
	for _, tag := range tags {
		tagID, err := services.UpsertTag(accessToken.Value, tag)
		if err != nil {
			http.Error(w, "Tag upsert failed", http.StatusInternalServerError)
			return
		}
		err = services.LinkTagToCard(accessToken.Value, cardID, tagID)
		if err != nil {
			http.Error(w, "Tag link failed", http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/edit?card_id="+cardID, http.StatusSeeOther)
}

func EditCardPage(w http.ResponseWriter, r *http.Request) {
	cardID := r.URL.Query().Get("card_id")
	if cardID == "" {
		http.Error(w, "Missing card_id", http.StatusBadRequest)
		return
	}

	userID, err := getCookieValue(r, "user_id")
	if err != nil {
		http.Error(w, "Not logged in", http.StatusUnauthorized)
		return
	}

	accessToken, err := r.Cookie("access_token")
	if err != nil {
		http.Error(w, "No access token", http.StatusUnauthorized)
		return
	}

	// Fetch card content from Supabase
	card, err := services.FetchFlashcard(cardID, accessToken.Value)
	if err != nil || card.ID == "" {
		http.Error(w, "Card not found", http.StatusNotFound)
		return
	}

	// Restrict editing to owner and non-official cards
	if card.CreatedBy != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Fetch associated tags
	tags, err := services.FetchTagsForCard(cardID, accessToken.Value)
	if err != nil {
		http.Error(w, "Failed to fetch tags", http.StatusInternalServerError)
		return
	}

	// Turn tags into comma-separated string
	tagNames := []string{}
	for _, tag := range tags {
		tagNames = append(tagNames, tag.Name)
	}
	tagString := strings.Join(tagNames, ", ")

	tmpl, err := template.ParseFiles("./frontend/templates/edit.html")
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	data := struct {
		CardID       string
		Front        string
		Back         string
		Tags         string
		IsOwner      bool
		ErrorMessage string
	}{
		CardID:       card.ID,
		Front:        card.Front.Content,
		Back:         card.Back.Content,
		Tags:         tagString,
		IsOwner:      card.CreatedBy == userID,
		ErrorMessage: "",
	}

	tmpl.Execute(w, data)
}

func renderEditFormWithError(w http.ResponseWriter, cardID, front, back, message string) {
	tmpl, err := template.ParseFiles("./frontend/templates/edit.html")
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	data := struct {
		CardID       string
		Front        string
		Back         string
		ErrorMessage string
	}{
		CardID:       cardID,
		Front:        front,
		Back:         back,
		ErrorMessage: message,
	}

	tmpl.Execute(w, data)
}

func ServeConfirmDeleteButtonEdit(w http.ResponseWriter, r *http.Request) {
	cardID := r.URL.Query().Get("card_id")
	if cardID == "" {
		http.Error(w, "Missing card_id", http.StatusBadRequest)
		return
	}

	tmpl, err := template.ParseFiles("./frontend/templates/partials/confirm-delete-button.html")
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
