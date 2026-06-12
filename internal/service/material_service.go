package service

import (
	"context"
	"errors"
	"fmt"

	"erp-api/internal/models"
	"erp-api/internal/repository"
)

var ErrFKViolation = errors.New("foreign key reference does not exist")

type MaterialService struct {
	repo *repository.MaterialRepo
}

func NewMaterialService(repo *repository.MaterialRepo) *MaterialService {
	return &MaterialService{repo: repo}
}

// ─── Unit ─────────────────────────────────────────────────────────────────────

func (s *MaterialService) ListUnits(ctx context.Context) ([]models.Unit, error) {
	return s.repo.ListUnits(ctx)
}

func (s *MaterialService) GetUnit(ctx context.Context, code string) (models.Unit, error) {
	return s.repo.GetUnit(ctx, code)
}

func (s *MaterialService) UpsertUnit(ctx context.Context, req models.CreateUnitRequest) (models.Unit, error) {
	return s.repo.UpsertUnit(ctx, req)
}

func (s *MaterialService) DeleteUnit(ctx context.Context, code string) error {
	return s.repo.DeleteUnit(ctx, code)
}

// ─── Subgroup ─────────────────────────────────────────────────────────────────

func (s *MaterialService) ListSubgroups(ctx context.Context, groupCode string) ([]models.Subgroup, error) {
	return s.repo.ListSubgroups(ctx, groupCode)
}

func (s *MaterialService) GetSubgroup(ctx context.Context, code string) (models.Subgroup, error) {
	return s.repo.GetSubgroup(ctx, code)
}

func (s *MaterialService) UpsertSubgroup(ctx context.Context, req models.CreateSubgroupRequest) (models.Subgroup, error) {
	if !s.repo.GroupExists(ctx, req.GroupCode) {
		return models.Subgroup{}, fmt.Errorf("%w: group_code '%s'", ErrFKViolation, req.GroupCode)
	}
	return s.repo.UpsertSubgroup(ctx, req)
}

func (s *MaterialService) DeleteSubgroup(ctx context.Context, code string) error {
	return s.repo.DeleteSubgroup(ctx, code)
}

// ─── MatName ──────────────────────────────────────────────────────────────────

func (s *MaterialService) ListMatNames(ctx context.Context, subgroupCode string) ([]models.MatName, error) {
	return s.repo.ListMatNames(ctx, subgroupCode)
}

func (s *MaterialService) GetMatName(ctx context.Context, subgroupCode, matNameCode string) (models.MatName, error) {
	return s.repo.GetMatName(ctx, subgroupCode, matNameCode)
}

func (s *MaterialService) UpsertMatName(ctx context.Context, req models.CreateMatNameRequest) (models.MatName, error) {
	if !s.repo.SubgroupExists(ctx, req.SubgroupCode) {
		return models.MatName{}, fmt.Errorf("%w: subgroup_code '%s'", ErrFKViolation, req.SubgroupCode)
	}
	return s.repo.UpsertMatName(ctx, req)
}

func (s *MaterialService) DeleteMatName(ctx context.Context, subgroupCode, matNameCode string) error {
	return s.repo.DeleteMatName(ctx, subgroupCode, matNameCode)
}

// ─── SpecSize ─────────────────────────────────────────────────────────────────

func (s *MaterialService) ListSpecSizes(ctx context.Context, matNameCode string) ([]models.SpecSize, error) {
	return s.repo.ListSpecSizes(ctx, matNameCode)
}

func (s *MaterialService) GetSpecSize(ctx context.Context, matNameCode, specCode string) (models.SpecSize, error) {
	return s.repo.GetSpecSize(ctx, matNameCode, specCode)
}

func (s *MaterialService) UpsertSpecSize(ctx context.Context, req models.CreateSpecSizeRequest) (models.SpecSize, error) {
	if !s.repo.MatNameExistsByCode(ctx, req.MatNameCode) {
		return models.SpecSize{}, fmt.Errorf("%w: mat_name_code '%s'", ErrFKViolation, req.MatNameCode)
	}
	return s.repo.UpsertSpecSize(ctx, req)
}

