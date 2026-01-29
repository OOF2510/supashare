package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
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

func createZip(files []*multipart.FileHeader) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	zipper := zip.NewWriter(buf)

	for _, file := range files {
		fileReader, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("Failed to open file %s: %w", file.Filename, err)
		}
		defer fileReader.Close()

		zipFile, err := zipper.Create(file.Filename)
		if err != nil {
			return nil, fmt.Errorf("Failed to create zip for %s: %w", file.Filename, err)
		}

		_, err = io.Copy(zipFile, fileReader)
		if err != nil {
			return nil, fmt.Errorf("Failed to add file %s to zip: %w", file.Filename, err)
		}
	}

	if err := zipper.Close(); err != nil {
		return nil, fmt.Errorf("Failed to finalize zip: %w", err)
	}

	return buf, nil
}
