package middleware

import (
	"log"
	"strings"

	"erp-api/internal/auth"
	"erp-api/internal/config"

	"github.com/gofiber/fiber/v2"
)

const ClaimsKey = "claims"

func JWTProtected(cfg *config.Config) fiber.Handler {
	svc := auth.NewService(cfg)
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if header == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing Authorization header")
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid Authorization format, expected: Bearer <token>")
		}

		claims, err := svc.ValidateAccessToken(parts[1])
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired token")
		}

		c.Locals(ClaimsKey, claims)
		return c.Next()
	}
}

// RequireRole checks the user has at least one of the given roles
func RequireRole(roles ...string) fiber.Handler {
	roleSet := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		roleSet[r] = struct{}{}
	}
	return func(c *fiber.Ctx) error {
		claims, ok := c.Locals(ClaimsKey).(*auth.Claims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "no claims found")
		}
		for _, r := range claims.Roles {
			if _, has := roleSet[r]; has {
				return c.Next()
			}
		}
		return fiber.NewError(fiber.StatusForbidden, "insufficient permissions")
	}
}

// DevBypass bypasses JWT for local development — injects a fake ADMIN claims so all
// handlers that call GetClaims still work without a real token.
func DevBypass() fiber.Handler {
	return func(c *fiber.Ctx) error {
		uid := int64(1)
		c.Locals(ClaimsKey, &auth.Claims{
			UserID:   uid,
			Username: "dev",
			FullName: "Dev User",
			Roles:    []string{"ADMIN"},
		})
		return c.Next()
	}
}

// GetClaims is a helper for handlers
func GetClaims(c *fiber.Ctx) *auth.Claims {
	claims, _ := c.Locals(ClaimsKey).(*auth.Claims)
	return claims
}

// ErrorHandler is a global fiber error handler
func ErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	msg := "internal server error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		msg = e.Message
	}

	log.Printf("❌ ErrorHandler: %d %s | path: %s | err: %v", code, msg, c.Path(), err)

	return c.Status(code).JSON(fiber.Map{
		"success": false,
		"error":   msg,
	})
}
