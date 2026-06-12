package handlers

import (
	"errors"

	"erp-api/internal/models"
	"erp-api/internal/repository"
	"erp-api/internal/service"

	"github.com/gofiber/fiber/v2"
)

type MaterialCodeHandler struct {
	svc *service.MaterialService
}

func NewMaterialCodeHandler(svc *service.MaterialService) *MaterialCodeHandler {
	return &MaterialCodeHandler{svc: svc}
}

func matErrStatus(err error) *fiber.Error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, repository.ErrHasChildren):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case errors.Is(err, service.ErrFKViolation):
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	default:
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
}

// ─── Unit ─────────────────────────────────────────────────────────────────────

// ListUnits godoc
// @Summary      List units of measure
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /matcode/units [get]
func (h *MaterialCodeHandler) ListUnits(c *fiber.Ctx) error {
	items, err := h.svc.ListUnits(c.Context())
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetUnit godoc
// @Summary      Get unit by code
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Unit Code"
// @Success      200   {object}  models.Unit
// @Failure      404   {object}  fiber.Map
// @Router       /matcode/units/{code} [get]
func (h *MaterialCodeHandler) GetUnit(c *fiber.Ctx) error {
	u, err := h.svc.GetUnit(c.Context(), c.Params("code"))
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": u})
}

// UpsertUnit godoc
// @Summary      Create or update unit (UPSERT)
// @Tags         MaterialCode
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateUnitRequest  true  "Unit data"
// @Success      200   {object}  models.Unit
// @Router       /matcode/units [post]
func (h *MaterialCodeHandler) UpsertUnit(c *fiber.Ctx) error {
	var req models.CreateUnitRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.UnitCode == "" || req.UnitName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "unit_code and unit_name are required")
	}
	u, err := h.svc.UpsertUnit(c.Context(), req)
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": u})
}

// DeleteUnit godoc
// @Summary      Delete unit
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Unit Code"
// @Success      200   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /matcode/units/{code} [delete]
func (h *MaterialCodeHandler) DeleteUnit(c *fiber.Ctx) error {
	if err := h.svc.DeleteUnit(c.Context(), c.Params("code")); err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "message": "unit deleted"})
}

// ─── Subgroup ─────────────────────────────────────────────────────────────────

// ListSubgroups godoc
// @Summary      List subgroups (filter by group_code)
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        group  query  string  false  "group_code filter"
// @Success      200    {object}  fiber.Map
// @Router       /matcode/subgroups [get]
func (h *MaterialCodeHandler) ListSubgroups(c *fiber.Ctx) error {
	items, err := h.svc.ListSubgroups(c.Context(), c.Query("group"))
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetSubgroup godoc
// @Summary      Get subgroup by code
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Subgroup Code"
// @Success      200   {object}  models.Subgroup
// @Failure      404   {object}  fiber.Map
// @Router       /matcode/subgroups/{code} [get]
func (h *MaterialCodeHandler) GetSubgroup(c *fiber.Ctx) error {
	s, err := h.svc.GetSubgroup(c.Context(), c.Params("code"))
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": s})
}

// UpsertSubgroup godoc
// @Summary      Create or update subgroup (UPSERT)
// @Tags         MaterialCode
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateSubgroupRequest  true  "Subgroup data"
// @Success      200   {object}  models.Subgroup
// @Failure      422   {object}  fiber.Map
// @Router       /matcode/subgroups [post]
func (h *MaterialCodeHandler) UpsertSubgroup(c *fiber.Ctx) error {
	var req models.CreateSubgroupRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.SubgroupCode == "" || req.GroupCode == "" || req.SubgroupName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "subgroup_code, group_code and subgroup_name are required")
	}
	s, err := h.svc.UpsertSubgroup(c.Context(), req)
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": s})
}

// DeleteSubgroup godoc
// @Summary      Delete subgroup
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Subgroup Code"
// @Success      200   {object}  fiber.Map
// @Router       /matcode/subgroups/{code} [delete]
func (h *MaterialCodeHandler) DeleteSubgroup(c *fiber.Ctx) error {
	if err := h.svc.DeleteSubgroup(c.Context(), c.Params("code")); err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "message": "subgroup deleted"})
}

// ─── MatName ──────────────────────────────────────────────────────────────────

