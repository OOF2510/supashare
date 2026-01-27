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
	"github.com/joho/godotenv"
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

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	app := fiber.New()
	s3Client := initS3()

	_ = s3Client // setup endpoint later

	app.Get("/", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")
		return ctx.SendFile("pages/index.htmx")
	})

	fmt.Printf("Starting server on http://localhost:%s\n", port)
	if err := app.Listen(":" + port); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}
