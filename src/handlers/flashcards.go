package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abstract-tutoring/models"
	"github.com/abstract-tutoring/services"
	"github.com/abstract-tutoring/utils"
)

func ServeHome(w http.ResponseWriter, r *http.Request) {
	_, err := r.Cookie("access_token")
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	tmpl, err := template.ParseFiles(
		"./frontend/templates/base.html",
		"./frontend/templates/flashcards.html",
	)
	if err != nil {
		log.Printf("Template parse error: %v", err)
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.ExecuteTemplate(w, "base", nil)
	if err != nil {
		http.Error(w, "Render error", http.StatusInternalServerError)
	}
}

func ServeFirstFlashcardFront(w http.ResponseWriter, r *http.Request) {
	// log.Println("[DEBUG] ServeFirstFlashcardFront called")
	cardCookie, err := r.Cookie("current_card_id")
	if err == nil && cardCookie.Value != "" {
		cardID := cardCookie.Value

		utils.ClearCookie(w, r, "current_card_id")

		renderFlashcardPartial(w, r, cardID)
		return
	}

	// check/update streak on normal card render
	if userCookie, err1 := r.Cookie("user_id"); err1 == nil && userCookie.Value != "" {
		if accessTokenCookie, err2 := r.Cookie("access_token"); err2 == nil && accessTokenCookie.Value != "" {
			supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
			apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")
			if studentId, ferr := fetchStudentId(r, userCookie.Value, supabaseUrl, apiKey); ferr == nil && studentId != "" {
				// log.Println("[DEBUG] Student ID found:", studentId)
				if serr := checkAndUpdateStreak(r, userCookie.Value, studentId, supabaseUrl, apiKey); serr != nil {
					log.Println("Failed to update streak:", serr)
				}
			}
		}
	}

	// Check if user has any cards at all, and show empty state if not
	userCookie, err := r.Cookie("user_id")
	accessTokenCookie, err2 := r.Cookie("access_token")
	if err == nil && err2 == nil && userCookie.Value != "" && accessTokenCookie.Value != "" {
		supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
		apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")
		studentId, err := fetchStudentId(r, userCookie.Value, supabaseUrl, apiKey)
		if err != nil || studentId == "" { // ← also check for empty studentId
			renderNoCardsAvailable(w, r)
			return
		}
		studentCards, err := fetchStudentCards(nil, r, studentId, supabaseUrl, apiKey)
		if err == nil && len(studentCards) == 0 {
			renderNoCardsAvailable(w, r)
			return
		}
	}
	renderFlashcardPartial(w, r, "")
}

func renderFlashcardPartial(w http.ResponseWriter, r *http.Request, optionalCardID string) {
	// log.Println("[DEBUG] renderFlashcardPartial called with optionalCardID:", optionalCardID)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in renderFlashcardPartial: %v", r)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}()

	data, err := buildCardData(w, r, optionalCardID)
	if err != nil {
		if err.Error() == "no cards available" {
			renderNoCardsAvailable(w, r)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	tmpl, err := template.New("card").Funcs(template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	}).ParseFiles("./frontend/templates/partials/card.html")

	if err != nil {
		log.Println("Template parse error:", err)
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.ExecuteTemplate(w, "card", data)
	if err != nil {
		log.Println("Template execution error:", err)
		http.Error(w, "Render error: "+err.Error(), http.StatusInternalServerError)
	}
}

func SubmitAnswer(w http.ResponseWriter, r *http.Request) {
	// log.Println("SubmitAnswer called")
	userId, err := getCookieValue(r, "user_id")
	if err != nil {
		http.Error(w, "Not logged in", http.StatusUnauthorized)
		return
	}

	cardId := r.FormValue("card_id")
	if cardId == "" {
		http.Error(w, "Missing card ID", http.StatusBadRequest)
		return
	}

	rating := r.FormValue("rating")
	if rating == "" {
		http.Error(w, "Missing rating", http.StatusBadRequest)
		return
	}

	supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
	apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")

	studentId, err := fetchStudentId(r, userId, supabaseUrl, apiKey)
	if err != nil {
		http.Error(w, "Error fetching student ID", http.StatusInternalServerError)
		return
	}

	currentStatus, err := fetchCardStatus(w, r, studentId, cardId, supabaseUrl, apiKey)
	if err != nil {
		http.Error(w, "Error fetching card status", http.StatusInternalServerError)
		return
	}

	newStatus, dueSeconds := services.LookupNext(currentStatus, rating)
	if newStatus == -1 {
		http.Error(w, "Invalid transition", http.StatusBadRequest)
		return
	}

	newDue := int(time.Now().Unix()) + dueSeconds
	err = updateCardStatus(w, r, studentId, cardId, newStatus, newDue, supabaseUrl, apiKey)
	if err != nil {
		http.Error(w, "Failed to update card", http.StatusInternalServerError)
		return
	}

	if currentStatus == 0 {
		err := incrementNumNewCardsToday(r, userId, supabaseUrl, apiKey)
		if err != nil {
			log.Println("Failed to increment numNewCardsToday:", err)
		}
	}
	// Check and update streak
	err = checkAndUpdateStreak(r, userId, studentId, supabaseUrl, apiKey)
	if err != nil {
		log.Println("Failed to update streak:", err)
	}

	renderFlashcardPartial(w, r, "")

}

func HandleSettingsPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles(
		"./frontend/templates/base.html",
		"./frontend/templates/settings.html",
		"./frontend/templates/partials/review-ahead-form.html",
	)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := buildSettingsContext(r, true)

	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "Execution error: "+err.Error(), http.StatusInternalServerError)
	}
}

