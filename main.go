package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

var URL string

type ChunkedUpload struct {
	Chunks   map[int][]byte
	Total    int
	Filename string
	UserID   string
	mu       sync.Mutex
}

var activeUploads = make(map[string]*ChunkedUpload)
var uploadsMu sync.Mutex

func main() {
	initLogger()

	err := godotenv.Load()
	if err != nil {
		appLogger.Warn("Error loading .env file, proceeding with system environment variables")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost"
	}

	URL = fmt.Sprintf("%s:%v/", baseURL, port) // ex: http://localhost:8080/

	err = initDB()
	if err != nil {
		appLogger.WithError(err).Fatal("Database initialization failed")
	}

	redisClient := initRedis()

	app := fiber.New(fiber.Config{
		BodyLimit: 8 * 1024 * 1024, // 8 MB
	})

	// Add logging middleware
	app.Use(loggerMiddleware())

	s3Client := initS3()

	app.Get("/", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")
		return ctx.SendFile("pages/index.htmx")
	})

	app.Post("/upload", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")

		file, err := ctx.FormFile("file")
		if err != nil {
			logWithContext(ctx).WithError(err).Error("No file uploaded")
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: No file uploaded</p>")
		}

		start := time.Now()
		logWithFields(ctx, logrus.Fields{
			"filename":  file.Filename,
			"file_size": formatBytes(uint64(file.Size)),
		}).Info("Starting file upload")

		// upload to supabase storage
		err = s3Client.UploadCtx(ctx)
		if err != nil {
			logWithContext(ctx).WithError(err).Error("File upload failed")
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString(fmt.Sprintf("<p>Error uploading file: %v</p>", err))
		}

		redisClient.deleteShareCache(getUserID(ctx))

		logWithFields(ctx, logrus.Fields{
			"filename":    file.Filename,
			"file_size":   formatBytes(uint64(file.Size)),
			"duration_ms": float64(time.Since(start).Nanoseconds()) / 1e6,
		}).Info("File uploaded successfully")
		return ctx.SendString(fmt.Sprintf("<p>File %s uploaded successfully!</p>", file.Filename))
	})

	app.Post("/upload/chunk", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")

		userId := ctx.FormValue("user_id")
		if userId == "" {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: User ID is required</p>")
		}

		uploadId := ctx.FormValue("upload_id")
		if uploadId == "" {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: Upload ID is required</p>")
		}

		filename := ctx.FormValue("filename")
		if filename == "" {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: Filename is required</p>")
		}

		indexStr := ctx.FormValue("index")
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: Invalid chunk index</p>")
		}

		totalStr := ctx.FormValue("total")
		total, err := strconv.Atoi(totalStr)
		if err != nil {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: Invalid total chunks</p>")
		}

		chunkFile, err := ctx.FormFile("chunk")
		if err != nil {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: No chunk uploaded</p>")
		}

		chunkData, err := chunkFile.Open()
		if err != nil {
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString("<p>Error: Failed to read chunk</p>")
		}
		defer chunkData.Close()

		chunkBytes, err := io.ReadAll(chunkData)
		if err != nil {
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString("<p>Error: Failed to read chunk data</p>")
		}

		uploadsMu.Lock()
		upload, exists := activeUploads[uploadId]
		if !exists {
			upload = &ChunkedUpload{
				Chunks:   make(map[int][]byte),
				Total:    total,
				Filename: filename,
				UserID:   userId,
			}
			activeUploads[uploadId] = upload
		}
		uploadsMu.Unlock()

		upload.mu.Lock()
		upload.Chunks[index] = chunkBytes
		receivedCount := len(upload.Chunks)
		upload.mu.Unlock()

		if receivedCount == total {
			upload.mu.Lock()

			indices := make([]int, 0, len(upload.Chunks))
			for i := range upload.Chunks {
				indices = append(indices, i)
			}
			sort.Ints(indices)

			var assembled []byte
			var totalSize int64
			for _, i := range indices {
				assembled = append(assembled, upload.Chunks[i]...)
				totalSize += int64(len(upload.Chunks[i]))
			}

			upload.mu.Unlock()

			_, err = s3Client.UploadFile(userId, filename, bytes.NewReader(assembled), totalSize)

			uploadsMu.Lock()
			delete(activeUploads, uploadId)
			uploadsMu.Unlock()

			if err != nil {
				ctx.Status(fiber.StatusInternalServerError)
				return ctx.SendString(fmt.Sprintf("<p>Error uploading file: %v</p>", err))
			}

			logWithFields(ctx, logrus.Fields{"filename": filename}).Info("File uploaded successfully (chunked)")
		}

		redisClient.deleteShareCache(getUserID(ctx))

		return ctx.SendString(fmt.Sprintf("<p>File %s uploaded successfully!</p>", filename))
	})

	app.Post("/create-zip", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")

		userId := ctx.FormValue("user_id")
		if userId == "" {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: User ID is required</p>")
		}

		form, err := ctx.MultipartForm()
		if err != nil {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: Could not parse form data</p>")
		}

		files := form.File["zip-files"]
		if len(files) == 0 {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: No files selected</p>")
		}

		zipBuffer, err := createZip(files)
		if err != nil {
			logWithContext(ctx).WithError(err).Error("Error creating zip")
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString("<p>Error creating zip archive</p>")
		}

		zipFilename := fmt.Sprintf("archive_%d.zip", time.Now().Unix())

		_, err = s3Client.UploadFile(userId, zipFilename, bytes.NewReader(zipBuffer.Bytes()), int64(zipBuffer.Len()))
		if err != nil {
			logWithContext(ctx).WithError(err).Error("Error uploading zip")
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString(fmt.Sprintf("<p>Error uploading zip file: %v</p>", err))
		}

		redisClient.deleteShareCache(getUserID(ctx))

		logWithFields(ctx, logrus.Fields{"zip_filename": zipFilename, "file_count": len(files)}).Info("Zip file created and uploaded successfully")
		return ctx.SendString(fmt.Sprintf("<p>Zip %s created successfully! (%d files)</p>", zipFilename, len(files)))
	})

	app.Post("/compress-media", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")

		userId := ctx.FormValue("user_id")
		if userId == "" {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: User ID is required</p>")
		}

		qualityStr := ctx.FormValue("quality")
		quality := getCompressionQuality(qualityStr)

		mediaFiles, err := ctx.MultipartForm()
		if err != nil {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: Could not parse form data</p>")
		}

		files := mediaFiles.File["media-files"]
		if len(files) == 0 {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: No media files selected</p>")
		}

		var videoFiles []*multipart.FileHeader
		var imageFiles []*multipart.FileHeader

		for _, file := range files {
			contentType := file.Header.Get("Content-Type")
			if strings.HasPrefix(contentType, "video/") {
				videoFiles = append(videoFiles, file)
			} else if strings.HasPrefix(contentType, "image/") {
				imageFiles = append(imageFiles, file)
			}
		}

		if len(videoFiles) == 0 && len(imageFiles) == 0 {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: No valid image or video files selected</p>")
		}

		var successCount int
		var failedFiles []string

		for _, file := range imageFiles {
			compressed, err := compressImage(file, quality)
			if err != nil {
				logWithFields(ctx, logrus.Fields{"filename": file.Filename, "error": err.Error()}).Error("Error compressing image")
				failedFiles = append(failedFiles, file.Filename)
				continue
			}

			compressedFilename := getCompressedFileName(file.Filename, false)

			_, err = s3Client.UploadFile(userId, compressedFilename, bytes.NewReader(compressed.Bytes()), int64(compressed.Len()))
			if err != nil {
				logWithFields(ctx, logrus.Fields{"filename": file.Filename, "error": err.Error()}).Error("Error uploading compressed image")
				failedFiles = append(failedFiles, file.Filename)
				continue
			}

			successCount++
			logWithFields(ctx, logrus.Fields{
				"filename":          file.Filename,
				"original_size":     formatBytes(uint64(file.Size)),
				"compressed_size":   formatBytes(uint64(compressed.Len())),
				"reduction_percent": (1 - float64(compressed.Len())/float64(file.Size)) * 100,
			}).Info("Image compressed successfully")
		}

		for _, file := range videoFiles {
			compressed, err := compressVideo(file, quality)
			if err != nil {
				logWithFields(ctx, logrus.Fields{"filename": file.Filename, "error": err.Error()}).Error("Error compressing video")
				failedFiles = append(failedFiles, file.Filename)
				continue
			}

			compressedFilename := getCompressedFileName(file.Filename, true)

			_, err = s3Client.UploadFile(userId, compressedFilename, bytes.NewReader(compressed.Bytes()), int64(compressed.Len()))
			if err != nil {
				logWithFields(ctx, logrus.Fields{"filename": file.Filename, "error": err.Error()}).Error("Error uploading compressed video")
				failedFiles = append(failedFiles, file.Filename)
				continue
			}

			successCount++
			logWithFields(ctx, logrus.Fields{
				"filename":          file.Filename,
				"original_size":     formatBytes(uint64(file.Size)),
				"compressed_size":   formatBytes(uint64(compressed.Len())),
				"reduction_percent": (1 - float64(compressed.Len())/float64(file.Size)) * 100,
			}).Info("Video compressed successfully")
		}

		if successCount == 0 {
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString("<p>All media compression failed</p>")
		}

		if len(failedFiles) > 0 {
			return ctx.SendString(fmt.Sprintf("<p> %d files compressed successfully. Failed: %v</p>", successCount, failedFiles))
		}

		redisClient.deleteShareCache(getUserID(ctx))

		return ctx.SendString(fmt.Sprintf("<p>Successfully compressed %d files!</p>", successCount))
	})

	app.Get("/my-shares", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")
		userID := ctx.Query("user_id")
		if userID == "" {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: User ID is required</p>")
		}

		uploads, err := redisClient.getShareCache(userID)

		if err != nil {
			uploads, err = getUploads(ctx)
			if err != nil {
				logWithContext(ctx).WithError(err).Error("Error retrieving uploads")
				ctx.Status(fiber.StatusInternalServerError)
				return ctx.SendString("<p>Error retrieving uploads</p>")
			}
			redisClient.setShareCache(userID, uploads)
		}

		if len(uploads) == 0 {
			return ctx.SendString(`
        <div class="has-text-centered py-6">
            <div style="font-size: 4rem; margin-bottom: 1rem; opacity: 0.5;">📂</div>
            <p class="has-text-grey">No shares yet. Upload files to create shares.</p>
        </div>
        `)
		}

		var html strings.Builder
		for _, upload := range uploads {

			fileUrl := fmt.Sprintf("%sshare/%s", URL, upload.ShareLink)

			fmt.Fprintf(&html, `
        <div class="box mb-3">
            <div class="is-flex is-justify-content-space-between is-align-items-center">
                <div class="is-flex is-align-items-center" style="gap: 1rem; flex: 1;">
                    <div style="font-size: 1.5rem;">📄</div>
                    <div style="flex: 1;">
                        <div class="has-text-weight-semibold">%s</div>
                        <div class="has-text-grey is-size-7">%s</div>
                    </div>
                </div>
                <div class="is-flex" style="gap: 0.5rem;">
                    <button class="button is-small is-primary is-light" onclick="copyLink('%s')">
                        <span class="icon is-small">
                            <span>📋</span>
                        </span>
                        <span>Copy Link</span>
                    </button>
                </div>
            </div>
        </div>
        `, upload.Filename, formatBytes(uint64(upload.FileSize)), fileUrl)
		}

		return ctx.SendString(html.String())
	})

	app.Get("/share/:id", func(ctx *fiber.Ctx) error {
		shareId := ctx.Params("id")

		var upload Upload
		if err := DB.Where(&Upload{ShareLink: shareId}).First(&upload).Error; err != nil {
			ctx.Status(fiber.StatusNotFound)
			ctx.Set(fiber.HeaderContentType, "text/html")

			logWithFields(ctx, logrus.Fields{"share_id": shareId, "error": err.Error()}).Error("File not found for share ID")
			return ctx.SendString("<p>File not found</p>")
		}

		fileStream, err := s3Client.getFileStream(upload.FileKey)
		if err != nil {
			ctx.Status(fiber.StatusInternalServerError)
			ctx.Set(fiber.HeaderContentType, "text/html")

			logWithFields(ctx, logrus.Fields{"share_id": shareId, "error": err.Error()}).Error("Error retrieving file stream")
			return ctx.SendString("<p>Error retrieving file</p>")
		}
		defer fileStream.Close()

		ctx.Set(fiber.HeaderContentType, "application/octet-stream")
		ctx.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"%s\"", upload.Filename))
		ctx.Set(fiber.HeaderContentLength, fmt.Sprintf("%d", upload.FileSize))

		written, err := io.Copy(ctx.Response().BodyWriter(), fileStream)
		if err != nil {
			if written == 0 {
				logWithFields(ctx, logrus.Fields{"share_id": shareId, "error": err.Error()}).Error("Failed to start file transfer")
			} else {
				logWithFields(ctx, logrus.Fields{"share_id": shareId, "bytes_sent": written}).Debug("Client disconnected during transfer")
			}
		}

		return nil
	})

	app.Get("/health", func(ctx *fiber.Ctx) error {
		stats, err := getSystemStats()
		if err != nil {
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.JSON(stats)
		}

		return ctx.JSON(stats)
	})

	appLogger.WithField("url", URL).Info("Starting server")
	if err := app.Listen(":" + port); err != nil {
		panic(fmt.Sprintf("Server error: %v\n", err))
	}
}
