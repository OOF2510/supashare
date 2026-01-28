package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

func formatBytes(bytes uint64) string {
	const (
		KB = 1 << 10
		MB = 1 << 20
		GB = 1 << 30
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func getUploads(ctx *fiber.Ctx) ([]Upload, error) {
	userId := ctx.Query("user_id")
	if userId == "" {
		ctx.Status(fiber.StatusBadRequest)
		ctx.SendString("<p>Error: User ID is required</p>")
		e := fmt.Errorf("No User ID provided")
		return nil, e
	}
	var uploads []Upload

	if err := DB.Where("user_id = ?", userId).Order("uploaded_at DESC").Find(&uploads).Error; err != nil {
		ctx.Status(fiber.StatusInternalServerError)
		ctx.SendString("<p>Error retrieving uploads</p>")
		return nil, err
	}

	return uploads, nil
}

func generateShareLink() string {
	bytes := make([]byte, 6)
	
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().Unix()) // fallback to unix timestamp
	}

	return base64.URLEncoding.EncodeToString(bytes)[:8]
}