func getCookieValue(r *http.Request, name string) (string, error) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func fetchStudentId(r *http.Request, userId, supabaseUrl, apiKey string) (string, error) {
	req, _ := http.NewRequest("GET", supabaseUrl+"/rest/v1/users_students?select=student_id&user_id=eq."+userId, nil)

	accessCookie, err := r.Cookie("access_token")
	if err != nil {
		return "", errors.New("access token missing")
	}
	token := accessCookie.Value

	req.Header.Set("apikey", apiKey)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return "", errors.New("failed to fetch student ID")
	}
	defer resp.Body.Close()

	var result []struct {
		StudentID string `json:"student_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errors.New("failed to decode student ID")
	}

	if len(result) == 0 {
		// No mapping exists — return sentinel
		return "", nil
	}

	return result[0].StudentID, nil
}

func fetchCardStatus(w http.ResponseWriter, r *http.Request, studentId, cardId, supabaseUrl, apiKey string) (int, error) {
	tokenCookie, err := r.Cookie("access_token")
	if err != nil {
		return 0, errors.New("access token missing")
	}
	token := tokenCookie.Value

	doRequest := func(token string) (*http.Response, error) {
		req, _ := http.NewRequest("GET", supabaseUrl+"/rest/v1/students_cards?select=status&student_id=eq."+studentId+"&card_id=eq."+cardId, nil)
		req.Header.Set("apikey", apiKey)
		req.Header.Set("Authorization", "Bearer "+token)
		return http.DefaultClient.Do(req)
	}

	resp, err := doRequest(token)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		newToken, newRefresh, refreshErr := refreshAccessToken(r)
		if refreshErr != nil {
			forceLogout(w, r)
			return 0, refreshErr
		}
		utils.SetCookie(w, r, "access_token", newToken, time.Now().Add(15*time.Minute))
		utils.SetCookie(w, r, "refresh_token", newRefresh, time.Now().Add(30*24*time.Hour))

		resp, err = doRequest(newToken)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return 0, errors.New("failed to fetch card status after refresh")
		}
	} else if resp.StatusCode != 200 {
		return 0, errors.New("failed to fetch card status")
	}

	var result []struct {
		Status int `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result) == 0 {
		return 0, errors.New("invalid card")
	}

	return result[0].Status, nil
}