// ListMatNames godoc
// @Summary      List material names (filter by subgroup_code)
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        subgroup  query  string  false  "subgroup_code filter"
// @Success      200       {object}  fiber.Map
// @Router       /matcode/matnames [get]
func (h *MaterialCodeHandler) ListMatNames(c *fiber.Ctx) error {
	items, err := h.svc.ListMatNames(c.Context(), c.Query("subgroup"))
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetMatName godoc
// @Summary      Get material name by subgroup_code and mat_name_code
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        subgroup  path  string  true  "Subgroup Code"
// @Param        code      path  string  true  "MatName Code"
// @Success      200       {object}  models.MatName
// @Failure      404       {object}  fiber.Map
// @Router       /matcode/matnames/{subgroup}/{code} [get]
func (h *MaterialCodeHandler) GetMatName(c *fiber.Ctx) error {
	m, err := h.svc.GetMatName(c.Context(), c.Params("subgroup"), c.Params("code"))
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": m})
}

// UpsertMatName godoc
// @Summary      Create or update material name (UPSERT)
// @Tags         MaterialCode
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateMatNameRequest  true  "MatName data"
// @Success      200   {object}  models.MatName
// @Failure      422   {object}  fiber.Map
// @Router       /matcode/matnames [post]
func (h *MaterialCodeHandler) UpsertMatName(c *fiber.Ctx) error {
	var req models.CreateMatNameRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.MatNameCode == "" || req.SubgroupCode == "" || req.MatNameTH == "" {
		return fiber.NewError(fiber.StatusBadRequest, "mat_name_code, subgroup_code and mat_name_th are required")
	}
	m, err := h.svc.UpsertMatName(c.Context(), req)
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": m})
}

// DeleteMatName godoc
// @Summary      Delete material name
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        subgroup  path  string  true  "Subgroup Code"
// @Param        code      path  string  true  "MatName Code"
// @Success      200       {object}  fiber.Map
// @Router       /matcode/matnames/{subgroup}/{code} [delete]
func (h *MaterialCodeHandler) DeleteMatName(c *fiber.Ctx) error {
	if err := h.svc.DeleteMatName(c.Context(), c.Params("subgroup"), c.Params("code")); err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "message": "mat_name deleted"})
}

// ─── SpecSize ─────────────────────────────────────────────────────────────────

// ListSpecSizes godoc
// @Summary      List spec sizes (filter by mat_name_code)
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        matname  query  string  false  "mat_name_code filter"
// @Success      200      {object}  fiber.Map
// @Router       /matcode/specs [get]
func (h *MaterialCodeHandler) ListSpecSizes(c *fiber.Ctx) error {
	items, err := h.svc.ListSpecSizes(c.Context(), c.Query("matname"))
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetSpecSize godoc
// @Summary      Get spec size by mat_name_code and spec_code
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        matname  path  string  true  "MatName Code"
// @Param        code     path  string  true  "Spec Code"
// @Success      200      {object}  models.SpecSize
// @Failure      404      {object}  fiber.Map
// @Router       /matcode/specs/{matname}/{code} [get]
func (h *MaterialCodeHandler) GetSpecSize(c *fiber.Ctx) error {
	s, err := h.svc.GetSpecSize(c.Context(), c.Params("matname"), c.Params("code"))
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": s})
}

// UpsertSpecSize godoc
// @Summary      Create or update spec size (UPSERT)
// @Tags         MaterialCode
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateSpecSizeRequest  true  "SpecSize data"
// @Success      200   {object}  models.SpecSize
// @Failure      422   {object}  fiber.Map
// @Router       /matcode/specs [post]
func (h *MaterialCodeHandler) UpsertSpecSize(c *fiber.Ctx) error {
	var req models.CreateSpecSizeRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.SpecCode == "" || req.MatNameCode == "" || req.SpecDescription == "" {
		return fiber.NewError(fiber.StatusBadRequest, "spec_code, mat_name_code and spec_description are required")
	}
	s, err := h.svc.UpsertSpecSize(c.Context(), req)
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": s})
}

// DeleteSpecSize godoc
// @Summary      Delete spec size
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        matname  path  string  true  "MatName Code"
// @Param        code     path  string  true  "Spec Code"
// @Success      200      {object}  fiber.Map
// @Router       /matcode/specs/{matname}/{code} [delete]
func (h *MaterialCodeHandler) DeleteSpecSize(c *fiber.Ctx) error {
	if err := h.svc.DeleteSpecSize(c.Context(), c.Params("matname"), c.Params("code")); err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "message": "spec deleted"})
}

