package main

import (
	"fmt"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
)

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

	app.Get("/", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")
		return ctx.SendFile("pages/index.htmx")
	})

	app.Get("/upload", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")

		file, err := ctx.FormFile("file")
		if err != nil {
			ctx.Status(fiber.StatusBadRequest)
			return ctx.SendString("<p>Error: No file uploaded</p>")
		}

		// upload to supabase storage
		err = UploadFile(ctx, s3Client)
		if err != nil {
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString(fmt.Sprintf("<p>Error uploading file: %v</p>", err))
		}

		return ctx.SendString(fmt.Sprintf("<p>File %s uploaded successfully!</p>", file.Filename))

	})

	fmt.Printf("Starting server on http://localhost:%s\n", port)
	if err := app.Listen(":" + port); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}
