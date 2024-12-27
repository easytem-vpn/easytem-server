package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

func HandleMobileInfo(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		netype := c.Query("netype")

		if netype == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Missing 'netype' query parameter",
			})
			return
		}

		err := rdb.Set(ctx, c.ClientIP(), netype, 0).Err()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to store Netype in Redis",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "OK",
		})
	}
}

func DelMobileInfo(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		err := rdb.Del(ctx, c.ClientIP()).Err()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to delete Netype in Redis",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "OK",
		})
	}
}
