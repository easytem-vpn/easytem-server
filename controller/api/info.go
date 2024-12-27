package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func HandleMobileInfo() gin.HandlerFunc {
	return func(c *gin.Context) {
		var request struct {
			Netype string `json:"type" binding:"required"`
		}

		if err := c.BindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request format",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Login info saved",
			"nettype": request.Netype,
		})
	}
}
