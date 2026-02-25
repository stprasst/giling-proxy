package middleware

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

// SetupSessions creates a session store
func SetupSessions(secret string) sessions.Store {
	store := cookie.NewStore([]byte(secret))
	store.Options(sessions.Options{
		MaxAge:   86400, // 24 hours
		HttpOnly: true,
		Secure:   false, // Set true in production with HTTPS
		Path:     "/",
	})
	return store
}

// AuthRequired is a middleware that checks if user is authenticated
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		userID := session.Get("user_id")
		if userID == nil {
			// Check if it's an API request
			if c.GetHeader("Accept") == "application/json" || c.Request.Header.Get("Content-Type") == "application/json" {
				c.JSON(401, gin.H{"error": "Unauthorized"})
				c.Abort()
				return
			}
			c.Redirect(302, "/login")
			c.Abort()
			return
		}
		c.Set("user_id", userID)
		c.Next()
	}
}
