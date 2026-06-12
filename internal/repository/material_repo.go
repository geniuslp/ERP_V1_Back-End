package repository

import (
	"context"
	"errors"

	"erp-api/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("record not found")
var ErrHasChildren = errors.New("record has dependent children")

type MaterialRepo struct {
	db *pgxpool.Pool
}

func NewMaterialRepo(db *pgxpool.Pool) *MaterialRepo {
	return &MaterialRepo{db: db}
}

// ─── Unit ─────────────────────────────────────────────────────────────────────

func (r *MaterialRepo) ListUnits(ctx context.Context) ([]models.Unit, error) {
	rows, err := r.db.Query(ctx, `SELECT unit_code, unit_name FROM unit ORDER BY unit_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Unit
	for rows.Next() {
		var u models.Unit
		if err := rows.Scan(&u.UnitCode, &u.UnitName); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

func (r *MaterialRepo) GetUnit(ctx context.Context, code string) (models.Unit, error) {
	var u models.Unit
	err := r.db.QueryRow(ctx, `SELECT unit_code, unit_name FROM unit WHERE unit_code = $1`, code).
		Scan(&u.UnitCode, &u.UnitName)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (r *MaterialRepo) UpsertUnit(ctx context.Context, req models.CreateUnitRequest) (models.Unit, error) {
	_, err := r.db.Exec(ctx, `
		INSERT INTO unit (unit_code, unit_name) VALUES ($1, $2)
		ON CONFLICT (unit_code) DO UPDATE SET unit_name = EXCLUDED.unit_name`,
		req.UnitCode, req.UnitName)
	if err != nil {
		return models.Unit{}, err
	}
	return models.Unit{UnitCode: req.UnitCode, UnitName: req.UnitName}, nil
}

func (r *MaterialRepo) DeleteUnit(ctx context.Context, code string) error {
	var count int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM material_code WHERE unit_code = $1`, code).Scan(&count)
	if count > 0 {
		return ErrHasChildren
	}
	tag, err := r.db.Exec(ctx, `DELETE FROM unit WHERE unit_code = $1`, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MaterialRepo) UnitExists(ctx context.Context, code string) bool {
	var n int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM unit WHERE unit_code = $1`, code).Scan(&n)
	return n > 0
}

// ─── Subgroup ─────────────────────────────────────────────────────────────────

func (r *MaterialRepo) ListSubgroups(ctx context.Context, groupCode string) ([]models.Subgroup, error) {
	q := `SELECT subgroup_code, group_code, subgroup_name FROM mat_subgroup ORDER BY subgroup_code`
	args := []interface{}{}
	if groupCode != "" {
		q = `SELECT subgroup_code, group_code, subgroup_name FROM mat_subgroup WHERE group_code = $1 ORDER BY subgroup_code`
		args = append(args, groupCode)
	}
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Subgroup
	for rows.Next() {
		var s models.Subgroup
		if err := rows.Scan(&s.SubgroupCode, &s.GroupCode, &s.SubgroupName); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *MaterialRepo) GetSubgroup(ctx context.Context, code string) (models.Subgroup, error) {
	var s models.Subgroup
	err := r.db.QueryRow(ctx,
		`SELECT subgroup_code, group_code, subgroup_name FROM mat_subgroup WHERE subgroup_code = $1`, code).
		Scan(&s.SubgroupCode, &s.GroupCode, &s.SubgroupName)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}

func (r *MaterialRepo) UpsertSubgroup(ctx context.Context, req models.CreateSubgroupRequest) (models.Subgroup, error) {
	_, err := r.db.Exec(ctx, `
		INSERT INTO mat_subgroup (subgroup_code, group_code, subgroup_name) VALUES ($1, $2, $3)
		ON CONFLICT (subgroup_code) DO UPDATE SET group_code = EXCLUDED.group_code, subgroup_name = EXCLUDED.subgroup_name`,
		req.SubgroupCode, req.GroupCode, req.SubgroupName)
	if err != nil {
		return models.Subgroup{}, err
	}
	return models.Subgroup{SubgroupCode: req.SubgroupCode, GroupCode: req.GroupCode, SubgroupName: req.SubgroupName}, nil
}

func (r *MaterialRepo) DeleteSubgroup(ctx context.Context, code string) error {
	var count int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM mat_name WHERE subgroup_code = $1`, code).Scan(&count)
	if count > 0 {
		return ErrHasChildren
	}
	tag, err := r.db.Exec(ctx, `DELETE FROM mat_subgroup WHERE subgroup_code = $1`, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MaterialRepo) SubgroupExists(ctx context.Context, code string) bool {
	var n int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM mat_subgroup WHERE subgroup_code = $1`, code).Scan(&n)
	return n > 0
}

// ─── MatName ──────────────────────────────────────────────────────────────────

func (r *MaterialRepo) ListMatNames(ctx context.Context, subgroupCode string) ([]models.MatName, error) {
	q := `SELECT mat_name_code, subgroup_code, mat_name_th, mat_name_en FROM mat_name ORDER BY mat_name_code`
	args := []interface{}{}
	if subgroupCode != "" {
		q = `SELECT mat_name_code, subgroup_code, mat_name_th, mat_name_en FROM mat_name WHERE subgroup_code = $1 ORDER BY mat_name_code`
		args = append(args, subgroupCode)
	}
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.MatName
	for rows.Next() {
		var m models.MatName
		if err := rows.Scan(&m.MatNameCode, &m.SubgroupCode, &m.MatNameTH, &m.MatNameEN); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *MaterialRepo) GetMatName(ctx context.Context, subgroupCode, matNameCode string) (models.MatName, error) {
	var m models.MatName
	err := r.db.QueryRow(ctx,
		`SELECT mat_name_code, subgroup_code, mat_name_th, mat_name_en FROM mat_name WHERE subgroup_code = $1 AND mat_name_code = $2`,
		subgroupCode, matNameCode).
		Scan(&m.MatNameCode, &m.SubgroupCode, &m.MatNameTH, &m.MatNameEN)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

func (r *MaterialRepo) UpsertMatName(ctx context.Context, req models.CreateMatNameRequest) (models.MatName, error) {
	_, err := r.db.Exec(ctx, `
		INSERT INTO mat_name (mat_name_code, subgroup_code, mat_name_th, mat_name_en) VALUES ($1, $2, $3, $4)
		ON CONFLICT (subgroup_code, mat_name_code) DO UPDATE SET mat_name_th = EXCLUDED.mat_name_th, mat_name_en = EXCLUDED.mat_name_en`,
		req.MatNameCode, req.SubgroupCode, req.MatNameTH, req.MatNameEN)
	if err != nil {
		return models.MatName{}, err
	}
	return models.MatName{MatNameCode: req.MatNameCode, SubgroupCode: req.SubgroupCode, MatNameTH: req.MatNameTH, MatNameEN: req.MatNameEN}, nil
}

func (r *MaterialRepo) DeleteMatName(ctx context.Context, subgroupCode, matNameCode string) error {
	var count int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM mat_spec WHERE mat_name_code = $1`, matNameCode).Scan(&count)
	if count > 0 {
		return ErrHasChildren
	}
	tag, err := r.db.Exec(ctx, `DELETE FROM mat_name WHERE subgroup_code = $1 AND mat_name_code = $2`, subgroupCode, matNameCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MaterialRepo) MatNameExists(ctx context.Context, subgroupCode, matNameCode string) bool {
	var n int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM mat_name WHERE subgroup_code = $1 AND mat_name_code = $2`, subgroupCode, matNameCode).Scan(&n)
	return n > 0
}

func (r *MaterialRepo) MatNameExistsByCode(ctx context.Context, matNameCode string) bool {
	var n int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM mat_name WHERE mat_name_code = $1`, matNameCode).Scan(&n)
	return n > 0
}

// ─── SpecSize ─────────────────────────────────────────────────────────────────

func (r *MaterialRepo) ListSpecSizes(ctx context.Context, matNameCode string) ([]models.SpecSize, error) {
	q := `SELECT spec_code, mat_name_code, spec_description FROM mat_spec ORDER BY spec_code`
	args := []interface{}{}
	if matNameCode != "" {
		q = `SELECT spec_code, mat_name_code, spec_description FROM mat_spec WHERE mat_name_code = $1 ORDER BY spec_code`
		args = append(args, matNameCode)
	}
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SpecSize
	for rows.Next() {
		var s models.SpecSize
		if err := rows.Scan(&s.SpecCode, &s.MatNameCode, &s.SpecDescription); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *MaterialRepo) GetSpecSize(ctx context.Context, matNameCode, specCode string) (models.SpecSize, error) {
	var s models.SpecSize
	err := r.db.QueryRow(ctx,
		`SELECT spec_code, mat_name_code, spec_description FROM mat_spec WHERE mat_name_code = $1 AND spec_code = $2`,
		matNameCode, specCode).
		Scan(&s.SpecCode, &s.MatNameCode, &s.SpecDescription)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}

func (r *MaterialRepo) UpsertSpecSize(ctx context.Context, req models.CreateSpecSizeRequest) (models.SpecSize, error) {
	_, err := r.db.Exec(ctx, `
		INSERT INTO mat_spec (spec_code, mat_name_code, spec_description) VALUES ($1, $2, $3)
		ON CONFLICT (mat_name_code, spec_code) DO UPDATE SET spec_description = EXCLUDED.spec_description`,
		req.SpecCode, req.MatNameCode, req.SpecDescription)
	if err != nil {
		return models.SpecSize{}, err
	}
	return models.SpecSize{SpecCode: req.SpecCode, MatNameCode: req.MatNameCode, SpecDescription: req.SpecDescription}, nil
}

func (r *MaterialRepo) DeleteSpecSize(ctx context.Context, matNameCode, specCode string) error {
	var count int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM material_code WHERE mat_name_code = $1 AND spec_code = $2`, matNameCode, specCode).Scan(&count)
	if count > 0 {
		return ErrHasChildren
	}
	tag, err := r.db.Exec(ctx, `DELETE FROM mat_spec WHERE mat_name_code = $1 AND spec_code = $2`, matNameCode, specCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MaterialRepo) SpecSizeExists(ctx context.Context, matNameCode, specCode string) bool {
	var n int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM mat_spec WHERE mat_name_code = $1 AND spec_code = $2`, matNameCode, specCode).Scan(&n)
	return n > 0
}

// ─── Brand ────────────────────────────────────────────────────────────────────

func (r *MaterialRepo) ListBrands(ctx context.Context) ([]models.Brand, error) {
	rows, err := r.db.Query(ctx, `SELECT brand_code, brand_name FROM brand ORDER BY brand_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Brand
	for rows.Next() {
		var b models.Brand
		if err := rows.Scan(&b.BrandCode, &b.BrandName); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *MaterialRepo) GetBrand(ctx context.Context, code string) (models.Brand, error) {
	var b models.Brand
	err := r.db.QueryRow(ctx, `SELECT brand_code, brand_name FROM brand WHERE brand_code = $1`, code).
		Scan(&b.BrandCode, &b.BrandName)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, ErrNotFound
	}
	return b, err
}

func (r *MaterialRepo) UpsertBrand(ctx context.Context, req models.CreateBrandRequest) (models.Brand, error) {
	_, err := r.db.Exec(ctx, `
		INSERT INTO brand (brand_code, brand_name) VALUES ($1, $2)
		ON CONFLICT (brand_code) DO UPDATE SET brand_name = EXCLUDED.brand_name`,
		req.BrandCode, req.BrandName)
	if err != nil {
		return models.Brand{}, err
	}
	return models.Brand{BrandCode: req.BrandCode, BrandName: req.BrandName}, nil
}

func (r *MaterialRepo) DeleteBrand(ctx context.Context, code string) error {
	var count int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM material_code WHERE brand_code = $1`, code).Scan(&count)
	if count > 0 {
		return ErrHasChildren
	}
	tag, err := r.db.Exec(ctx, `DELETE FROM brand WHERE brand_code = $1`, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MaterialRepo) BrandExists(ctx context.Context, code string) bool {
	var n int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM brand WHERE brand_code = $1`, code).Scan(&n)
	return n > 0
}

// ─── MaterialCode ─────────────────────────────────────────────────────────────

func (r *MaterialRepo) ListMaterialCodes(ctx context.Context) ([]models.MaterialCode, error) {
	rows, err := r.db.Query(ctx, `
		SELECT mat_code, group_code, subgroup_code, mat_name_code, spec_code, brand_code, unit_code, is_active, created_at
		FROM material_code ORDER BY mat_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.MaterialCode
	for rows.Next() {
		var m models.MaterialCode
		if err := rows.Scan(&m.MatCode, &m.GroupCode, &m.SubgroupCode, &m.MatNameCode,
			&m.SpecCode, &m.BrandCode, &m.UnitCode, &m.IsActive, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *MaterialRepo) GetMaterialCode(ctx context.Context, matCode string) (models.MaterialCode, error) {
	var m models.MaterialCode
	err := r.db.QueryRow(ctx, `
		SELECT mat_code, group_code, subgroup_code, mat_name_code, spec_code, brand_code, unit_code, is_active, created_at
		FROM material_code WHERE mat_code = $1`, matCode).
		Scan(&m.MatCode, &m.GroupCode, &m.SubgroupCode, &m.MatNameCode,
			&m.SpecCode, &m.BrandCode, &m.UnitCode, &m.IsActive, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

func (r *MaterialRepo) UpsertMaterialCode(ctx context.Context, m models.MaterialCode) (models.MaterialCode, error) {
	_, err := r.db.Exec(ctx, `
		INSERT INTO material_code (mat_code, group_code, subgroup_code, mat_name_code, spec_code, brand_code, unit_code, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (mat_code) DO UPDATE SET is_active = EXCLUDED.is_active`,
		m.MatCode, m.GroupCode, m.SubgroupCode, m.MatNameCode, m.SpecCode, m.BrandCode, m.UnitCode, m.IsActive)
	if err != nil {
		return models.MaterialCode{}, err
	}
	return r.GetMaterialCode(ctx, m.MatCode)
}

func (r *MaterialRepo) UpdateMaterialCodeActive(ctx context.Context, matCode string, isActive bool) error {
	tag, err := r.db.Exec(ctx, `UPDATE material_code SET is_active = $1 WHERE mat_code = $2`, isActive, matCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MaterialRepo) DeleteMaterialCode(ctx context.Context, matCode string) error {
	var count int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM inventory WHERE mat_code = $1`, matCode).Scan(&count)
	if count > 0 {
		return ErrHasChildren
	}
	tag, err := r.db.Exec(ctx, `DELETE FROM material_code WHERE mat_code = $1`, matCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MaterialRepo) GroupExists(ctx context.Context, code string) bool {
	var n int
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM mat_group WHERE group_code = $1`, code).Scan(&n)
	return n > 0
}
