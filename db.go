package main

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"time"
)

type Upload struct {
	ID         uint           `gorm:"primaryKey"`
	UserID     string         `gorm:"index;not null"`
	Filename   string         `gorm:"not null"`
	FileKey    string         `gorm:"uniqueIndex;not null"`
	FileSize   int64          `gorm:"not null"`
	ShareLink  string         `gorm:"uniqueIndex"`
	UploadedAt time.Time      `gorm:"autoCreateTime"`
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

var DB *gorm.DB

func initDB() error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}

	var err error
	DB, err = gorm.Open(postgres.New(postgres.Config{
		DSN:                  url,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		PrepareStmt: false,
		Logger:      logger.Default.LogMode(logger.Error),
	})
	if err != nil {
		return fmt.Errorf("Failed to connect to database: %w", err)
	}

	fmt.Println("Database Initialized Successfully (PrepareStmt: false, PreferSimpleProtocol: true)")
	return nil
}
