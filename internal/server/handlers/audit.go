package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/models"
	"gorm.io/gorm"
)

func ListAudit(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var logs []models.AuditLog
		db.Order("created_at desc").Limit(200).Find(&logs)
		c.JSON(http.StatusOK, gin.H{"audit_logs": logs})
	}
}
