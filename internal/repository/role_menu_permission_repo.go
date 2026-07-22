package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RoleMenuPermissionRow struct {
	ID        int64     `json:"id"`
	RoleID    int64     `json:"role_id"`
	RoleCode  string    `json:"role_code"`
	RoleName  string    `json:"role_name"`
	MenuID    int64     `json:"menu_id"`
	MenuCode  string    `json:"menu_code"`
	MenuName  string    `json:"menu_name"`
	ParentID  *int64    `json:"parent_id"`
	CanRead   bool      `json:"can_read"`
	CanWrite  bool      `json:"can_write"`
	CanUpdate bool      `json:"can_update"`
	CanDelete bool      `json:"can_delete"`
}

type MenuRow struct {
	ID        int64      `json:"id"`
	ParentID  *int64     `json:"parent_id"`
	MenuCode  string     `json:"menu_code"`
	MenuName  string     `json:"menu_name"`
	MenuPath  *string    `json:"menu_path"`
	SortOrder int        `json:"sort_order"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Children  []*MenuRow `json:"children,omitempty"`
}

type UpsertPermissionInput struct {
	RoleID    int64 `json:"role_id"`
	MenuID    int64 `json:"menu_id"`
	CanRead   bool  `json:"can_read"`
	CanWrite  bool  `json:"can_write"`
	CanUpdate bool  `json:"can_update"`
	CanDelete bool  `json:"can_delete"`
}

type RoleMenuPermissionRepo struct {
	db *pgxpool.Pool
}

func NewRoleMenuPermissionRepo(db *pgxpool.Pool) *RoleMenuPermissionRepo {
	return &RoleMenuPermissionRepo{db: db}
}

func (r *RoleMenuPermissionRepo) ListAll(ctx context.Context) ([]RoleMenuPermissionRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			rmp.id,
			ro.id, ro.role_code, ro.role_name,
			m.id, m.menu_code, m.menu_name, m.parent_id,
			rmp.can_read, rmp.can_write, rmp.can_update, rmp.can_delete
		FROM role_menu_permissions rmp
		JOIN roles ro ON ro.id = rmp.role_id
		JOIN menus  m  ON m.id  = rmp.menu_id
		ORDER BY ro.role_code, m.sort_order, m.menu_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RoleMenuPermissionRow
	for rows.Next() {
		var p RoleMenuPermissionRow
		if err := rows.Scan(
			&p.ID,
			&p.RoleID, &p.RoleCode, &p.RoleName,
			&p.MenuID, &p.MenuCode, &p.MenuName, &p.ParentID,
			&p.CanRead, &p.CanWrite, &p.CanUpdate, &p.CanDelete,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *RoleMenuPermissionRepo) BatchUpsert(ctx context.Context, inputs []UpsertPermissionInput) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, inp := range inputs {
		_, err := tx.Exec(ctx, `
			INSERT INTO role_menu_permissions
				(role_id, menu_id, can_read, can_write, can_update, can_delete, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW())
			ON CONFLICT (role_id, menu_id) DO UPDATE SET
				can_read   = EXCLUDED.can_read,
				can_write  = EXCLUDED.can_write,
				can_update = EXCLUDED.can_update,
				can_delete = EXCLUDED.can_delete,
				updated_at = NOW()`,
			inp.RoleID, inp.MenuID, inp.CanRead, inp.CanWrite, inp.CanUpdate, inp.CanDelete,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *RoleMenuPermissionRepo) ListMenus(ctx context.Context) ([]MenuRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, parent_id, menu_code, menu_name, menu_path, sort_order, created_at, updated_at
		FROM menus
		ORDER BY sort_order, menu_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []MenuRow
	for rows.Next() {
		var m MenuRow
		if err := rows.Scan(&m.ID, &m.ParentID, &m.MenuCode, &m.MenuName, &m.MenuPath, &m.SortOrder, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		all = append(all, m)
	}
	return all, nil
}
