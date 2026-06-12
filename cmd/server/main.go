package main

import (
	"log"

	"erp-api/internal/config"
	"erp-api/internal/database"
	"erp-api/internal/middleware"
	"erp-api/internal/routes"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	fiberSwagger "github.com/gofiber/swagger"

	_ "erp-api/docs" // swagger docs
)

// @title           ERP API
// @version         2.0
// @description     ERP System API — PR/PO/Store Management with full Approval workflow
// @termsOfService  http://swagger.io/terms/

// @contact.name   ERP Support
// @contact.email  support@erp.local

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host      localhost:8080
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer {token}"
func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()

	app := fiber.New(fiber.Config{
		AppName:      "ERP API v2",
		ErrorHandler: middleware.ErrorHandler,
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} ${method} ${path} ${latency}\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
	}))

	// Swagger UI
	app.Get("/swagger/*", fiberSwagger.HandlerDefault)
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/swagger/index.html")
	})

	routes.Register(app, db, cfg)

	for _, r := range app.GetRoutes() {
		log.Printf("Route: %s %s", r.Method, r.Path)
	}

	log.Printf("🚀 ERP API starting on port %s", cfg.Port)
	log.Printf("📚 Swagger UI: http://localhost:%s/swagger/index.html", cfg.Port)
	log.Fatal(app.Listen(":" + cfg.Port))
}