func updateCardStatus(w http.ResponseWriter, r *http.Request, studentId, cardId string, status, due int, supabaseUrl, apiKey string) error {
	tokenCookie, err := r.Cookie("access_token")
	if err != nil {
		return errors.New("access token missing")
	}
	token := tokenCookie.Value

	body := map[string]interface{}{
		"status": status,
		"due":    due,
	}
	jsonBody, _ := json.Marshal(body)

	doRequest := func(token string) (*http.Response, error) {
		req, _ := http.NewRequest("PATCH", supabaseUrl+"/rest/v1/students_cards?student_id=eq."+studentId+"&card_id=eq."+cardId, bytes.NewReader(jsonBody))
		req.Header.Set("apikey", apiKey)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		return http.DefaultClient.Do(req)
	}

	resp, err := doRequest(token)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		newToken, newRefresh, refreshErr := refreshAccessToken(r)
		if refreshErr != nil {
			forceLogout(w, r)
			return refreshErr
		}
		utils.SetCookie(w, r, "access_token", newToken, time.Now().Add(15*time.Minute))
		utils.SetCookie(w, r, "refresh_token", newRefresh, time.Now().Add(30*24*time.Hour))

		resp, err = doRequest(newToken)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 204 {
			return fmt.Errorf("unexpected status code after refresh: %d", resp.StatusCode)
		}
	} else if resp.StatusCode != 204 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func getNumNewCardsToday(r *http.Request, userId, supabaseUrl, apiKey string) (int, error) {
	accessCookie, err := r.Cookie("access_token")
	if err != nil {
		return 0, errors.New("access token missing")
	}
	token := accessCookie.Value

	// Fetch both fields
	url := fmt.Sprintf("%s/rest/v1/users_students?select=num_new_cards_today,num_new_cards_today_updated_at&user_id=eq.%s", supabaseUrl, userId)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("apikey", apiKey)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return 0, errors.New("failed to fetch num_new_cards_today")
	}
	defer resp.Body.Close()

	var result []struct {
		NumNewCardsToday          int   `json:"num_new_cards_today"`
		NumNewCardsTodayUpdatedAt int64 `json:"num_new_cards_today_updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result) == 0 {
		return 0, errors.New("failed to decode num_new_cards_today")
	}

	nowUnix := time.Now().UTC().Unix()
	ukDay := utils.UnixToUKTime(nowUnix).Format("2006-01-02")
	lastUpdatedUnix := result[0].NumNewCardsTodayUpdatedAt
	lastUpdatedDay := utils.UnixToUKTime(lastUpdatedUnix).Format("2006-01-02")

	// If last updated is before today, reset counter and timestamp
	if lastUpdatedUnix == 0 || lastUpdatedDay != ukDay {
		patchUrl := fmt.Sprintf("%s/rest/v1/users_students?user_id=eq.%s", supabaseUrl, userId)
		body := map[string]interface{}{
			"num_new_cards_today":            0,
			"num_new_cards_today_updated_at": nowUnix,
		}
		jsonBody, _ := json.Marshal(body)
		patchReq, _ := http.NewRequest("PATCH", patchUrl, bytes.NewReader(jsonBody))
		patchReq.Header.Set("apikey", apiKey)
		patchReq.Header.Set("Authorization", "Bearer "+token)
		patchReq.Header.Set("Content-Type", "application/json")

		patchResp, err := http.DefaultClient.Do(patchReq)
		if err != nil || (patchResp.StatusCode != 200 && patchResp.StatusCode != 204) {
			return 0, errors.New("failed to reset num_new_cards_today")
		}
		defer patchResp.Body.Close()
		return 0, nil
	}

	return result[0].NumNewCardsToday, nil
}

func incrementNumNewCardsToday(r *http.Request, userId, supabaseUrl, apiKey string) error {
	accessCookie, err := r.Cookie("access_token")
	if err != nil {
		return errors.New("access token missing")
	}
	token := accessCookie.Value

	// Fetch last updated date and current count
	url := fmt.Sprintf("%s/rest/v1/users_students?select=num_new_cards_today,num_new_cards_today_updated_at&user_id=eq.%s", supabaseUrl, userId)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("apikey", apiKey)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return errors.New("failed to fetch num_new_cards_today_updated_at")
	}
	defer resp.Body.Close()

	var result []struct {
		NumNewCardsToday          int   `json:"num_new_cards_today"`
		NumNewCardsTodayUpdatedAt int64 `json:"num_new_cards_today_updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// If decode fails, treat as first card of the day
		result = []struct {
			NumNewCardsToday          int   `json:"num_new_cards_today"`
			NumNewCardsTodayUpdatedAt int64 `json:"num_new_cards_today_updated_at"`
		}{{0, 0}}
	}

	now := time.Now().UTC()
	nowUnix := now.Unix()
	ukDay := utils.UnixToUKTime(nowUnix).Format("2006-01-02")

	var lastUpdatedUnix int64
	var lastUpdatedDay string
	var count int

	if len(result) > 0 {
		lastUpdatedUnix = result[0].NumNewCardsTodayUpdatedAt
		lastUpdatedDay = utils.UnixToUKTime(lastUpdatedUnix).Format("2006-01-02")
		if lastUpdatedUnix == 0 || lastUpdatedDay != ukDay {
			count = 1
		} else {
			count = result[0].NumNewCardsToday + 1
		}
	} else {
		// No record, treat as first card of the day
		count = 1
	}

	// Patch the value
	patchUrl := fmt.Sprintf("%s/rest/v1/users_students?user_id=eq.%s", supabaseUrl, userId)
	body := map[string]interface{}{
		"num_new_cards_today":            count,
		"num_new_cards_today_updated_at": nowUnix,
	}
	jsonBody, _ := json.Marshal(body)
	patchReq, _ := http.NewRequest("PATCH", patchUrl, bytes.NewReader(jsonBody))
	patchReq.Header.Set("apikey", apiKey)
	patchReq.Header.Set("Authorization", "Bearer "+token)
	patchReq.Header.Set("Content-Type", "application/json")

	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil || (patchResp.StatusCode != 200 && patchResp.StatusCode != 204) {
		return errors.New("failed to update num_new_cards_today")
	}
	defer patchResp.Body.Close()

	return nil
}

