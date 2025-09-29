package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/abstract-tutoring/models"
	"github.com/abstract-tutoring/utils"
)

func LoginPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("./frontend/templates/login.html", "./frontend/templates/partials/backend-error.html")
	if err != nil {
		log.Printf("LoginPage: Template parse error: %v", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, struct {
		Error   string
		Code    int
		Message string
	}{
		Error:   r.URL.Query().Get("error"),
		Code:    0,
		Message: r.URL.Query().Get("message"),
	})
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	log.Printf("LoginHandler: Login attempt for email: %s", email)

	accessToken, refreshToken, userId, err := AuthenticateWithSupabase(email, password)
	if err != nil {
		log.Printf("LoginHandler: Authentication failed for email: %s", email)
		tmpl, tmplErr := template.ParseFiles(
			"./frontend/templates/login.html",
			"./frontend/templates/partials/backend-error.html",
		)
		if tmplErr != nil {
			log.Printf("LoginHandler: Template parse error: %v", tmplErr)
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}

		// General error struct for alert partial
		tmpl.Execute(w, struct {
			Error   string
			Code    int
			Message string
		}{
			Error:   "Invalid credentials",
			Code:    0,
			Message: "Invalid credentials",
		})
		return
	}

	// Set cookies (secure defaults via utils.SetCookie)
	utils.SetCookie(w, r, "access_token", accessToken, time.Now().Add(15*time.Minute))
	utils.SetCookie(w, r, "refresh_token", refreshToken, time.Now().Add(30*24*time.Hour))
	utils.SetCookie(w, r, "user_id", userId, time.Now().Add(30*24*time.Hour))
	// session cookies for UI state/preferences
	utils.SetCookie(w, r, "review_ahead_days", "0", time.Time{})
	utils.SetCookie(w, r, "max_new_cards_per_day", models.MaxNewCardsPerDay, time.Time{})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func AuthenticateWithSupabase(email, password string) (string, string, string, error) {
	supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
	apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")

	loginPayload := map[string]string{
		"email":    email,
		"password": password,
	}
	payloadBytes, _ := json.Marshal(loginPayload)

	req, err := http.NewRequest("POST", supabaseUrl+"/auth/v1/token?grant_type=password", bytes.NewReader(payloadBytes))
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("apikey", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("AuthenticateWithSupabase: Non-200 status: %d, body: %s", resp.StatusCode, string(body))
		return "", "", "", errors.New("invalid credentials")
	}

	var result models.LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", "", err
	}

	return result.AccessToken, result.RefreshToken, result.User.ID, nil
}

func refreshAccessToken(r *http.Request) (string, string, error) {
	refreshCookie, err := r.Cookie("refresh_token")
	if err != nil {
		return "", "", errors.New("refresh token missing")
	}
	refreshToken := refreshCookie.Value

	supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
	apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")

	payload := map[string]string{
		"refresh_token": refreshToken,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", supabaseUrl+"/auth/v1/token?grant_type=refresh_token", bytes.NewReader(payloadBytes))
	req.Header.Set("apikey", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", "", errors.New("failed to refresh access token")
	}
	defer resp.Body.Close()

	var result models.LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", errors.New("failed to decode refreshed token")
	}

	return result.AccessToken, result.RefreshToken, nil
}

