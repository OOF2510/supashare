package main

import (
	"maps"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

var appLogger = logrus.New()

func initLogger() {
	level, err := logrus.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		level = logrus.InfoLevel
	}
	appLogger.SetLevel(level)

	if os.Getenv("LOG_FORMAT") == "json" {
		appLogger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})
	} else {
		appLogger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
		})
	}

	appLogger.SetOutput(os.Stdout)
}

func maskDSN(dsn string) string {
	if len(dsn) == 0 {
		return ""
	}
	parts := dsn
	for i := 0; i < len(dsn)-8; i++ {
		if dsn[i:i+9] == "password=" {
			end := i + 9
			for end < len(dsn) && dsn[end] != ' ' {
				end++
			}
			parts = dsn[:i+9] + "***REDACTED***" + dsn[end:]
			break
		}
	}
	return parts
}

func loggerMiddleware() fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		requestID := uuid.New().String()
		ctx.Locals("request_id", requestID)

		start := time.Now()

		appLogger.WithFields(logrus.Fields{
			"request_id": requestID,
			"method":     ctx.Method(),
			"path":       ctx.Path(),
			"remote_ip":  ctx.IP(),
			"user_agent": ctx.Get("User-Agent"),
		}).Debug("Request started")

		err := ctx.Next()

		duration := time.Since(start)
		status := ctx.Response().StatusCode()

		fields := logrus.Fields{
			"request_id":   requestID,
			"method":       ctx.Method(),
			"path":         ctx.Path(),
			"status":       status,
			"duration_ms":  float64(duration.Nanoseconds()) / 1e6,
			"duration_str": duration.String(),
		}

		if err != nil {
			fields["error"] = err.Error()
			appLogger.WithFields(fields).Error("Request failed")
		} else if status >= 500 {
			appLogger.WithFields(fields).Error("Server error")
		} else if status >= 400 {
			appLogger.WithFields(fields).Warn("Client error")
		} else {
			appLogger.WithFields(fields).Info("Request completed")
		}

		return err
	}
}

func getRequestID(ctx *fiber.Ctx) string {
	if id, ok := ctx.Locals("request_id").(string); ok {
		return id
	}
	return "unknown"
}

func getUserID(ctx *fiber.Ctx) string {
	userID := ctx.FormValue("user_id")
	if userID == "" {
		userID = ctx.Query("user_id")
	}
	return userID
}

func logWithContext(ctx *fiber.Ctx) *logrus.Entry {
	fields := logrus.Fields{
		"request_id": getRequestID(ctx),
	}

	if userID := getUserID(ctx); userID != "" {
		fields["user_id"] = userID
	}

	return appLogger.WithFields(fields)
}

func logWithFields(ctx *fiber.Ctx, additionalFields logrus.Fields) *logrus.Entry {
	fields := logrus.Fields{
		"request_id": getRequestID(ctx),
	}

	if userID := getUserID(ctx); userID != "" {
		fields["user_id"] = userID
	}

	maps.Copy(fields, additionalFields)

	return appLogger.WithFields(fields)
}