func renderNoCardsAvailable(w http.ResponseWriter, r *http.Request) {
	utils.ClearCookie(w, r, "current_card_id")

	tmpl, err := template.ParseFiles(
		"./frontend/templates/partials/empty.html",
		"./frontend/templates/partials/review-ahead-form.html",
	)
	if err != nil {
		log.Println("Template error:", err)
		http.Error(w, "Could not load empty state", http.StatusInternalServerError)
		return
	}

	data := buildSettingsContext(r, false)

	// --- Add streak info to context ---
	userCookie, err1 := r.Cookie("user_id")
	accessTokenCookie, err2 := r.Cookie("access_token")
	streakCount := 0
	streakEmoji := ""
	if err1 == nil && err2 == nil && userCookie.Value != "" && accessTokenCookie.Value != "" {
		userId := userCookie.Value
		accessToken := accessTokenCookie.Value
		supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
		apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")
		req, _ := http.NewRequest("GET", supabaseUrl+"/rest/v1/users_students?select=streak_start_time,streak_end_time&user_id=eq."+userId, nil)
		req.Header.Set("apikey", apiKey)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			var streakResult []struct {
				StreakStartTime int64 `json:"streak_start_time"`
				StreakEndTime   int64 `json:"streak_end_time"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&streakResult); err == nil && len(streakResult) > 0 {
				streakCount, streakEmoji = services.GetCurrentStreak(streakResult[0].StreakStartTime, streakResult[0].StreakEndTime)
			}
		}
	}
	data["StreakCount"] = streakCount
	data["StreakEmoji"] = streakEmoji
	// --- End streak info ---

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Render error: "+err.Error(), http.StatusInternalServerError)
	}
}

func HandleReviewAhead(w http.ResponseWriter, r *http.Request) {
	// Clear current_card_id cookie
	utils.ClearCookie(w, r, "current_card_id")

	// Parse raw form values
	tagFilter := r.FormValue("tag_filter")
	daysStr := r.FormValue("days")
	maxStr := r.FormValue("new_max") // Now interpreted as a direct value, not an increment

	// session cookie for tag filter
	utils.SetCookie(w, r, "tag_filter", tagFilter, time.Time{})

	// Set 'review_ahead_days' only if present and valid
	if daysStr != "" {
		if days, err := strconv.Atoi(daysStr); err == nil && days >= 0 {
			utils.SetCookie(w, r, "review_ahead_days", strconv.Itoa(days), time.Time{})
		}
	}

	// Set 'max_new_cards_per_day' only if present and valid
	if maxStr != "" {
		if maxNewCards, err := strconv.Atoi(maxStr); err == nil && maxNewCards >= 0 {
			utils.SetCookie(w, r, "max_new_cards_per_day", strconv.Itoa(maxNewCards), time.Time{})
		}
	}

	// No content returned; client will redirect
	w.WriteHeader(http.StatusNoContent)
}

func ServeStatusPanel(w http.ResponseWriter, r *http.Request) {
	userCookie, err := r.Cookie("user_id")
	if err != nil || userCookie.Value == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	userId := userCookie.Value

	supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
	apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")

	studentId, err := fetchStudentId(r, userId, supabaseUrl, apiKey)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	cards, err := fetchStudentCards(w, r, studentId, supabaseUrl, apiKey)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	accessTokenCookie, err := r.Cookie("access_token")
	if err != nil || accessTokenCookie.Value == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	accessToken := accessTokenCookie.Value

	allCards, err := services.LoadCardsJSON(accessToken)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Parse tag filter from cookie
	tagFilter := map[string]bool{}
	if cookie, err := r.Cookie("tag_filter"); err == nil && cookie.Value != "" {
		parts := strings.Split(cookie.Value, ",")
		for _, p := range parts {
			clean := strings.ToLower(strings.TrimSpace(p))
			if clean != "" {
				tagFilter[clean] = true
			}
		}
	}

	filteredCards := services.FilterStudentCardsByTags(cards, allCards, tagFilter)

	reviewAheadOffset := GetReviewAheadSeconds(r)
	now := time.Now().Unix() + reviewAheadOffset

	var newCount, inProgCount, reviewCount int

	numNewToday, err := getNumNewCardsToday(r, userId, supabaseUrl, apiKey)
	if err != nil {
		log.Println("Error fetching num_new_cards_today:", err)
		numNewToday = 0 // Fallback to 0 if there's an error
	}

	maxNewCardsPerDay := getMaxNewCardsPerDay(r)

	i, err := strconv.Atoi(maxNewCardsPerDay)
	if err != nil {
		log.Println("Conversion error:", err)
		return
	}

	maxNewAllowed := i - numNewToday

	for _, c := range filteredCards {
		switch c.Status {
		case 0:
			if c.Due <= now && newCount < maxNewAllowed {
				newCount++
			}
		case 1, 2, 3:
			inProgCount++
		case 4, 5, 6:
			if c.Due <= now {
				reviewCount++
			}
		}
	}

	currentStatus := r.URL.Query().Get("current")

	data := map[string]interface{}{
		"New":        newCount,
		"InProgress": inProgCount,
		"Review":     reviewCount,
		"CardStatus": currentStatus,
	}

	tmpl, err := template.ParseFiles("./frontend/templates/partials/status-panel.html")
	if err != nil {
		log.Println("Failed to parse status-panel template:", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, data); err != nil {
		log.Println("Failed to render status-panel:", err)
	}
}

func GetReviewAheadSeconds(r *http.Request) int64 {
	cookie, err := r.Cookie("review_ahead_days")
	if err != nil {
		return 0
	}
	days, err := strconv.Atoi(cookie.Value)
	if err != nil {
		return 0
	}
	return int64(days * 24 * 3600)
}

func getMaxNewCardsPerDay(r *http.Request) string {
	cookie, err := r.Cookie("max_new_cards_per_day")
	if err != nil {
		return ""
	}
	return cookie.Value
}

func HandleGoToCard(w http.ResponseWriter, r *http.Request) {
	cardID := r.URL.Query().Get("card_id")

	// Fallback to cookie if not in query
	if cardID == "" {
		cookie, err := r.Cookie("current_card_id")
		if err == nil && cookie.Value != "" {
			cardID = cookie.Value
		}
	}

	// Still missing? Just redirect to "/"
	if cardID == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	GoToCard(w, r, cardID)
}

func GoToCard(w http.ResponseWriter, r *http.Request, cardID string) {
	if cardID == "" {
		http.Error(w, "Missing card ID", http.StatusBadRequest)
		return
	}

	// session cookie for current card (no Expires => session)
	utils.SetCookie(w, r, "current_card_id", cardID, time.Time{})

	// Redirect to flashcards layout
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func buildCardData(w http.ResponseWriter, r *http.Request, optionalCardID string) (map[string]interface{}, error) {
	userCookie, err := r.Cookie("user_id")
	if err != nil || userCookie.Value == "" {
		return nil, fmt.Errorf("unauthenticated")
	}
	userId := userCookie.Value

	accessTokenCookie, err := r.Cookie("access_token")
	if err != nil || accessTokenCookie.Value == "" {
		return nil, fmt.Errorf("unauthenticated")
	}
	accessToken := accessTokenCookie.Value

	supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
	apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")

	studentId, err := fetchStudentId(r, userId, supabaseUrl, apiKey)
	if err != nil || studentId == "" {
		return nil, fmt.Errorf("failed to fetch student ID")
	}

	allCards, err := services.LoadCardsJSON(accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to load cards")
	}

	studentCards, err := fetchStudentCards(nil, r, studentId, supabaseUrl, apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load student cards")
	}

	var cardID string
	if optionalCardID != "" {
		cardID = optionalCardID
	} else {
		numNewToday, err := getNumNewCardsToday(r, userId, supabaseUrl, apiKey)
		if err != nil {
			log.Println("Error fetching num_new_cards_today:", err)
			numNewToday = 0 // Fallback to 0 if there's an error
		}

		reviewAheadOffset := GetReviewAheadSeconds(r)
		maxNewCardsPerDay := getMaxNewCardsPerDay(r)

		allowedTags := make(map[string]bool)
		if tagCookie, err := r.Cookie("tag_filter"); err == nil {
			raw := tagCookie.Value
			for _, t := range strings.Split(raw, ",") {
				trimmed := strings.TrimSpace(t)
				if trimmed != "" {
					allowedTags[trimmed] = true
				}
			}
		}

		pickedID, _, err := services.PickNextCard(
			reviewAheadOffset,
			studentCards,
			numNewToday,
			maxNewCardsPerDay,
			allCards,
			allowedTags,
		)

		if err != nil {
			return nil, fmt.Errorf("no cards available")
		}
		cardID = pickedID
	}

	var pickedCard *models.StudentCard
	for _, c := range studentCards {
		if c.CardID == cardID {
			pickedCard = &c
			break
		}
	}
	if pickedCard == nil {
		return nil, fmt.Errorf("picked card is not in student cards")
	}

	card, ok := allCards[cardID]
	if !ok {
		return nil, fmt.Errorf("card not found")
	}

	tagNames := make([]string, 0, len(card.Tags))
	for _, tag := range card.Tags {
		tagNames = append(tagNames, tag.Name)
	}
	tagNames = services.SortTagsAlphabetically(tagNames)

	cardStatus := services.MapStatusToLabel(pickedCard.Status)

	// session cookie for current card (no Expires => session)
	utils.SetCookie(w, r, "current_card_id", cardID, time.Time{})

	var front, back string
	if card.CreatedBy == userId || card.CreatedBy == "" {
		front = services.ResolveAssetsInContent(card.Front.Content, card.Assets, accessToken)
		back = services.ResolveAssetsInContent(card.Back.Content, card.Assets, accessToken)
	} else {
		safeFront, _ := services.SanitiseAndValidate(card.Front.Content)
		safeBack, _ := services.SanitiseAndValidate(card.Back.Content)
		front = services.ResolveAssetsInContent(safeFront, card.Assets, accessToken)
		back = services.ResolveAssetsInContent(safeBack, card.Assets, accessToken)
	}

	// After you have cardStatus and pickedCard.Status
	ratingTimes := []struct {
		Label      string
		Value      int
		TimeString string
	}{}

	for _, rating := range []struct {
		Value int
		Label string
	}{
		{1, "Bad"}, {2, "Okay"}, {3, "Good"}, {4, "Great"},
	} {
		nextStatus, dueSeconds := services.LookupNext(pickedCard.Status, fmt.Sprintf("%d", rating.Value))
		var timeStr string
		if nextStatus == -1 {
			timeStr = "?"
		} else {
			timeStr = services.FormatDueTime(dueSeconds, nextStatus)
		}
		ratingTimes = append(ratingTimes, struct {
			Label      string
			Value      int
			TimeString string
		}{rating.Label, rating.Value, timeStr})
	}

	// Fetch streak times
	req, _ := http.NewRequest("GET", supabaseUrl+"/rest/v1/users_students?select=streak_start_time,streak_end_time&user_id=eq."+userId, nil)
	req.Header.Set("apikey", apiKey)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)

	var streakCount int
	var streakEmoji string
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		var streakResult []struct {
			StreakStartTime int64 `json:"streak_start_time"`
			StreakEndTime   int64 `json:"streak_end_time"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&streakResult); err == nil && len(streakResult) > 0 {
			streakCount, streakEmoji = services.GetCurrentStreak(streakResult[0].StreakStartTime, streakResult[0].StreakEndTime)
		}
	}

	// Add to your context:
	return map[string]interface{}{
		"Front": front,
		"Back":  back,
		"Ratings": []map[string]interface{}{
			{"Value": 1, "Label": "Bad"},
			{"Value": 2, "Label": "Okay"},
			{"Value": 3, "Label": "Good"},
			{"Value": 4, "Label": "Great"},
		},
		"RatingTimes": ratingTimes,
		"CardStatus":  cardStatus,
		"CardID":      cardID,
		"IsOwner":     card.CreatedBy == userId,
		"Tags":        tagNames,
		"StreakCount": streakCount,
		"StreakEmoji": streakEmoji,
	}, nil
}