func forceLogout(w http.ResponseWriter, r *http.Request) {
	// Delete all auth-related cookies using utils.ClearCookie
	cookiesToClear := []string{"access_token", "refresh_token", "user_id", "current_card_id"}

	for _, name := range cookiesToClear {
		utils.ClearCookie(w, r, name)
	}

	// Redirect to login page
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	// Clear cookies using utils.ClearCookie
	for _, name := range []string{"access_token", "refresh_token", "user_id", "current_card_id", "review_ahead_days", "max_new_cards_per_day"} {
		utils.ClearCookie(w, r, name)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func fetchStudentIdByUserId(userId, accessToken string) (string, error) {
	supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
	apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")

	req, _ := http.NewRequest("GET", supabaseUrl+"/rest/v1/users_students?select=student_id&user_id=eq."+userId, nil)

	req.Header.Set("apikey", apiKey)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return "", errors.New("failed to fetch student ID")
	}
	defer resp.Body.Close()

	var result []struct {
		StudentID string `json:"student_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result) == 0 {
		return "", errors.New("student ID not found")
	}
	return result[0].StudentID, nil
}

func clearSessionCookies(w http.ResponseWriter, r *http.Request) {
	// Clear session cookies (use utils.ClearCookie to match security flags)
	utils.ClearCookie(w, r, "user_id")
	utils.ClearCookie(w, r, "access_token")
}

func ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("./frontend/templates/forgot-password.html", "./frontend/templates/partials/backend-error.html")
	if err != nil {
		log.Printf("ForgotPasswordPage: Failed to parse template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

func ForgotPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/forgot-password", http.StatusSeeOther)
		return
	}
	email := r.FormValue("email")
	if email == "" {
		tmpl, err := template.ParseFiles("./frontend/templates/forgot-password.html", "./frontend/templates/partials/backend-error.html")
		if err != nil {
			log.Printf("ForgotPasswordHandler: Template parse error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, struct {
			Error   string
			Code    int
			Message string
		}{
			Error:   "Email required",
			Code:    0,
			Message: "",
		})
		return
	}

	supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
	apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")

	payload := map[string]string{
		"email": email,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", supabaseUrl+"/auth/v1/recover", bytes.NewReader(payloadBytes))
	if err != nil {
		log.Printf("ForgotPasswordHandler: Failed to create request: %v", err)
		tmpl, _ := template.ParseFiles("./frontend/templates/forgot-password.html", "./frontend/templates/partials/backend-error.html")
		tmpl.Execute(w, struct{ Error string }{Error: "Failed to send reset email"})
		return
	}
	req.Header.Set("apikey", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("ForgotPasswordHandler: HTTP error: %v", err)
		tmpl, _ := template.ParseFiles("./frontend/templates/forgot-password.html", "./frontend/templates/partials/backend-error.html")
		tmpl.Execute(w, struct{ Error string }{Error: "Failed to send reset email"})
		return
	}
	defer resp.Body.Close()

	log.Printf("ForgotPasswordHandler: Response status code: %d", resp.StatusCode)

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("ForgotPasswordHandler: Non-200 status code: %d, body: %s", resp.StatusCode, string(body))

		var apiErr struct {
			Code      int    `json:"code"`
			ErrorCode string `json:"error_code"`
			Message   string `json:"msg"`
			Error     string `json:"error"`
		}
		json.Unmarshal(body, &apiErr)

		// Prefer msg, fallback to error, fallback to generic
		errMsg := apiErr.Message
		if errMsg == "" {
			errMsg = apiErr.Error
		}
		if errMsg == "" {
			errMsg = "Failed to send reset email"
		}

		tmpl, _ := template.ParseFiles("./frontend/templates/forgot-password.html", "./frontend/templates/partials/backend-error.html")
		tmpl.Execute(w, struct {
			Error     string
			Code      int
			ErrorCode string
		}{
			Error:     errMsg,
			Code:      apiErr.Code,
			ErrorCode: apiErr.ErrorCode,
		})
		return
	}

	tmpl, _ := template.ParseFiles("./frontend/templates/forgot-password.html", "./frontend/templates/partials/backend-error.html")
	tmpl.Execute(w, struct {
		Error   string
		Code    int
		Message string
	}{
		Error:   "",
		Code:    0,
		Message: "Check your email for a reset link.",
	})
}

func PasswordResetPage(w http.ResponseWriter, r *http.Request) {
	accessToken := r.URL.Query().Get("access_token")
	tmpl, err := template.ParseFiles("./frontend/templates/password-reset.html", "./frontend/templates/partials/backend-error.html")
	if err != nil {
		log.Printf("PasswordResetPage: Failed to parse template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, struct{ AccessToken string }{AccessToken: accessToken})
}

func PasswordResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	accessToken := ""
	cookie, err := r.Cookie("access_token")
	if err == nil {
		accessToken = cookie.Value
	}

	newPassword := r.FormValue("password")
	if accessToken == "" || newPassword == "" {
		tmpl, _ := template.ParseFiles("./frontend/templates/password-reset.html", "./frontend/templates/partials/backend-error.html")
		tmpl.Execute(w, struct {
			Error       string
			Code        int
			Message     string
			AccessToken string
		}{
			Error:       "Missing token or password",
			Code:        0,
			Message:     "",
			AccessToken: accessToken,
		})
		return
	}

	supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
	apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")

	payload := map[string]interface{}{
		"password": newPassword,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest("PUT", supabaseUrl+"/auth/v1/user", bytes.NewReader(payloadBytes))
	if err != nil {
		log.Printf("PasswordResetHandler: Failed to create request: %v", err)
		tmpl, _ := template.ParseFiles("./frontend/templates/password-reset.html", "./frontend/templates/partials/backend-error.html")
		tmpl.Execute(w, struct {
			Error       string
			Code        int
			Message     string
			AccessToken string
		}{
			Error:       "Failed to create request",
			Code:        0,
			Message:     "",
			AccessToken: accessToken,
		})
		return
	}
	req.Header.Set("apikey", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("PasswordResetHandler: Failed to send request: %v", err)
		tmpl, _ := template.ParseFiles("./frontend/templates/password-reset.html", "./frontend/templates/partials/backend-error.html")
		tmpl.Execute(w, struct {
			Error       string
			Code        int
			Message     string
			AccessToken string
		}{
			Error:       "Failed to send request",
			Code:        0,
			Message:     "",
			AccessToken: accessToken,
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("PasswordResetHandler: Non-200 status code: %d, body: %s", resp.StatusCode, string(body))

		var apiErr struct {
			Code    int    `json:"code"`
			Message string `json:"msg"`
			Error   string `json:"error"`
		}
		json.Unmarshal(body, &apiErr)

		errMsg := apiErr.Message
		if errMsg == "" {
			errMsg = apiErr.Error
		}
		if errMsg == "" {
			errMsg = "Failed to reset password"
		}

		tmpl, _ := template.ParseFiles("./frontend/templates/password-reset.html", "./frontend/templates/partials/backend-error.html")
		tmpl.Execute(w, struct {
			Error       string
			Code        int
			Message     string
			AccessToken string
		}{
			Error:       errMsg,
			Code:        apiErr.Code,
			Message:     apiErr.Message,
			AccessToken: accessToken,
		})
		return
	}

	http.Redirect(w, r, "/login?message=Password+reset+successful.+You+can+now+log+in.", http.StatusSeeOther)
}

func ConfirmHandler(w http.ResponseWriter, r *http.Request) {
	tokenHash := r.URL.Query().Get("token_hash")
	typ := r.URL.Query().Get("type")
	redirectUrl := r.URL.Query().Get("redirectUrl")
	if tokenHash == "" || typ == "" {
		http.Error(w, "Missing token_hash or type", http.StatusBadRequest)
		return
	}

	supabaseUrl := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_URL")
	apiKey := utils.MustGetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY")

	payload := map[string]string{
		"type":       typ,
		"token_hash": tokenHash,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", supabaseUrl+"/auth/v1/verify", bytes.NewReader(payloadBytes))
	if err != nil {
		log.Printf("ConfirmHandler: Failed to create request: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	req.Header.Set("apikey", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("ConfirmHandler: HTTP error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		log.Printf("ConfirmHandler: Supabase response status: %d", resp.StatusCode)
		tmpl, err := template.ParseFiles("./frontend/templates/login.html", "./frontend/templates/partials/backend-error.html")
		if err != nil {
			http.Redirect(w, r, "/login?error=Error+403%3A+Invalid+reset+link", http.StatusSeeOther)
			return
		}
		tmpl.Execute(w, struct{ Error string }{Error: "Error 403: Invalid reset link"})
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("ConfirmHandler: Supabase response status: %d", resp.StatusCode)
		http.Error(w, "Verification failed", http.StatusUnauthorized)
		return
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		User         struct {
			ID string `json:"id"`
		} `json:"user"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("ConfirmHandler: Failed to decode Supabase response: %v", err)
		http.Error(w, "Failed to decode Supabase response", http.StatusInternalServerError)
		return
	}

	// Set cookies with secure defaults and expiries
	utils.SetCookie(w, r, "access_token", result.AccessToken, time.Now().Add(15*time.Minute))
	utils.SetCookie(w, r, "refresh_token", result.RefreshToken, time.Now().Add(30*24*time.Hour))
	utils.SetCookie(w, r, "user_id", result.User.ID, time.Now().Add(30*24*time.Hour))

	if redirectUrl == "" {
		redirectUrl = "/password-reset"
	}
	http.Redirect(w, r, redirectUrl, http.StatusSeeOther)
}
