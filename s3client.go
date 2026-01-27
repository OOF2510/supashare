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
	})

	return client
}

func UploadFile(ctx *fiber.Ctx, s3Client *s3.Client) error {
	file, err := ctx.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "No file uploaded")
	}

	fileBuffer, err := file.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Could not create file buffer")
	}
	defer fileBuffer.Close()

	bucketName := os.Getenv("S3_BUCKET_NAME")
	objectKey := file.Filename

	result, err := s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
		Body:   fileBuffer,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("Failed to upload to S3: %v", err))
	}

	return ctx.SendString(fmt.Sprintf("<p>File %s uploaded successfully! ETag: %s</p>", file.Filename, *result.ETag))
}
