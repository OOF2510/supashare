package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"image/jpeg"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/gofiber/fiber/v2"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
	ffmpeg "github.com/u2takey/ffmpeg-go"
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

func getSystemStats() (fiber.Map, error) {
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		return fiber.Map{
			"ok":    false,
			"error": "Could not retrieve CPU usage",
		}, err
	}
	memStat, err := mem.VirtualMemory()
	if err != nil {
		return fiber.Map{
			"ok":    false,
			"error": "Could not retrieve memory stats",
		}, err
	}

	pid := os.Getpid()
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return fiber.Map{
			"ok":    false,
			"error": "Could not retrieve process info",
		}, err
	}
	procMemInfo, err := proc.MemoryInfo()
	if err != nil {
		return fiber.Map{
			"ok":    false,
			"error": "Could not retrieve process memory stats",
		}, err
	}

	hostInfo, err := host.Info()
	if err != nil {
		return fiber.Map{
			"ok":    false,
			"error": "Could not retrieve host info",
		}, err
	}

	return fiber.Map{
		"ok": true,
		"system": fiber.Map{
			"cpu": fiber.Map{
				"usage": cpuPercent[0],
			},
			"memory": fiber.Map{
				"systemTotal": formatBytes(memStat.Total),
				"processUsed": formatBytes(procMemInfo.RSS),
			},
			"host": fiber.Map{
				"os":       hostInfo.OS,
				"platform": hostInfo.Platform,
				"uptime":   hostInfo.Uptime,
			},
		},
	}, nil
}

type CompressionQuality int

const (
	QualityHigh CompressionQuality = iota
	QualityMedium
	QualityLow
)

func getCompressionQuality(qualityStr string) CompressionQuality {
	switch qualityStr {
	case "high":
		return QualityHigh
	case "medium":
		return QualityMedium
	case "low":
		return QualityLow
	default:
		return QualityMedium
	}
}

func compressImage(file *multipart.FileHeader, quality CompressionQuality) (*bytes.Buffer, error) {
	srcFile, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("Failed to open image file: %w", err)
	}
	defer srcFile.Close()

	img, err := imaging.Decode(srcFile)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode image: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	maxDimension := 2048
	switch quality {
	case QualityMedium:
		maxDimension = 1600
	case QualityLow:
		maxDimension = 1200
	}

	if width > maxDimension || height > maxDimension {
		img = imaging.Fit(img, maxDimension, maxDimension, imaging.Lanczos)
	}

	buf := new(bytes.Buffer)

	ext := strings.ToLower(filepath.Ext(file.Filename))

	if ext == ".png" {
		// lossless compression, convert to jpeg for better compression
		jpegQuality := 85
		switch quality {
		case QualityMedium:
			jpegQuality = 75
		case QualityLow:
			jpegQuality = 60
		}

		err = jpeg.Encode(buf, img, &jpeg.Options{Quality: jpegQuality})
	} else {
		// JPEG compression
		jpegQuality := 90
		switch quality {
		case QualityMedium:
			jpegQuality = 80
		case QualityLow:
			jpegQuality = 65
		}

		err = jpeg.Encode(buf, img, &jpeg.Options{Quality: jpegQuality})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to encode image: %w", err)
	}

	return buf, nil
}

func compressVideo(file *multipart.FileHeader, quality CompressionQuality) (*bytes.Buffer, error) {
	srcFile, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("Failed to open video file: %w", err)
	}
	defer srcFile.Close()

	inputBuf := new(bytes.Buffer)
	if _, err := inputBuf.ReadFrom(srcFile); err != nil {
		return nil, fmt.Errorf("Failed to read video file: %w", err)
	}

	crf := "28"
	switch quality {
	case QualityHigh:
		crf = "23"
	case QualityLow:
		crf = "32"
	}

	outputBuf := new(bytes.Buffer)

	err = ffmpeg.Input("pipe:", ffmpeg.KwArgs{"format": getVideoFormat(file.Filename)}).
		Output("pipe:", ffmpeg.KwArgs{
			"c:v":      "libx264",
			"crf":      crf,
			"preset":   "medium",
			"c:a":      "aac",
			"b:a":      "128k",
			"movflags": "+faststart",
			"f":        "mp4",
		}).
		WithInput(bytes.NewReader(inputBuf.Bytes())).
		WithOutput(outputBuf).
		Run()

	if err != nil {
		return nil, fmt.Errorf("Failed to compress video: %w", err)
	}

	return outputBuf, nil
}

func getVideoFormat(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".mp4":
		return "mp4"
	case ".mov":
		return "mov"
	case ".avi":
		return "avi"
	case ".mkv":
		return "matroska"
	case ".webm":
		return "webm"
	default:
		return "mp4"
	}
}

func getCompressedFileName(original string, isVideo bool) string {
	ext := filepath.Ext(original)
	name := strings.TrimSuffix(original, ext)

	if isVideo {
		return fmt.Sprintf("%s_compressed%s", name, ext)
	}

	// Convert PNGs to JPEG for better compression
	if strings.ToLower(ext) == ".png" {
		return fmt.Sprintf("%s_compressed.jpg", name)
	}

	return fmt.Sprintf("%s_compressed%s", name, ext)
}
