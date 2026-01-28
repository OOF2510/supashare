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
		fmt.Println("Error loading .env file, proceeding with system environment variables")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

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
		err = UploadFile(ctx, s3Client)
		if err != nil {
			fmt.Println(fmt.Errorf("Error uploading file: %w", err))
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString(fmt.Sprintf("<p>Error uploading file: %v</p>", err))
		}

		fmt.Printf("File %s uploaded successfully\n", file.Filename)
		return ctx.SendString(fmt.Sprintf("<p>File %s uploaded successfully!</p>", file.Filename))
	})

	fmt.Printf("Starting server on http://localhost:%s\n", port)
	if err := app.Listen(":" + port); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}
