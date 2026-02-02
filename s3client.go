package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gofiber/fiber/v2"
)

type S3Client struct {
	*s3.Client
}

func initS3() *S3Client {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(os.Getenv("S3_REGION")),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     os.Getenv("S3_ACCESS_KEY"),
				SecretAccessKey: os.Getenv("S3_SECRET_KEY"),
				Source:          "StaticCredentials",
			},
		}),
	)
	if err != nil {
		msg := fmt.Errorf("unable to load SDK config, %v", err)
		log.Panic(msg)
	}

	client := s3.NewFromConfig(cfg, func(op *s3.Options) {
		op.BaseEndpoint = aws.String(os.Getenv("S3_STORAGE_ENDPOINT"))
		op.UsePathStyle = true
	})

	return &S3Client{Client: client}
}

func (s *S3Client) UploadFile(userId, filename string, data io.Reader, fileSize int64) (string, error) {
	bucketName := os.Getenv("S3_BUCKET_NAME")

	_, err := s.Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(filename),
		Body:   data,
	})
	if err != nil {
		return "", fmt.Errorf("error uploading file: %w", err)
	}

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
		return fiber.NewError(fiber.StatusBadRequest, "<p>Could not parse form data</p>")
	}

	files := form.File["file"]
	if len(files) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "<p>No files uploaded</p>")
	}

	bucketName := os.Getenv("S3_BUCKET_NAME")

	var successCount int
	var failedFiles []string

	for _, file := range files {

		fileBuffer, err := file.Open()
		if err != nil {
			failedFiles = append(failedFiles, file.Filename)
			continue
		}

		objectKey := file.Filename

		if _, err := s.Client.HeadObject(context.TODO(), &s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(objectKey),
		}); err == nil {
			objectKey = fmt.Sprintf("%d_%s", time.Now().Unix(), file.Filename)
		}

		_, err = s.Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(objectKey),
			Body:   fileBuffer,
		})
		if err != nil {
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
			failedFiles = append(failedFiles, file.Filename)
			continue
		}
		successCount++
	}

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
	output, err := s.Client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(fileKey),
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to get file stream: %w", err)
	}

	return output.Body, nil
}
