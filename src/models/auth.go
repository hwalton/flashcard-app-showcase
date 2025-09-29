package models

type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	User         struct {
		ID string `json:"id"`
	} `json:"user"`
}

type PasswordResetPageData struct {
	Error       string
	Message     string
	AccessToken string
}