func buildSettingsContext(r *http.Request, showCancel bool) map[string]interface{} {
	days := "0"
	if cookie, err := r.Cookie("review_ahead_days"); err == nil {
		days = cookie.Value
	}

	max := "0"
	if cookie, err := r.Cookie("max_new_cards_per_day"); err == nil {
		max = cookie.Value
	}

	tagFilter := ""
	if cookie, err := r.Cookie("tag_filter"); err == nil {
		tagFilter = cookie.Value
	}

	tags := []string{}

	userCookie, err := r.Cookie("user_id")
	accessTokenCookie, err2 := r.Cookie("access_token")
	if err == nil && err2 == nil {
		userId := userCookie.Value
		accessToken := accessTokenCookie.Value

		supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
		apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")

		studentId, err := fetchStudentId(r, userId, supabaseUrl, apiKey)
		if err == nil && studentId != "" {
			allCards, err := services.LoadCardsJSON(accessToken)
			if err == nil {
				studentCards, err := fetchStudentCards(nil, r, studentId, supabaseUrl, apiKey)
				if err == nil {
					tagSet := make(map[string]struct{})

					for _, sc := range studentCards {
						card, ok := allCards[sc.CardID]
						if !ok || card.CreatedBy != userId {
							continue
						}
						for _, tag := range card.Tags {
							tagSet[tag.Name] = struct{}{}
						}
					}

					for tag := range tagSet {
						tags = append(tags, tag)
					}
					tags = services.SortTagsAlphabetically(tags)
				}
			}
		}
	}

	return map[string]interface{}{
		"CurrentDays":      days,
		"CurrentMax":       max,
		"ShowCancel":       showCancel,
		"UserTags":         tags,
		"CurrentTagFilter": tagFilter,
	}
}