// ─── Brand ────────────────────────────────────────────────────────────────────

// ListBrands godoc
// @Summary      List brands
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /matcode/brands [get]
func (h *MaterialCodeHandler) ListBrands(c *fiber.Ctx) error {
	items, err := h.svc.ListBrands(c.Context())
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetBrand godoc
// @Summary      Get brand by code
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Brand Code"
// @Success      200   {object}  models.Brand
// @Failure      404   {object}  fiber.Map
// @Router       /matcode/brands/{code} [get]
func (h *MaterialCodeHandler) GetBrand(c *fiber.Ctx) error {
	b, err := h.svc.GetBrand(c.Context(), c.Params("code"))
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": b})
}

// UpsertBrand godoc
// @Summary      Create or update brand (UPSERT)
// @Tags         MaterialCode
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateBrandRequest  true  "Brand data"
// @Success      200   {object}  models.Brand
// @Router       /matcode/brands [post]
func (h *MaterialCodeHandler) UpsertBrand(c *fiber.Ctx) error {
	var req models.CreateBrandRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.BrandCode == "" || req.BrandName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "brand_code and brand_name are required")
	}
	b, err := h.svc.UpsertBrand(c.Context(), req)
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": b})
}

// DeleteBrand godoc
// @Summary      Delete brand
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Brand Code"
// @Success      200   {object}  fiber.Map
// @Router       /matcode/brands/{code} [delete]
func (h *MaterialCodeHandler) DeleteBrand(c *fiber.Ctx) error {
	if err := h.svc.DeleteBrand(c.Context(), c.Params("code")); err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "message": "brand deleted"})
}

// ─── MaterialCode ─────────────────────────────────────────────────────────────

// ListMaterialCodes godoc
// @Summary      List all material codes
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /matcode/codes [get]
func (h *MaterialCodeHandler) ListMaterialCodes(c *fiber.Ctx) error {
	items, err := h.svc.ListMaterialCodes(c.Context())
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetMaterialCode godoc
// @Summary      Get material code by mat_code
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Mat Code"
// @Success      200   {object}  models.MaterialCode
// @Failure      404   {object}  fiber.Map
// @Router       /matcode/codes/{code} [get]
func (h *MaterialCodeHandler) GetMaterialCode(c *fiber.Ctx) error {
	m, err := h.svc.GetMaterialCode(c.Context(), c.Params("code"))
	if err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "data": m})
}

// CreateMaterialCode godoc
// @Summary      Create material code (auto-generates mat_code)
// @Tags         MaterialCode
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateMaterialCodeRequest  true  "Material code data"
// @Success      201   {object}  models.MaterialCode
// @Failure      422   {object}  fiber.Map
// @Router       /matcode/codes [post]
func (h *MaterialCodeHandler) CreateMaterialCode(c *fiber.Ctx) error {
	var req models.CreateMaterialCodeRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.GroupCode == "" || req.SubgroupCode == "" || req.MatNameCode == "" ||
		req.SpecCode == "" || req.BrandCode == "" || req.UnitCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "all codes are required")
	}
	m, err := h.svc.CreateMaterialCode(c.Context(), req)
	if err != nil {
		return matErrStatus(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": m})
}

// UpdateMaterialCode godoc
// @Summary      Update material code (is_active only)
// @Tags         MaterialCode
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        code  path  string                          true  "Mat Code"
// @Param        body  body  models.UpdateMaterialCodeRequest true  "Update data"
// @Success      200   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /matcode/codes/{code} [put]
func (h *MaterialCodeHandler) UpdateMaterialCode(c *fiber.Ctx) error {
	var req models.UpdateMaterialCodeRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := h.svc.UpdateMaterialCode(c.Context(), c.Params("code"), req); err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "message": "material code updated"})
}

// DeleteMaterialCode godoc
// @Summary      Delete material code
// @Tags         MaterialCode
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Mat Code"
// @Success      200   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /matcode/codes/{code} [delete]
func (h *MaterialCodeHandler) DeleteMaterialCode(c *fiber.Ctx) error {
	if err := h.svc.DeleteMaterialCode(c.Context(), c.Params("code")); err != nil {
		return matErrStatus(err)
	}
	return c.JSON(fiber.Map{"success": true, "message": "material code deleted"})
}
