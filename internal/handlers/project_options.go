package handlers

import (
	"context"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type projectOption struct {
	ID                  int64    `json:"id"`
	ProjectCode         string   `json:"project_code"`
	ProjectName         string   `json:"project_name"`
	CustomerID          *int64   `json:"customer_id"`
	CustomerName        *string  `json:"customer_name"`
	JobCodes            []string `json:"job_codes"`
	ProjectKind         string   `json:"project_kind"`
	Status              string   `json:"status"`
	IsActive            bool     `json:"is_active"`
	WarehouseCode       *string  `json:"warehouse_code"`
	LinkedWarehouseCode *string  `json:"linked_warehouse_code"`
}

// Options godoc
// @Summary      Project dropdown options (unpaginated)
// @Description  Every matching project in one response for form selectors (Memo, PR, PO, WO, Petty Cash) — GET /master/projects is paginated (max page_size 100, default 20) and cuts the list off. Ordered by project_code. Default = status ACTIVE and is_active = true; include_closed adds status CLOSED, include_inactive adds status INACTIVE and is_active = false. search matches project_code or project_name. A warehouse project is one where COALESCE(linked_warehouse_code, warehouse_code) is set (same rule as the Stock page): warehouse_only keeps only those, exclude_warehouse drops them (if both are sent, nothing matches).
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        search             query  string  false  "project_code / project_name"
// @Param        include_closed     query  bool    false  "also status CLOSED (default false)"
// @Param        include_inactive   query  bool    false  "also status INACTIVE / is_active=false (default false)"
// @Param        warehouse_only     query  bool    false  "only warehouse projects"
// @Param        exclude_warehouse  query  bool    false  "drop warehouse projects"
// @Success      200  {object}  fiber.Map
// @Router       /master/projects/options [get]
func (h *ProjectHandler) Options(c *fiber.Ctx) error {
	var search interface{}
	if s := strings.TrimSpace(c.Query("search")); s != "" {
		search = "%" + s + "%"
	}
	includeClosed := c.QueryBool("include_closed", false)
	includeInactive := c.QueryBool("include_inactive", false)
	warehouseOnly := c.QueryBool("warehouse_only", false)
	excludeWarehouse := c.QueryBool("exclude_warehouse", false)

	rows, err := h.db.Query(context.Background(), `
		SELECT p.id, p.project_code, p.project_name, p.customer_id, cu.customer_name,
		       COALESCE(p.job_codes, '{}'), p.project_kind, p.status, p.is_active,
		       p.warehouse_code, p.linked_warehouse_code
		FROM project p
		LEFT JOIN customer cu ON cu.cus_id = p.customer_id
		WHERE ($1::text IS NULL OR p.project_code ILIKE $1 OR p.project_name ILIKE $1)
		  AND (p.status = 'ACTIVE'
		       OR ($2::bool AND p.status = 'CLOSED')
		       OR ($3::bool AND p.status = 'INACTIVE'))
		  AND (p.is_active OR $3::bool)
		  AND (NOT $4::bool OR COALESCE(NULLIF(p.linked_warehouse_code, ''), NULLIF(p.warehouse_code, '')) IS NOT NULL)
		  AND (NOT $5::bool OR COALESCE(NULLIF(p.linked_warehouse_code, ''), NULLIF(p.warehouse_code, '')) IS NULL)
		ORDER BY p.project_code`,
		search, includeClosed, includeInactive, warehouseOnly, excludeWarehouse)
	if err != nil {
		log.Printf("project options: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายชื่อโครงการได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer rows.Close()

	items := []projectOption{}
	for rows.Next() {
		var o projectOption
		if err := rows.Scan(&o.ID, &o.ProjectCode, &o.ProjectName, &o.CustomerID, &o.CustomerName,
			&o.JobCodes, &o.ProjectKind, &o.Status, &o.IsActive, &o.WarehouseCode, &o.LinkedWarehouseCode); err != nil {
			log.Printf("project options scan: %v", err)
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายชื่อโครงการได้ กรุณาลองใหม่อีกครั้ง")
		}
		if o.JobCodes == nil {
			o.JobCodes = []string{}
		}
		items = append(items, o)
	}
	if err := rows.Err(); err != nil {
		log.Printf("project options rows: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายชื่อโครงการได้ กรุณาลองใหม่อีกครั้ง")
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}