func fetchStudentCards(w http.ResponseWriter, r *http.Request, studentId, supabaseUrl, apiKey string) ([]models.StudentCard, error) {
	tokenCookie, err := r.Cookie("access_token")
	if err != nil {
		return nil, errors.New("access token missing")
	}
	token := tokenCookie.Value

	doRequest := func(token string) (*http.Response, error) {
		req, _ := http.NewRequest("GET", supabaseUrl+"/rest/v1/students_cards?select=card_id,status,due&student_id=eq."+studentId, nil)
		req.Header.Set("apikey", apiKey)
		req.Header.Set("Authorization", "Bearer "+token)
		return http.DefaultClient.Do(req)
	}

	resp, err := doRequest(token)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		// refresh
		newToken, newRefresh, refreshErr := refreshAccessToken(r)
		if refreshErr != nil {
			forceLogout(w, r)
			return nil, refreshErr
		}
		utils.SetCookie(w, r, "access_token", newToken, time.Now().Add(15*time.Minute))
		utils.SetCookie(w, r, "refresh_token", newRefresh, time.Now().Add(30*24*time.Hour))

		resp, err = doRequest(newToken)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, errors.New("failed to fetch cards after refresh")
		}
	} else if resp.StatusCode != 200 {
		return nil, errors.New("failed to fetch cards")
	}

	var cards []models.StudentCard
	if err := json.NewDecoder(resp.Body).Decode(&cards); err != nil {
		return nil, errors.New("failed to decode cards")
	}

	sort.Slice(cards, func(i, j int) bool {
		return utils.LexicalCardIDLess(cards[i].CardID, cards[j].CardID)
	})
	return cards, nil
}

