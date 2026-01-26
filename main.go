package supashare

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"os"
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
	
	app.Get("/", func(ctx *fiber.Ctx) error {
		return ctx.JSON(map[string]string{"message": "Hello, World!"})
	})
	
	fmt.Printf("Starting server on http://localhost:%s\n", port)
	app.Listen(port)
}