package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"proxy-checker/internal/database"
	"proxy-checker/internal/services"
)

// AuthHandler handles authentication routes
type AuthHandler struct {
	db *sql.DB
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(db *sql.DB) *AuthHandler {
	return &AuthHandler{db: db}
}

// ShowLogin renders the login page
func (h *AuthHandler) ShowLogin(c *gin.Context) {
	// Check if already logged in
	session := sessions.Default(c)
	if session.Get("user_id") != nil {
		c.Redirect(http.StatusFound, "/admin/dashboard")
		return
	}

	c.HTML(http.StatusOK, "login.html", gin.H{
		"title": "Login",
	})
}

// Login handles login form submission
func (h *AuthHandler) Login(c *gin.Context) {
	password := c.PostForm("password")

	if password == "" {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"title": "Login",
			"error": "Password is required",
		})
		return
	}

	// Get user from database
	user, err := database.GetUser(h.db)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"title": "Login",
			"error": "Database error",
		})
		return
	}

	if user == nil {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"title": "Login",
			"error": "User not found",
		})
		return
	}

	// Check password
	if !services.CheckPassword(password, user.PasswordHash) {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"title": "Login",
			"error": "Invalid password",
		})
		return
	}

	// Set session
	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	if err := session.Save(); err != nil {
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"title": "Login",
			"error": "Failed to save session",
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/dashboard")
}

// Logout handles logout
func (h *AuthHandler) Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.Redirect(http.StatusFound, "/login")
}

// SetupInitialUser creates the initial admin user if not exists
func SetupInitialUser(db *sql.DB, password string) error {
	// Check if user exists
	user, err := database.GetUser(db)
	if err != nil {
		return err
	}

	if user != nil {
		return nil // User already exists
	}

	// Hash password
	hash, err := services.HashPassword(password)
	if err != nil {
		return err
	}

	// Create user
	return database.CreateUser(db, hash)
}
