package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
)

type S3Client struct {
	client     *s3.Client
	bucketName string
}

func initS3() *S3Client {
	endpoint := os.Getenv("S3_STORAGE_ENDPOINT")
	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")
	region := os.Getenv("S3_REGION")
	bucketName := os.Getenv("S3_BUCKET_NAME")

	cfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
		o.UsePathStyle = true
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		appLogger.WithError(err).WithFields(logrus.Fields{
			"bucket":   bucketName,
			"endpoint": endpoint,
		}).Warn("failed to access bucket - may not exist or credentials incorrect")
	}

	appLogger.WithFields(logrus.Fields{
		"endpoint": endpoint,
		"bucket":   bucketName,
		"region":   region,
	}).Info("S3 client initialized successfully")

	return &S3Client{
		client:     client,
		bucketName: bucketName,
	}
}

func (s *S3Client) UploadFile(userId, filename string, data io.Reader, fileSize int64) (string, error) {
	startTime := time.Now()

	appLogger.WithFields(logrus.Fields{
		"user_id":   userId,
		"filename":  filename,
		"file_size": fileSize,
		"bucket":    s.bucketName,
	}).Info("starting file upload")

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, data); err != nil {
		return "", fmt.Errorf("error reading file data: %w", err)
	}

	objectKey := filename

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(objectKey),
	})
	cancel()

	if err == nil {
		// Object exists, append timestamp
		objectKey = fmt.Sprintf("%d_%s", time.Now().Unix(), filename)
		appLogger.WithFields(logrus.Fields{
			"original_filename": filename,
			"new_object_key":    objectKey,
		}).Info("file already exists, using new key")
	}

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucketName),
		Key:           aws.String(objectKey),
		Body:          bytes.NewReader(buf.Bytes()),
		ContentLength: aws.Int64(int64(buf.Len())),
	})
	if err != nil {
		appLogger.WithError(err).WithFields(logrus.Fields{
			"user_id":  userId,
			"filename": filename,
			"key":      objectKey,
		}).Error("file upload failed")
		return "", fmt.Errorf("error uploading file: %w", err)
	}

	duration := time.Since(startTime)
	appLogger.WithFields(logrus.Fields{
		"user_id":   userId,
		"filename":  filename,
		"key":       objectKey,
		"file_size": fileSize,
		"duration":  duration,
	}).Info("file upload completed successfully")

	shareLink := generateShareLink()
	uploadRecord := Upload{
		UserID:    userId,
		Filename:  filename,
		FileKey:   objectKey,
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

		// Read file content
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, fileBuffer); err != nil {
			appLogger.WithError(err).WithFields(logrus.Fields{
				"filename": file.Filename,
				"user_id":  userId,
			}).Warn("failed to read file")
			fileBuffer.Close()
			failedFiles = append(failedFiles, file.Filename)
			continue
		}
		fileBuffer.Close()

		objectKey := file.Filename

		ctxCheck, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, errStat := s.client.HeadObject(ctxCheck, &s3.HeadObjectInput{
			Bucket: aws.String(s.bucketName),
			Key:    aws.String(objectKey),
		})
		cancel()

		if errStat == nil {
			objectKey = fmt.Sprintf("%d_%s", time.Now().Unix(), file.Filename)
		}

		ctxUpload, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err = s.client.PutObject(ctxUpload, &s3.PutObjectInput{
			Bucket:        aws.String(s.bucketName),
			Key:           aws.String(objectKey),
			Body:          bytes.NewReader(buf.Bytes()),
			ContentLength: aws.Int64(int64(buf.Len())),
		})
		cancel()

		if err != nil {
			appLogger.WithError(err).WithFields(logrus.Fields{
				"filename":   file.Filename,
				"object_key": objectKey,
				"user_id":    userId,
			}).Warn("failed to upload file")
			failedFiles = append(failedFiles, file.Filename)
			continue
		}

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

	var uploadedFilenames []string
	for _, file := range files {
		uploadedFilenames = append(uploadedFilenames, file.Filename)
	}

	return ctx.SendString(fmt.Sprintf("<p>Files %s uploaded successfully!</p>", strings.Join(uploadedFilenames, ", ")))
}

func (s *S3Client) getFileStream(fileKey string) (io.ReadCloser, error) {
	appLogger.WithFields(logrus.Fields{
		"file_key": fileKey,
		"bucket":   s.bucketName,
	}).Debug("retrieving file stream")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(fileKey),
	})
	if err != nil {
		appLogger.WithError(err).WithFields(logrus.Fields{
			"file_key": fileKey,
			"bucket":   s.bucketName,
		}).Error("failed to get file stream")
		return nil, fmt.Errorf("failed to get file stream: %w", err)
	}

	return output.Body, nil
}
