package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
)

func ValidateGoogleToken(accessToken string) (string, error) {
	oauth2Service, err := oauth2.NewService(context.Background(), option.WithoutAuthentication())
	if err != nil {
		return "", fmt.Errorf("server configuration error")
	}

	tokenInfo, err := oauth2Service.Tokeninfo().AccessToken(accessToken).Do()
	if err != nil {
		return "", fmt.Errorf("invalid token")
	}

	return tokenInfo.Email, nil
}

func ValidateFacebookToken(accessToken string) (string, error) {
	url := fmt.Sprintf("https://graph.facebook.com/me?fields=id,name,email&access_token=%s", accessToken)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to verify facebook token")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("invalid facebook token")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response")
	}

	var userInfo FacebookUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return "", fmt.Errorf("failed to parse user info")
	}

	if userInfo.Email == "" {
		return "", fmt.Errorf("email not provided by facebook")
	}

	return userInfo.Email, nil
}
