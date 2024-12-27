package api

import (
	"go-oauth/pkg/auth"
	"net/http"

	"github.com/gin-gonic/gin"
)

func HandleLoginInfo(authConfig auth.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request struct {
			Gmail string `json:"gmail" binding:"required,email"`
			IP    string `json:"ip" binding:"required"`
		}

		if err := c.BindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request format",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Login info saved",
			"gmail":   request.Gmail,
			"ip":      request.IP,
		})
	}
}

func HandleSocialLogin(authConfig auth.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request auth.SocialLoginRequest
		if err := c.BindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, auth.SocialLoginResponse{
				Error: "Invalid request format",
			})
			return
		}

		var userEmail string
		var err error

		switch request.Provider {
		case "google":
			userEmail, err = auth.ValidateGoogleToken(request.AccessToken)
		case "facebook":
			userEmail, err = auth.ValidateFacebookToken(request.AccessToken)
		default:
			c.JSON(http.StatusBadRequest, auth.SocialLoginResponse{
				Error: "Invalid provider",
			})
			return
		}

		if err != nil {
			c.JSON(http.StatusUnauthorized, auth.SocialLoginResponse{
				Error: err.Error(),
			})
			return
		}

		refreshToken, expiresIn, err := auth.CreateRefreshToken(authConfig, userEmail)
		if err != nil {
			c.JSON(http.StatusInternalServerError, auth.SocialLoginResponse{
				Error: "Failed to create refresh token",
			})
			return
		}

		c.JSON(http.StatusOK, auth.SocialLoginResponse{
			RefreshToken: refreshToken,
			ExpiresIn:    expiresIn,
		})
	}
}
