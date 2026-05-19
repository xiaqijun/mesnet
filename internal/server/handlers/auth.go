package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/middleware"
	"github.com/mesnet/mesnet/internal/server/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type loginBody struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type changePasswordBody struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

func Login(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body loginBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
			return
		}

		var user models.User
		if err := db.Where("username = ?", body.Username).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
			return
		}

		token, err := middleware.GenerateToken(user.ID, user.Username, user.MustChangePass)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token":            token,
			"user_id":          user.ID,
			"username":         user.Username,
			"must_change_pass": user.MustChangePass,
		})
	}
}

func ChangePassword(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetUint("user_id")

		var body changePasswordBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "old_password and new_password (min 6 chars) required"})
			return
		}

		var user models.User
		if err := db.First(&user, userID).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.OldPassword)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "old password is incorrect"})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}

		db.Model(&user).Updates(map[string]any{
			"password_hash":   string(hash),
			"must_change_pass": false,
			"updated_at":      time.Now(),
		})

		// Issue new token without must_change_pass flag
		token, err := middleware.GenerateToken(user.ID, user.Username, false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token":    token,
			"user_id":  user.ID,
			"username": user.Username,
		})
	}
}

func Me(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetUint("user_id")

		var user models.User
		if err := db.First(&user, userID).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"user_id":          user.ID,
			"username":         user.Username,
			"must_change_pass": user.MustChangePass,
			"created_at":       user.CreatedAt,
		})
	}
}

// SeedAdmin creates the default admin user if no users exist.
func SeedAdmin(db *gorm.DB) {
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count > 0 {
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		panic("failed to hash default admin password: " + err.Error())
	}

	admin := models.User{
		Username:       "admin",
		PasswordHash:   string(hash),
		MustChangePass: true,
	}
	if err := db.Create(&admin).Error; err != nil {
		panic("failed to create default admin user: " + err.Error())
	}
}
