package database

import (
	"fmt"
	"log"

	"github.com/glebarez/sqlite"
	"github.com/mesnet/mesnet/internal/server/config"
	"github.com/mesnet/mesnet/internal/server/models"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var instance *gorm.DB

func Init(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch cfg.Database.Driver {
	case "sqlite":
		dialector = sqlite.Open(cfg.Database.DSN)
	case "postgres":
		dialector = postgres.Open(cfg.Database.DSN)
	case "mysql":
		dialector = mysql.Open(cfg.Database.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Database.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("connect database failed: %w", err)
	}

	// SQLite pragma
	if cfg.Database.Driver == "sqlite" {
		db.Exec("PRAGMA journal_mode=WAL")
		db.Exec("PRAGMA foreign_keys=ON")
	}

	instance = db
	log.Printf("database connected: %s", cfg.Database.Driver)
	return db, nil
}

func Migrate(db *gorm.DB) {
	if err := db.AutoMigrate(
		&models.Node{},
		&models.Tunnel{},
		&models.AuditLog{},
		&models.TrafficSnapshot{},
		&models.User{},
	); err != nil {
		log.Fatalf("migration failed: %v", err)
	}
	log.Println("database migrated")
}

func Close(db *gorm.DB) {
	sqlDB, _ := db.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}
}
