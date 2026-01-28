package main

import (
	"fmt"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
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

	app.Get("/my-shares", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")

		uploads, err := getUploads(ctx)
		if err != nil {
			fmt.Println(fmt.Errorf("Error retrieving uploads: %w", err))
		}

		if len(uploads) == 0 {
			return ctx.SendString(`
			<div class="empty-state">
				<div class="empty-state-icon">📂</div>
				<p>No shares yet. Upload files to create shares.</p>
			</div>
		`)
		}
		html := ""
		for _, upload := range uploads {
			html += fmt.Sprintf(`
			<div class="file-item">
				<div class="file-info">
					<div class="file-icon">📄</div>
					<div>
						<div class="file-name">%s</div>
						<div class="file-size">%d bytes</div>
					</div>
				</div>
				<div class="file-actions">
					<button class="action-btn" onclick="copyLink('%s')">📋 Copy Link</button>
				</div>
			</div>
		`, upload.Filename, upload.FileSize, upload.ShareLink)
		}

		return ctx.SendString(html)
	})

	app.Get("/health", func(ctx *fiber.Ctx) error {
		cpuPercent, err := cpu.Percent(time.Second, false)
		if err != nil {
			return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"ok":    false,
				"error": "Could not retrieve CPU usage",
			})
		}
		memStat, err := mem.VirtualMemory()
		if err != nil {
			return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"ok":    false,
				"error": "Could not retrieve memory stats",
			})
		}

		pid := os.Getpid()
		proc, err := process.NewProcess(int32(pid))
		if err != nil {
			return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"ok":    false,
				"error": "Could not retrieve process info",
			})
		}
		procMemInfo, err := proc.MemoryInfo()
		if err != nil {
			return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"ok":    false,
				"error": "Could not retrieve process memory stats",
			})
		}

		hostInfo, err := host.Info()
		if err != nil {
			return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"ok":    false,
				"error": "Could not retrieve host info",
			})
		}

		return ctx.JSON(fiber.Map{
			"ok": true,
			"system": fiber.Map{
				"cpu": fiber.Map{
					"usage": cpuPercent[0],
				},
				"memory": fiber.Map{
					"systemTotal": formatBytes(memStat.Total),
					"processUsed": formatBytes(procMemInfo.RSS),
				},
				"host": fiber.Map{
					"os":       hostInfo.OS,
					"platform": hostInfo.Platform,
					"uptime":   hostInfo.Uptime,
				},
			},
		})
	})

	fmt.Printf("Starting server on http://localhost:%s\n", port)
	if err := app.Listen(":" + port); err != nil {
		panic(fmt.Sprintf("Server error: %v\n", err))
	}
}
