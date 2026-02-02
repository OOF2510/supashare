package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
)

var URL string

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file, proceeding with system environment variables")
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
		msg := fmt.Errorf("Database initialization error: %v\n", err)
		panic(msg)
	}

	app := fiber.New()
	s3Client := initS3()

	app.Get("/", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")
		return ctx.SendFile("pages/index.htmx")
	})

	app.Post("/upload", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")

		file, err := ctx.FormFile("file")
		if err != nil {
			fmt.Println(fmt.Errorf("No file uploaded: %w", err))
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: No file uploaded</p>")
		}

		// upload to supabase storage
		err = s3Client.UploadCtx(ctx)
		if err != nil {
			fmt.Println(fmt.Errorf("Error uploading file: %w", err))
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString(fmt.Sprintf("<p>Error uploading file: %v</p>", err))
		}

		fmt.Printf("File %s uploaded successfully\n", file.Filename)
		return ctx.SendString(fmt.Sprintf("<p>File %s uploaded successfully!</p>", file.Filename))
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
			fmt.Println(fmt.Errorf("Error creating zip: %w", err))
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString("<p>Error creating zip archive</p>")
		}

		zipFilename := fmt.Sprintf("archive_%d.zip", time.Now().Unix())

		bucketName := os.Getenv("S3_BUCKET_NAME")
		_, err = s3Client.Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(zipFilename),
			Body:   bytes.NewReader(zipBuffer.Bytes()),
		})
		if err != nil {
			fmt.Println(fmt.Errorf("Error uploading zip: %w", err))
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString("<p>Error uploading zip file</p>")
		}

		shareLink := generateShareLink()
		uploadRecord := Upload{
			UserID:    userId,
			Filename:  zipFilename,
			FileKey:   zipFilename,
			FileSize:  int64(zipBuffer.Len()),
			ShareLink: shareLink,
		}

		if err := DB.Create(&uploadRecord).Error; err != nil {
			fmt.Println(fmt.Errorf("Error saving upload record: %w", err))
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString("<p>Error saving upload record</p>")
		}

		fmt.Printf("Zip file %s created and uploaded successfully\n", zipFilename)
		return ctx.SendString(fmt.Sprintf("<p>Zip %s created successfully! (%d files)</p>", zipFilename, len(files)))
	})

	app.Post("/compress-media", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")

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

		// check if image or video files
		var validFiles []*multipart.FileHeader
		for _, file := range files {
			if strings.HasPrefix(file.Header.Get("Content-Type"), "image/") || strings.HasPrefix(file.Header.Get("Content-Type"), "video/") {
				validFiles = append(validFiles, file)
			}
		}
		if len(validFiles) == 0 {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: No valid image or video files selected</p>")
		}

		var videoFiles []*multipart.FileHeader
		var imageFiles []*multipart.FileHeader
		for _, file := range validFiles {
			if strings.HasPrefix(file.Header.Get("Content-Type"), "video/") {
				videoFiles = append(videoFiles, file)
			} else if strings.HasPrefix(file.Header.Get("Content-Type"), "image/") {
				imageFiles = append(imageFiles, file)
			}
		}

		fmt.Printf("Received %d videos and %d images for compression\n", len(videoFiles), len(imageFiles))

		return ctx.SendString(fmt.Sprintf("<p>Received %d videos and %d images for compression (work in progress)</p>", len(videoFiles), len(imageFiles)))
	})

	app.Get("/my-shares", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")

		uploads, err := getUploads(ctx)
		if err != nil {
			fmt.Println(fmt.Errorf("Error retrieving uploads: %w", err))
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

			fmt.Println(fmt.Errorf("File not found for share ID %s: %w", shareId, err))
			return ctx.SendString("<p>File not found</p>")
		}

		fileStream, err := s3Client.getFileStream(upload.FileKey)
		if err != nil {
			ctx.Status(fiber.StatusInternalServerError)
			ctx.Set(fiber.HeaderContentType, "text/html")

			fmt.Println(fmt.Errorf("Error retrieving file stream: %w", err))
			return ctx.SendString("<p>Error retrieving file</p>")
		}
		defer fileStream.Close()

		ctx.Set(fiber.HeaderContentType, "application/octet-stream")
		ctx.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"%s\"", upload.Filename))
		ctx.Set(fiber.HeaderContentLength, fmt.Sprintf("%d", upload.FileSize))

		// copy stream to response
		_, err = io.Copy(ctx.Response().BodyWriter(), fileStream)
		if err != nil {
			ctx.Status(fiber.StatusInternalServerError)
			fmt.Println(fmt.Errorf("Error sending file: %w", err))
			return nil
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

	fmt.Printf("Starting server on %s\n", URL)
	if err := app.Listen(":" + port); err != nil {
		panic(fmt.Sprintf("Server error: %v\n", err))
	}
}
