package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gofiber/fiber/v2"
)

func initS3() *s3.Client {
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

	return client
}

func UploadFile(ctx *fiber.Ctx, s3Client *s3.Client) error {
	file, err := ctx.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "<p>No file uploaded</p>")
	}

	userId := ctx.FormValue("user_id")
	if userId == "" {
		return fiber.NewError(fiber.StatusBadRequest, "<p>User ID is required</p>")
	}

	fileBuffer, err := file.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "<p>Could not create file buffer</p>")
	}
	defer fileBuffer.Close()

	bucketName := os.Getenv("S3_BUCKET_NAME")
	objectKey := file.Filename

	_ , err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
		Body:   fileBuffer,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("<p>Failed to upload to Supabase: %v</p>", err))
	}

	shareLink := generateShareLink(file.Filename)

	uploadRecord := Upload{
		UserID:     userId,
		Filename:   file.Filename,
		FileKey:    objectKey,
		FileSize:   file.Size,
		ShareLink:  shareLink,
	}

		if err := DB.Create(&uploadRecord).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "<p>Failed to save upload record to database</p>")
	}

	return ctx.SendString(fmt.Sprintf("<p>File %s uploaded successfully!</p>", file.Filename))
}

// Placeholder for share link generation
func generateShareLink(filename string) string {
	return fmt.Sprintf("https://supashare.oof2510.space/share/%s", filename)
}