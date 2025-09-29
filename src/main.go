package main

import (
	"log"
	"net/http"

	"github.com/abstract-tutoring/handlers"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Println("No .env file found or error loading it:", err)
	}

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./frontend/static"))))

	http.HandleFunc("/", handlers.ServeHome)
	http.HandleFunc("/login", handlers.LoginPage)
	http.HandleFunc("/perform-login", handlers.LoginHandler)
	http.HandleFunc("/flashcard/front", handlers.ServeFirstFlashcardFront)
	http.HandleFunc("/flashcard/answer", handlers.SubmitAnswer)
	http.HandleFunc("/flashcard/status", handlers.ServeStatusPanel)
	http.HandleFunc("/flashcard/review-ahead", handlers.HandleReviewAhead)
	http.HandleFunc("/logout", handlers.LogoutHandler)
	http.HandleFunc("/browse", handlers.ServeBrowsePage)
	http.HandleFunc("/goto", handlers.HandleGoToCard)
	http.HandleFunc("/create", handlers.CreateCardPage)
	http.HandleFunc("/create-card", handlers.CreateCardHandler)
	http.HandleFunc("/unlink-card", handlers.UnlinkCardHandler)
	http.HandleFunc("/confirm-delete-button", handlers.ServeConfirmDeleteButton)
	http.HandleFunc("/edit", handlers.EditCardPage)
	http.HandleFunc("/edit-card", handlers.EditCardHandler)
	http.HandleFunc("/settings", handlers.HandleSettingsPage)
	http.HandleFunc("/confirm-delete-button-edit", handlers.ServeConfirmDeleteButtonEdit)
	http.HandleFunc("/forgot-password", handlers.ForgotPasswordPage)
	http.HandleFunc("/perform-forgot-password", handlers.ForgotPasswordHandler)
	http.HandleFunc("/password-reset", handlers.PasswordResetPage)
	http.HandleFunc("/perform-password-reset", handlers.PasswordResetHandler)

	log.Println("Server listening on :8080")
	http.HandleFunc("/api/auth/confirm", handlers.ConfirmHandler)

	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("Server error:", err)
	}
}
