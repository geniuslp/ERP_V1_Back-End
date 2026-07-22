package service

import (
	"context"

	"erp-api/internal/repository"
)

type RoleMenuPermissionService struct {
	repo *repository.RoleMenuPermissionRepo
}

func NewRoleMenuPermissionService(repo *repository.RoleMenuPermissionRepo) *RoleMenuPermissionService {
	return &RoleMenuPermissionService{repo: repo}
}

func (s *RoleMenuPermissionService) ListAll(ctx context.Context) ([]repository.RoleMenuPermissionRow, error) {
	return s.repo.ListAll(ctx)
}

func (s *RoleMenuPermissionService) BatchUpsert(ctx context.Context, inputs []repository.UpsertPermissionInput) error {
	for i := range inputs {
		if !inputs[i].CanRead {
			inputs[i].CanWrite = false
			inputs[i].CanUpdate = false
			inputs[i].CanDelete = false
		}
	}
	return s.repo.BatchUpsert(ctx, inputs)
}

func (s *RoleMenuPermissionService) ListMenus(ctx context.Context) ([]repository.MenuRow, error) {
	return s.repo.ListMenus(ctx)
}
