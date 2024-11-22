package auth

type SocialLoginRequest struct {
	AccessToken string `json:"accessToken"`
	Provider    string `json:"provider"`
}

type SocialLoginResponse struct {
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error,omitempty"`
}

type FacebookUserInfo struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}