func checkAndUpdateStreak(r *http.Request, userId, studentId, supabaseUrl, apiKey string) error {
	// log.Println("checkAndUpdateStreak called for user:", userId, "student:", studentId)
	// Fetch all student cards
	cards, err := fetchStudentCards(nil, r, studentId, supabaseUrl, apiKey)
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	stats := services.CountDueCards(cards, now)

	// log.Println("In Progress Due:", stats.InProgressDue)
	// log.Println("Review Due:", stats.ReviewDue)
	// log.Println("New Available:", stats.NewAvailable)

	// Get today's new card count
	numNewToday, err := getNumNewCardsToday(r, userId, supabaseUrl, apiKey)
	if err != nil {
		return err
	}

	// Only count streak if all due cards are done and (>= 20 new cards completed OR no new cards left)
	if stats.InProgressDue == 0 && stats.ReviewDue == 0 && (numNewToday >= 20 || stats.NewAvailable == 0) {
		nowUnix := time.Now().UTC().Unix()
		// Fetch current streak times
		url := fmt.Sprintf("%s/rest/v1/users_students?select=streak_start_time,streak_end_time&user_id=eq.%s", supabaseUrl, userId)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("apikey", apiKey)
		accessCookie, _ := r.Cookie("access_token")
		req.Header.Set("Authorization", "Bearer "+accessCookie.Value)
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != 200 {
			return errors.New("failed to fetch streak times")
		}
		defer resp.Body.Close()
		var result []struct {
			StreakStartTime int64 `json:"streak_start_time"`
			StreakEndTime   int64 `json:"streak_end_time"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result) == 0 {
			return errors.New("failed to decode streak times")
		}

		// If streak_end_time is before today, start a new streak
		ukDay := utils.UnixToUKTime(nowUnix).Format("2006-01-02")
		// Calculate yesterday's UK date string
		yesterdayUnix := nowUnix - 24*3600
		yesterdayUKDay := utils.UnixToUKTime(yesterdayUnix).Format("2006-01-02")
		lastEndDay := utils.UnixToUKTime(result[0].StreakEndTime).Format("2006-01-02")
		patchUrl := fmt.Sprintf("%s/rest/v1/users_students?user_id=eq.%s", supabaseUrl, userId)
		body := map[string]interface{}{}

		// Reset streak if lastEndDay is before yesterday
		if lastEndDay < yesterdayUKDay {
			body["streak_start_time"] = nowUnix
		} else if lastEndDay != ukDay {
			// New streak (lastEndDay is yesterday)
			body["streak_start_time"] = nowUnix
		}
		// Always update streak_end_time to now
		body["streak_end_time"] = nowUnix

		jsonBody, _ := json.Marshal(body)
		patchReq, _ := http.NewRequest("PATCH", patchUrl, bytes.NewReader(jsonBody))
		patchReq.Header.Set("apikey", apiKey)
		patchReq.Header.Set("Authorization", "Bearer "+accessCookie.Value)
		patchReq.Header.Set("Content-Type", "application/json")
		patchResp, err := http.DefaultClient.Do(patchReq)
		if err != nil {
			return errors.New("failed to update streak times")
		}
		defer patchResp.Body.Close()
		if patchResp.StatusCode != 200 && patchResp.StatusCode != 204 {
			return errors.New("failed to update streak times")
		}
	}

	return nil
}