func (s *MaterialService) DeleteSpecSize(ctx context.Context, matNameCode, specCode string) error {
	return s.repo.DeleteSpecSize(ctx, matNameCode, specCode)
}

// ─── Brand ────────────────────────────────────────────────────────────────────

func (s *MaterialService) ListBrands(ctx context.Context) ([]models.Brand, error) {
	return s.repo.ListBrands(ctx)
}

func (s *MaterialService) GetBrand(ctx context.Context, code string) (models.Brand, error) {
	return s.repo.GetBrand(ctx, code)
}

func (s *MaterialService) UpsertBrand(ctx context.Context, req models.CreateBrandRequest) (models.Brand, error) {
	return s.repo.UpsertBrand(ctx, req)
}

func (s *MaterialService) DeleteBrand(ctx context.Context, code string) error {
	return s.repo.DeleteBrand(ctx, code)
}

// ─── MaterialCode ─────────────────────────────────────────────────────────────

func (s *MaterialService) ListMaterialCodes(ctx context.Context) ([]models.MaterialCode, error) {
	return s.repo.ListMaterialCodes(ctx)
}

func (s *MaterialService) GetMaterialCode(ctx context.Context, matCode string) (models.MaterialCode, error) {
	return s.repo.GetMaterialCode(ctx, matCode)
}

func (s *MaterialService) CreateMaterialCode(ctx context.Context, req models.CreateMaterialCodeRequest) (models.MaterialCode, error) {
	if !s.repo.GroupExists(ctx, req.GroupCode) {
		return models.MaterialCode{}, fmt.Errorf("%w: group_code '%s'", ErrFKViolation, req.GroupCode)
	}
	if !s.repo.SubgroupExists(ctx, req.SubgroupCode) {
		return models.MaterialCode{}, fmt.Errorf("%w: subgroup_code '%s'", ErrFKViolation, req.SubgroupCode)
	}
	if !s.repo.MatNameExists(ctx, req.SubgroupCode, req.MatNameCode) {
		return models.MaterialCode{}, fmt.Errorf("%w: mat_name_code '%s'", ErrFKViolation, req.MatNameCode)
	}
	if !s.repo.SpecSizeExists(ctx, req.MatNameCode, req.SpecCode) {
		return models.MaterialCode{}, fmt.Errorf("%w: spec_code '%s'", ErrFKViolation, req.SpecCode)
	}
	if !s.repo.BrandExists(ctx, req.BrandCode) {
		return models.MaterialCode{}, fmt.Errorf("%w: brand_code '%s'", ErrFKViolation, req.BrandCode)
	}
	if !s.repo.UnitExists(ctx, req.UnitCode) {
		return models.MaterialCode{}, fmt.Errorf("%w: unit_code '%s'", ErrFKViolation, req.UnitCode)
	}

	matCode := req.GroupCode + req.SubgroupCode + req.MatNameCode + req.SpecCode + req.BrandCode + req.UnitCode

	m := models.MaterialCode{
		MatCode:      matCode,
		GroupCode:    req.GroupCode,
		SubgroupCode: req.SubgroupCode,
		MatNameCode:  req.MatNameCode,
		SpecCode:     req.SpecCode,
		BrandCode:    req.BrandCode,
		UnitCode:     req.UnitCode,
		IsActive:     true,
	}
	return s.repo.UpsertMaterialCode(ctx, m)
}

func (s *MaterialService) UpdateMaterialCode(ctx context.Context, matCode string, req models.UpdateMaterialCodeRequest) error {
	return s.repo.UpdateMaterialCodeActive(ctx, matCode, req.IsActive)
}

func (s *MaterialService) DeleteMaterialCode(ctx context.Context, matCode string) error {
	return s.repo.DeleteMaterialCode(ctx, matCode)
}
