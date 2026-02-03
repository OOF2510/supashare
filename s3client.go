package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/sirupsen/logrus"
)

type S3Client struct {
	*minio.Client
}

func initS3() *S3Client {
	endpoint := os.Getenv("S3_STORAGE_ENDPOINT")
	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")

	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		appLogger.WithError(err).WithFields(logrus.Fields{
			"endpoint": endpoint,
		}).Panic("unable to parse endpoint URL")
	}

	hostEndpoint := parsedURL.Host
	if hostEndpoint == "" {
		hostEndpoint = endpoint
	}

	secure := parsedURL.Scheme == "https" || parsedURL.Scheme == ""
	client, err := minio.New(hostEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		appLogger.WithError(err).WithFields(logrus.Fields{
			"endpoint": hostEndpoint,
			"secure":   secure,
		}).Panic("unable to create Minio client")
	}

	bucketName := os.Getenv("S3_BUCKET_NAME")
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		appLogger.WithError(err).WithFields(logrus.Fields{
			"bucket": bucketName,
		}).Warn("failed to check bucket existence")
	} else if !exists {
		appLogger.WithFields(logrus.Fields{
			"bucket": bucketName,
		}).Warn("bucket does not exist")
	}

	appLogger.WithFields(logrus.Fields{
		"endpoint": hostEndpoint,
		"bucket":   bucketName,
		"secure":   secure,
	}).Info("S3 client initialized successfully")

	return &S3Client{Client: client}
}

func (s *S3Client) UploadFile(userId, filename string, data io.Reader, fileSize int64) (string, error) {
	bucketName := os.Getenv("S3_BUCKET_NAME")
	startTime := time.Now()

	appLogger.WithFields(logrus.Fields{
		"user_id":   userId,
		"filename":  filename,
		"file_size": fileSize,
		"bucket":    bucketName,
	}).Info("starting file upload")

	_, err := s.Client.PutObject(context.TODO(), bucketName, filename, data, fileSize, minio.PutObjectOptions{})
	if err != nil {
		appLogger.WithError(err).WithFields(logrus.Fields{
			"user_id":  userId,
			"filename": filename,
		}).Error("file upload failed")
		return "", fmt.Errorf("error uploading file: %w", err)
	}

	duration := time.Since(startTime)
	appLogger.WithFields(logrus.Fields{
		"user_id":   userId,
		"filename":  filename,
		"file_size": fileSize,
		"duration":  duration,
	}).Info("file upload completed successfully")

	shareLink := generateShareLink()
	uploadRecord := Upload{
		UserID:    userId,
		Filename:  filename,
		FileKey:   filename,
		FileSize:  fileSize,
		ShareLink: shareLink,
	}

	if err := DB.Create(&uploadRecord).Error; err != nil {
		return "", fmt.Errorf("error saving upload record: %w", err)
	}

	return shareLink, nil
}

func (s *S3Client) UploadCtx(ctx *fiber.Ctx) error {
	userId := ctx.FormValue("user_id")
	if userId == "" {
		return fiber.NewError(fiber.StatusBadRequest, "<p>User ID is required</p>")
	}

	form, err := ctx.MultipartForm()
	if err != nil {
		appLogger.WithError(err).Warn("could not parse form data")
		return fiber.NewError(fiber.StatusBadRequest, "<p>Could not parse form data</p>")
	}

	files := form.File["file"]
	if len(files) == 0 {
		appLogger.WithFields(logrus.Fields{
			"user_id": userId,
		}).Warn("no files uploaded")
		return fiber.NewError(fiber.StatusBadRequest, "<p>No files uploaded</p>")
	}

	bucketName := os.Getenv("S3_BUCKET_NAME")

	appLogger.WithFields(logrus.Fields{
		"user_id":    userId,
		"file_count": len(files),
		"total_size": ctx.Context().Request.Header.ContentLength(),
	}).Info("starting batch file upload")

	var successCount int
	var failedFiles []string

	for _, file := range files {

		fileBuffer, err := file.Open()
		if err != nil {
			appLogger.WithError(err).WithFields(logrus.Fields{
				"filename": file.Filename,
				"user_id":  userId,
			}).Warn("failed to open file")
			failedFiles = append(failedFiles, file.Filename)
			continue
		}

		objectKey := file.Filename

		// Check if object already exists
		_, errStat := s.Client.StatObject(context.TODO(), bucketName, objectKey, minio.StatObjectOptions{})
		if errStat == nil {
			objectKey = fmt.Sprintf("%d_%s", time.Now().Unix(), file.Filename)
		}

		_, err = s.Client.PutObject(context.TODO(), bucketName, objectKey, fileBuffer, file.Size, minio.PutObjectOptions{})
		if err != nil {
			appLogger.WithError(err).WithFields(logrus.Fields{
				"filename":   file.Filename,
				"object_key": objectKey,
				"user_id":    userId,
			}).Warn("failed to upload file")
			failedFiles = append(failedFiles, file.Filename)
			continue
		}
		fileBuffer.Close()

		shareLink := generateShareLink()

		uploadRecord := Upload{
			UserID:    userId,
			Filename:  file.Filename,
			FileKey:   objectKey,
			FileSize:  file.Size,
			ShareLink: shareLink,
		}

		if err := DB.Create(&uploadRecord).Error; err != nil {
			appLogger.WithError(err).WithFields(logrus.Fields{
				"filename":   file.Filename,
				"object_key": objectKey,
				"user_id":    userId,
			}).Warn("failed to save upload record")
			failedFiles = append(failedFiles, file.Filename)
			continue
		}
		successCount++
	}

	appLogger.WithFields(logrus.Fields{
		"user_id":       userId,
		"success_count": successCount,
		"failed_count":  len(failedFiles),
		"failed_files":  failedFiles,
	}).Info("batch file upload completed")

	if successCount == 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "<p>All file uploads failed</p>")
	}

	if len(failedFiles) > 0 {
		return ctx.SendString(fmt.Sprintf("<p>%d files uploaded successfully. Failed to upload: %v</p>", successCount, failedFiles))
	}

	// get list of uploaded filenames
	var uploadedFilenames []string
	for _, file := range files {
		uploadedFilenames = append(uploadedFilenames, file.Filename)
	}

	return ctx.SendString(fmt.Sprintf("<p>Files %s uploaded successfully!</p>", strings.Join(uploadedFilenames, ", ")))
}

func (s *S3Client) getFileStream(fileKey string) (io.ReadCloser, error) {
	bucketName := os.Getenv("S3_BUCKET_NAME")

	appLogger.WithFields(logrus.Fields{
		"file_key": fileKey,
		"bucket":   bucketName,
	}).Debug("retrieving file stream")

	output, err := s.Client.GetObject(context.TODO(), bucketName, fileKey, minio.GetObjectOptions{})
	if err != nil {
		appLogger.WithError(err).WithFields(logrus.Fields{
			"file_key": fileKey,
			"bucket":   bucketName,
		}).Error("failed to get file stream")
		return nil, fmt.Errorf("failed to get file stream: %w", err)
	}

	return output, nil
}
