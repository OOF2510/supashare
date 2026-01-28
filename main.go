package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

var URL string

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file, proceeding with system environment variables")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost"
	}

	URL = fmt.Sprintf("%s:%v/", baseURL, port) // ex: http://localhost:8080/

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
        <div class="has-text-centered py-6">
            <div style="font-size: 4rem; margin-bottom: 1rem; opacity: 0.5;">📂</div>
            <p class="has-text-grey">No shares yet. Upload files to create shares.</p>
        </div>
        `)
		}

		var html strings.Builder
		for _, upload := range uploads {

			fileUrl := fmt.Sprintf("%sshare/%s", URL, upload.ShareLink)

			fmt.Fprintf(&html, `
        <div class="box mb-3">
            <div class="is-flex is-justify-content-space-between is-align-items-center">
                <div class="is-flex is-align-items-center" style="gap: 1rem; flex: 1;">
                    <div style="font-size: 1.5rem;">📄</div>
                    <div style="flex: 1;">
                        <div class="has-text-weight-semibold">%s</div>
                        <div class="has-text-grey is-size-7">%s</div>
                    </div>
                </div>
                <div class="is-flex" style="gap: 0.5rem;">
                    <button class="button is-small is-primary is-light" onclick="copyLink('%s')">
                        <span class="icon is-small">
                            <span>📋</span>
                        </span>
                        <span>Copy Link</span>
                    </button>
                </div>
            </div>
        </div>
        `, upload.Filename, formatBytes(uint64(upload.FileSize)), fileUrl)
		}

		return ctx.SendString(html.String())
	})

	app.Get("/share/:id", func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, "text/html")
		shareId := ctx.Params("id")

		var upload Upload
		if err := DB.Where(&Upload{ShareLink: shareId}).First(&upload).Error; err != nil {
			ctx.Status(fiber.StatusNotFound)
			return ctx.SendString("<p>File not found</p>")
		}

		fileStream, err := getFileStream(s3Client, upload.FileKey)
		if err != nil {
			ctx.Status(fiber.StatusInternalServerError)
			return ctx.SendString("<p>Error retrieving file</p>")
		}
		defer fileStream.Close()

		ctx.Set(fiber.HeaderContentType, "application/octet-stream")
		ctx.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"%s\"", upload.Filename))
		ctx.Set(fiber.HeaderContentLength, fmt.Sprintf("%d", upload.FileSize))

		return ctx.SendStream(fileStream)
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

	fmt.Printf("Starting server on %s\n", URL)
	if err := app.Listen(":" + port); err != nil {
		panic(fmt.Sprintf("Server error: %v\n", err))
	}
}
