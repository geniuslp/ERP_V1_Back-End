package models

import "time"

// ─── Auth / Users ────────────────────────────────────────────────────────────

type User struct {
	ID           int64          `json:"id" db:"id"`
	Username     string         `json:"username" db:"username"`
	FullName     string         `json:"full_name" db:"full_name"`
	Email        *string        `json:"email,omitempty" db:"email"`
	PasswordHash string         `json:"-" db:"password_hash"`
	LocationCode *string        `json:"location_code,omitempty" db:"location_code"`
	EmployeeCode *string        `json:"employee_code,omitempty" db:"employee_code"`
	Department   *string        `json:"department,omitempty" db:"department"`
	DeptCode     *string        `json:"dept_code,omitempty" db:"dept_code"`
	DeptName     *string        `json:"dept_name,omitempty"`
	IsActive     bool           `json:"is_active" db:"is_active"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at" db:"updated_at"`
	Roles        []string       `json:"roles,omitempty"`
	RoleInfos    []UserRoleInfo `json:"role_infos,omitempty"`
}

type CreateUserRequest struct {
	Username     string   `json:"username" validate:"required,max=50"`
	Password     string   `json:"password" validate:"required,min=6"`
	FullName     string   `json:"full_name" validate:"required,max=200"`
	Email        *string  `json:"email,omitempty"`
	LocationCode *string  `json:"location_code,omitempty"`
	EmployeeCode *string  `json:"employee_code,omitempty"`
	Department   *string  `json:"department,omitempty"`
	RoleCodes    []string `json:"role_codes,omitempty"`
}

type UpdateUserRequest struct {
	FullName     string  `json:"full_name" validate:"required,max=200"`
	Email        *string `json:"email,omitempty"`
	LocationCode *string `json:"location_code,omitempty"`
	EmployeeCode *string `json:"employee_code,omitempty"`
	Department   *string `json:"department,omitempty"`
	DeptCode     *string `json:"dept_code,omitempty"`
	RoleID       *int64  `json:"role_id,omitempty"`
}

type UpdateUserRolesRequest struct {
	RoleIDs []int64 `json:"role_ids" validate:"required"`
}

type UserRoleInfo struct {
	RoleID   int64  `json:"role_id"`
	RoleCode string `json:"role_code"`
	RoleName string `json:"role_name"`
}

type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

type ResetPasswordRequest struct {
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

// ─── Master: Location ────────────────────────────────────────────────────────

type Location struct {
	LocationCode string    `json:"location_code" db:"location_code"`
	LocationName string    `json:"location_name" db:"location_name"`
	LocationType string    `json:"location_type" db:"location_type"` // department | project | site
	ParentCode   *string   `json:"parent_code,omitempty" db:"parent_code"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type CreateLocationRequest struct {
	LocationCode string  `json:"location_code" validate:"required,max=20"`
	LocationName string  `json:"location_name" validate:"required,max=200"`
	LocationType string  `json:"location_type" validate:"required,oneof=department project site"`
	ParentCode   *string `json:"parent_code,omitempty"`
}

// ─── Master: Project ─────────────────────────────────────────────────────────

type ProjectFull struct {
	Id                    int        `json:"id"`
	ProjectCode           string     `json:"project_code"`
	ProjectName           string     `json:"project_name"`
	LocationCode          *string    `json:"location_code,omitempty"`           // free-text project address — no longer an FK/lookup against location.location_code
	DeptCode              *string    `json:"dept_code,omitempty"`               // "แผนก" — FK -> departments.dept_code
	DeptName              *string    `json:"dept_name,omitempty"`               // joined from departments.dept_name via dept_code
	OwnerID               *int64     `json:"owner_id,omitempty"`                // deprecated: superseded by responsible_person_name (kept, unused by new UI — not dropped, same approach as credit)
	OwnerName             *string    `json:"owner_name,omitempty"`              // joined from users.full_name via owner_id ("ผู้รับผิดชอบหลัก" — legacy)
	ProjectOwnerName      *string    `json:"project_owner_name,omitempty"`      // free text ("เจ้าของโครงการ"), distinct from owner_id/owner_name
	ResponsiblePersonName *string    `json:"responsible_person_name,omitempty"` // free text ("ผู้รับผิดชอบหลัก"), required on write — replaces the owner_id dropdown
	JobCodes              []string   `json:"job_codes,omitempty"`
	BudgetAmount          float64    `json:"budget_amount"`
	SpentAmount           float64    `json:"spent_amount"`
	PaidAmount            float64    `json:"paid_amount"`
	RemainingAmount       float64    `json:"remaining_amount"`
	ConsultantName        *string    `json:"consultant_name,omitempty"`
	ConsultantPhone       *string    `json:"consultant_phone,omitempty"` // "เบอร์ติดต่อของที่ปรึกษา", freeform
	StartDate             *time.Time `json:"start_date,omitempty"`
	EndDate               *time.Time `json:"end_date,omitempty"`
	Status                string     `json:"status"`
	IsActive              bool       `json:"is_active"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	CreatedBy             *int64     `json:"created_by,omitempty"`
	UpdatedBy             *int64     `json:"updated_by,omitempty"`
}

type CreateProjectReq struct {
	ProjectCode           string   `json:"project_code"`
	ProjectName           string   `json:"project_name"`
	LocationCode          *string  `json:"location_code,omitempty"` // free-text project address
	DeptCode              *string  `json:"dept_code,omitempty"`     // "แผนก" — FK -> departments.dept_code
	OwnerID               *int64   `json:"owner_id,omitempty"`      // deprecated, kept for backward compat — not required, not used to drive validation
	ProjectOwnerName      *string  `json:"project_owner_name,omitempty"`
	ResponsiblePersonName string   `json:"responsible_person_name" validate:"required"` // "ผู้รับผิดชอบหลัก" — required free text
	JobCodes              []string `json:"job_codes,omitempty"`                         // subset of MP/ME/MS/MF/MG/MH/G — validated server-side
	BudgetAmount          float64  `json:"budget_amount"`
	ConsultantName        *string  `json:"consultant_name,omitempty"`
	ConsultantPhone       *string  `json:"consultant_phone,omitempty"` // freeform, no validation
	StartDate             *string  `json:"start_date,omitempty"`
	EndDate               *string  `json:"end_date,omitempty"`
	Status                string   `json:"status"`
}

type UpdateProjectReq struct {
	ProjectCode           string   `json:"project_code"`
	ProjectName           string   `json:"project_name"`
	LocationCode          *string  `json:"location_code,omitempty"` // free-text project address
	DeptCode              *string  `json:"dept_code,omitempty"`     // "แผนก" — FK -> departments.dept_code
	OwnerID               *int64   `json:"owner_id,omitempty"`      // deprecated, kept for backward compat
	ProjectOwnerName      *string  `json:"project_owner_name,omitempty"`
	ResponsiblePersonName string   `json:"responsible_person_name" validate:"required"` // "ผู้รับผิดชอบหลัก" — required free text
	JobCodes              []string `json:"job_codes,omitempty"`                         // subset of MP/ME/MS/MF/MG/MH/G — validated server-side
	BudgetAmount          float64  `json:"budget_amount"`
	ConsultantName        *string  `json:"consultant_name,omitempty"`
	ConsultantPhone       *string  `json:"consultant_phone,omitempty"` // freeform, no validation
	StartDate             *string  `json:"start_date,omitempty"`       // "YYYY-MM-DD" — pgx casts to date automatically
	EndDate               *string  `json:"end_date,omitempty"`         // "YYYY-MM-DD"
	Status                string   `json:"status"`
	IsActive              bool     `json:"is_active"`
}

type ProjectListFilter struct {
	Search   string `query:"search"`
	Status   string `query:"status"`
	IsActive string `query:"is_active"`
	Page     int    `query:"page"`
	PageSize int    `query:"page_size"`
}

// ─── Master: Warehouse & Zone ────────────────────────────────────────────────

type Warehouse struct {
	WarehouseCode string    `json:"warehouse_code" db:"warehouse_code"`
	WarehouseName string    `json:"warehouse_name" db:"warehouse_name"`
	Address       *string   `json:"address,omitempty" db:"address"`
	IsActive      bool      `json:"is_active" db:"is_active"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type StorageZone struct {
	ZoneID        int64  `json:"zone_id" db:"zone_id"`
	WarehouseCode string `json:"warehouse_code" db:"warehouse_code"`
	ZoneCode      string `json:"zone_code" db:"zone_code"`
	ZoneName      string `json:"zone_name" db:"zone_name"`
	ZoneType      string `json:"zone_type" db:"zone_type"` // rack | shelf | bin | area
}

type CreateWarehouseRequest struct {
	WarehouseCode string  `json:"warehouse_code" validate:"required,max=20"`
	WarehouseName string  `json:"warehouse_name" validate:"required,max=200"`
	Address       *string `json:"address,omitempty"`
}

type CreateZoneRequest struct {
	ZoneCode string `json:"zone_code" validate:"required,max=30"`
	ZoneName string `json:"zone_name" validate:"required,max=100"`
	ZoneType string `json:"zone_type" validate:"required,oneof=rack shelf bin area"`
}

// ─── Master: Supplier ────────────────────────────────────────────────────────

type Supplier struct {
	SupplierName string    `json:"supplier_name" db:"supplier_name"`
	TaxID        *string   `json:"tax_id,omitempty" db:"tax_id"`
	Address      *string   `json:"address,omitempty" db:"address"`
	ContactName  *string   `json:"contact_name,omitempty" db:"contact_name"`
	ContactPhone *string   `json:"contact_phone,omitempty" db:"contact_phone"`
	ContactEmail *string   `json:"contact_email,omitempty" db:"contact_email"`
	PaymentTerms *string   `json:"payment_terms,omitempty" db:"payment_terms"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type CreateSupplierRequest struct {
	SupplierName string  `json:"supplier_name" validate:"required,max=200"`
	TaxID        *string `json:"tax_id,omitempty"`
	Address      *string `json:"address,omitempty"`
	ContactName  *string `json:"contact_name,omitempty"`
	ContactPhone *string `json:"contact_phone,omitempty"`
	ContactEmail *string `json:"contact_email,omitempty" validate:"omitempty,email"`
	PaymentTerms *string `json:"payment_terms,omitempty"`
}

// ─── Master: Material ────────────────────────────────────────────────────────

type MatGroup struct {
	Id        int       `json:"id" db:"id"`
	GroupCode string    `json:"group_code" db:"group_code"`
	GroupName string    `json:"group_name" db:"group_name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedBy *int64    `json:"created_by,omitempty" db:"created_by"`
	UpdatedBy *int64    `json:"updated_by,omitempty" db:"updated_by"`
}

type CreateGroupRequest struct {
	GroupCode string `json:"group_code" validate:"required,max=20"`
	GroupName string `json:"group_name" validate:"required,max=200"`
	IsActive  bool   `json:"is_active"`
}

type UpdateGroupRequest struct {
	GroupName string `json:"group_name" validate:"required,max=200"`
}

type Subgroup struct {
	SubgroupCode string `json:"subgroup_code" db:"subgroup_code"`
	GroupCode    string `json:"group_code" db:"group_code"`
	SubgroupName string `json:"subgroup_name" db:"subgroup_name"`
}

type CreateSubgroupRequest struct {
	SubgroupCode string `json:"subgroup_code" validate:"required,max=20"`
	GroupCode    string `json:"group_code" validate:"required,max=20"`
	SubgroupName string `json:"subgroup_name" validate:"required,max=200"`
}

type UpdateSubgroupRequest struct {
	SubgroupName string `json:"subgroup_name" validate:"required,max=200"`
}

type MatName struct {
	MatNameCode  string  `json:"mat_name_code" db:"mat_name_code"`
	SubgroupCode string  `json:"subgroup_code" db:"subgroup_code"`
	MatNameTH    string  `json:"mat_name_th" db:"mat_name_th"`
	MatNameEN    *string `json:"mat_name_en,omitempty" db:"mat_name_en"`
}

type CreateMatNameRequest struct {
	MatNameCode  string  `json:"mat_name_code" validate:"required,max=20"`
	SubgroupCode string  `json:"subgroup_code" validate:"required,max=20"`
	MatNameTH    string  `json:"mat_name_th" validate:"required,max=500"`
	MatNameEN    *string `json:"mat_name_en,omitempty"`
}

type UpdateMatNameRequest struct {
	MatNameTH string  `json:"mat_name_th" validate:"required,max=500"`
	MatNameEN *string `json:"mat_name_en,omitempty"`
}

type SpecSize struct {
	SpecCode        string `json:"spec_code" db:"spec_code"`
	MatNameCode     string `json:"mat_name_code" db:"mat_name_code"`
	SpecDescription string `json:"spec_description" db:"spec_description"`
}

type CreateSpecSizeRequest struct {
	SpecCode        string `json:"spec_code" validate:"required,max=20"`
	MatNameCode     string `json:"mat_name_code" validate:"required,max=20"`
	SpecDescription string `json:"spec_description" validate:"required,max=500"`
}

type UpdateSpecSizeRequest struct {
	SpecDescription string `json:"spec_description" validate:"required,max=500"`
}

type Brand struct {
	BrandCode string `json:"brand_code" db:"brand_code"`
	BrandName string `json:"brand_name" db:"brand_name"`
}

type CreateBrandRequest struct {
	BrandCode string `json:"brand_code" validate:"required,max=20"`
	BrandName string `json:"brand_name" validate:"required,max=200"`
}

type UpdateBrandRequest struct {
	BrandName string `json:"brand_name" validate:"required,max=200"`
}

type Unit struct {
	UnitCode string `json:"unit_code" db:"unit_code"`
	UnitName string `json:"unit_name" db:"unit_name"`
}

type CreateUnitRequest struct {
	UnitCode string `json:"unit_code" validate:"required,max=20"`
	UnitName string `json:"unit_name" validate:"required,max=200"`
}

type UpdateUnitRequest struct {
	UnitName string `json:"unit_name" validate:"required,max=200"`
}

// ─── Master: Dropdown handler models ─────────────────────────────────────────
// These are used by the dedicated CRUD handlers under /master/*
// The "slim" structs above (Unit, Subgroup, etc.) are kept for the matcode service.

type UnitFull struct {
	Id        int       `json:"id"`
	UnitCode  string    `json:"unit_code"`
	UnitName  string    `json:"unit_name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy *int64    `json:"created_by,omitempty"`
	UpdatedBy *int64    `json:"updated_by,omitempty"`
}
type CreateUnitReq struct {
	UnitCode string `json:"unit_code"`
	UnitName string `json:"unit_name"`
	IsActive bool   `json:"is_active"`
}
type UpdateUnitReq struct {
	UnitName string `json:"unit_name"`
}

type WarehouseFull struct {
	Id            int       `json:"id"`
	WarehouseCode string    `json:"warehouse_code"`
	WarehouseName string    `json:"warehouse_name"`
	Address       *string   `json:"address,omitempty"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	CreatedBy     *int64    `json:"created_by,omitempty"`
	UpdatedBy     *int64    `json:"updated_by,omitempty"`
}
type CreateWarehouseReq struct {
	WarehouseCode string  `json:"warehouse_code"`
	WarehouseName string  `json:"warehouse_name"`
	Address       *string `json:"address,omitempty"`
	IsActive      bool    `json:"is_active"`
}
type UpdateWarehouseReq struct {
	WarehouseName string  `json:"warehouse_name"`
	Address       *string `json:"address,omitempty"`
}

type StorageZoneFull struct {
	Id          int       `json:"id"`
	WarehouseId int       `json:"warehouse_id"`
	ZoneCode    string    `json:"zone_code"`
	ZoneName    string    `json:"zone_name"`
	ZoneType    *string   `json:"zone_type,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   *int64    `json:"created_by,omitempty"`
	UpdatedBy   *int64    `json:"updated_by,omitempty"`
}
type CreateZoneReq struct {
	WarehouseId int     `json:"warehouse_id"`
	ZoneCode    string  `json:"zone_code"`
	ZoneName    string  `json:"zone_name"`
	ZoneType    *string `json:"zone_type,omitempty"`
}
type UpdateZoneReq struct {
	ZoneName string  `json:"zone_name"`
	ZoneType *string `json:"zone_type,omitempty"`
}

type LocationFull struct {
	Id           int       `json:"id"`
	LocationCode string    `json:"location_code"`
	LocationName string    `json:"location_name"`
	LocationType string    `json:"location_type"`
	ParentId     *int      `json:"parent_id,omitempty"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	CreatedBy    *int64    `json:"created_by,omitempty"`
	UpdatedBy    *int64    `json:"updated_by,omitempty"`
}
type CreateLocationReq struct {
	LocationCode string `json:"location_code"`
	LocationName string `json:"location_name"`
	LocationType string `json:"location_type"`
	ParentId     *int   `json:"parent_id,omitempty"`
	IsActive     bool   `json:"is_active"`
}
type UpdateLocationReq struct {
	LocationCode string `json:"location_code"`
	LocationName string `json:"location_name"`
	LocationType string `json:"location_type"`
	ParentId     *int   `json:"parent_id,omitempty"`
	IsActive     bool   `json:"is_active"`
}

type SupplierFull struct {
	Id               int       `json:"id"`
	SupplierName     string    `json:"supplier_name"`
	TaxID            *string   `json:"tax_id,omitempty"`
	Address          *string   `json:"address,omitempty"`
	ContactName      *string   `json:"contact_name,omitempty"`
	ContactPhone     *string   `json:"contact_phone,omitempty"`
	ContactEmail     *string   `json:"contact_email,omitempty"`
	OfficePhone      *string   `json:"office_phone,omitempty"`
	Fax              *string   `json:"fax,omitempty"`
	SalesPerson      *string   `json:"sales_person,omitempty"`
	SalesPersonPhone *string   `json:"sales_person_phone,omitempty"`
	Currency         *string   `json:"currency,omitempty"`
	PaymentTerms     *string   `json:"payment_terms,omitempty"`
	Remarks          *string   `json:"remarks,omitempty"`
	IsActive         bool      `json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	CreatedBy        *int64    `json:"created_by,omitempty"`
	UpdatedBy        *int64    `json:"updated_by,omitempty"`
}
type CreateSupplierReq struct {
	SupplierName     string  `json:"supplier_name"`
	TaxID            *string `json:"tax_id,omitempty"`
	Address          *string `json:"address,omitempty"`
	ContactName      *string `json:"contact_name,omitempty"`
	ContactPhone     *string `json:"contact_phone,omitempty"`
	ContactEmail     *string `json:"contact_email,omitempty"`
	SalesPerson      *string `json:"sales_person,omitempty"`
	SalesPersonPhone *string `json:"sales_person_phone,omitempty"`
	PaymentTerms     *string `json:"payment_terms,omitempty"`
	Remarks          *string `json:"remarks,omitempty"`
	IsActive         bool    `json:"is_active"`
}
type UpdateSupplierReq struct {
	SupplierName     string  `json:"supplier_name"`
	TaxID            *string `json:"tax_id,omitempty"`
	Address          *string `json:"address,omitempty"`
	ContactName      *string `json:"contact_name,omitempty"`
	ContactPhone     *string `json:"contact_phone,omitempty"`
	ContactEmail     *string `json:"contact_email,omitempty"`
	OfficePhone      *string `json:"office_phone,omitempty"`
	Fax              *string `json:"fax,omitempty"`
	SalesPerson      *string `json:"sales_person,omitempty"`
	SalesPersonPhone *string `json:"sales_person_phone,omitempty"`
	Currency         *string `json:"currency,omitempty"`
	PaymentTerms     *string `json:"payment_terms,omitempty"`
	Remarks          *string `json:"remarks,omitempty"`
}

// BulkInsertSupplierLine is one supplier entry in a POST /supplier/bulk request.
// supplier_name is the only required field; everything else is optional and
// unvalidated. There is no supplier_code anymore — supplier.id (auto-increment
// PK) is the only identifier, assigned by the DB on insert.
type BulkInsertSupplierLine struct {
	SupplierName     string  `json:"supplier_name"`
	TaxID            *string `json:"tax_id,omitempty"`
	Address          *string `json:"address,omitempty"`
	ContactName      *string `json:"contact_name,omitempty"`
	ContactPhone     *string `json:"contact_phone,omitempty"`
	ContactEmail     *string `json:"contact_email,omitempty"`
	OfficePhone      *string `json:"office_phone,omitempty"`
	Fax              *string `json:"fax,omitempty"`
	PaymentTerms     *string `json:"payment_terms,omitempty"`
	Currency         *string `json:"currency,omitempty"`
	SalesPerson      *string `json:"sales_person,omitempty"`
	SalesPersonPhone *string `json:"sales_person_phone,omitempty"`
}

type BulkInsertSupplierRequest struct {
	Suppliers []BulkInsertSupplierLine `json:"suppliers"`
}

type BulkInsertSupplierResultLine struct {
	SupplierID   int64  `json:"supplier_id"`
	SupplierName string `json:"supplier_name"`
}

type BulkInsertSupplierResponse struct {
	Count     int                            `json:"count"`
	Suppliers []BulkInsertSupplierResultLine `json:"suppliers"`
}

type SubgroupFull struct {
	Id           int       `json:"id"`
	SubgroupCode string    `json:"subgroup_code"`
	GroupId      int       `json:"group_id"`
	SubgroupName string    `json:"subgroup_name"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	CreatedBy    *int64    `json:"created_by,omitempty"`
	UpdatedBy    *int64    `json:"updated_by,omitempty"`
}
type CreateSubgroupReq struct {
	SubgroupCode string `json:"subgroup_code"`
	GroupId      int    `json:"group_id"`
	SubgroupName string `json:"subgroup_name"`
	IsActive     bool   `json:"is_active"`
}
type UpdateSubgroupReq struct {
	SubgroupName string `json:"subgroup_name"`
}

type MatNameFull struct {
	Id          int       `json:"id"`
	MatNameCode string    `json:"mat_name_code"`
	SubgroupId  int       `json:"subgroup_id"`
	Name        string    `json:"mat_name"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   *int64    `json:"created_by,omitempty"`
	UpdatedBy   *int64    `json:"updated_by,omitempty"`
}
type CreateMatNameReq struct {
	MatNameCode string `json:"mat_name_code"`
	SubgroupId  int    `json:"subgroup_id"`
	Name        string `json:"mat_name"`
	IsActive    bool   `json:"is_active"`
}
type UpdateMatNameReq struct {
	Name string `json:"mat_name"`
}

type SpecSizeFull struct {
	Id              int       `json:"id"`
	SpecCode        string    `json:"spec_code"`
	MatNameId       int       `json:"mat_name_id"`
	SpecDescription string    `json:"spec_description"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	CreatedBy       *int64    `json:"created_by,omitempty"`
	UpdatedBy       *int64    `json:"updated_by,omitempty"`
}
type CreateSpecSizeReq struct {
	SpecCode        string `json:"spec_code"`
	MatNameId       int    `json:"mat_name_id"`
	SpecDescription string `json:"spec_description"`
	IsActive        bool   `json:"is_active"`
}
type UpdateSpecSizeReq struct {
	SpecDescription string `json:"spec_description"`
}

type BrandFull struct {
	Id        int       `json:"id"`
	BrandCode string    `json:"brand_code"`
	SpecId    int       `json:"spec_id"`
	BrandName string    `json:"brand_name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy *int64    `json:"created_by,omitempty"`
	UpdatedBy *int64    `json:"updated_by,omitempty"`
}
type CreateBrandReq struct {
	BrandCode string `json:"brand_code"`
	SpecId    int    `json:"spec_id"`
	BrandName string `json:"brand_name"`
	IsActive  bool   `json:"is_active"`
}
type UpdateBrandReq struct {
	BrandName string `json:"brand_name"`
}

type MaterialCode struct {
	MatCode      string    `json:"mat_code" db:"mat_code"`
	GroupCode    string    `json:"group_code" db:"group_code"`
	SubgroupCode string    `json:"subgroup_code" db:"subgroup_code"`
	MatNameCode  string    `json:"mat_name_code" db:"mat_name_code"`
	SpecCode     string    `json:"spec_code" db:"spec_code"`
	BrandCode    string    `json:"brand_code" db:"brand_code"`
	UnitCode     string    `json:"unit_code" db:"unit_code"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type CreateMaterialCodeRequest struct {
	GroupCode    string `json:"group_code" validate:"required,max=20"`
	SubgroupCode string `json:"subgroup_code" validate:"required,max=20"`
	MatNameCode  string `json:"mat_name_code" validate:"required,max=20"`
	SpecCode     string `json:"spec_code" validate:"required,max=20"`
	BrandCode    string `json:"brand_code" validate:"required,max=20"`
	UnitCode     string `json:"unit_code" validate:"required,max=20"`
}

type UpdateMaterialCodeRequest struct {
	IsActive bool `json:"is_active"`
}

type UpdateMaterialRequest struct {
	SubgroupName    string `json:"subgroup_name" validate:"required,max=200"`
	MatNameTH       string `json:"mat_name_th" validate:"required,max=500"`
	SpecDescription string `json:"spec_description" validate:"required,max=500"`
	BrandName       string `json:"brand_name" validate:"required,max=200"`
	UnitName        string `json:"unit_name" validate:"required,max=200"`
	IsActive        bool   `json:"is_active"`
}

type CreateMaterialRequest struct {
	GroupCode       string `json:"group_code" validate:"required,max=20"`
	SubgroupCode    string `json:"subgroup_code" validate:"required,max=20"`
	SubgroupName    string `json:"subgroup_name" validate:"required,max=200"`
	MatNameCode     string `json:"mat_name_code" validate:"required,max=20"`
	MatNameTH       string `json:"mat_name_th" validate:"required,max=500"`
	SpecCode        string `json:"spec_code" validate:"required,max=20"`
	SpecDescription string `json:"spec_description" validate:"required,max=500"`
	BrandCode       string `json:"brand_code" validate:"required,max=20"`
	BrandName       string `json:"brand_name" validate:"required,max=200"`
	UnitCode        string `json:"unit_code" validate:"required,max=20"`
	UnitName        string `json:"unit_name" validate:"required,max=200"`
}

type MaterialFull struct {
	MatCode         string  `json:"mat_code"`
	GroupName       string  `json:"group_name"`
	SubgroupName    string  `json:"subgroup_name"`
	MatNameTH       string  `json:"mat_name_th"`
	MatNameEN       *string `json:"mat_name_en,omitempty"`
	SpecDescription string  `json:"spec_description"`
	BrandName       string  `json:"brand_name"`
	UnitName        string  `json:"unit_name"`
	IsActive        bool    `json:"is_active"`
}

// MaterialSearchItem is one result of the type-ahead material search (Create PO combobox,
// and — when warehouse_code is passed — the Requisition item picker).
type MaterialSearchItem struct {
	MatCode   string   `json:"mat_code"`
	MatName   string   `json:"mat_name"`
	Unit      string   `json:"unit"`
	LastPrice *float64 `json:"last_price"`            // nullable
	CostCode  *string  `json:"cost_code"`             // nullable
	QtyOnHand *float64 `json:"qty_on_hand,omitempty"` // only populated when warehouse_code is passed
}

// MaterialDetailFull is the full id+name detail view for GET /master/materials/{code},
// joined across every material_code FK — used by the material edit page so it has both
// the display names and the ids needed to pre-select each cascading dropdown.
type MaterialDetailFull struct {
	ID              int64  `json:"id"`
	MatCode         string `json:"mat_code"`
	IsActive        bool   `json:"is_active"`
	GroupID         int64  `json:"group_id"`
	GroupName       string `json:"group_name"`
	SubgroupID      int64  `json:"subgroup_id"`
	SubgroupName    string `json:"subgroup_name"`
	MatNameID       int64  `json:"mat_name_id"`
	MatName         string `json:"mat_name"`
	SpecID          int64  `json:"spec_id"`
	SpecDescription string `json:"spec_description"`
	BrandID         int64  `json:"brand_id"`
	BrandName       string `json:"brand_name"`
	UnitID          int64  `json:"unit_id"`
	UnitName        string `json:"unit_name"`
}

// UpdateMaterialFullRequest is the body for PUT /master/materials/{code}. All fields are
// optional/partial — only provided fields are changed. mat_code is never accepted from the
// client — it is always derived server-side from group/subgroup/mat_name/spec/brand/unit
// codes (see handler) and cascaded to every referencing table when it changes.
type UpdateMaterialFullRequest struct {
	GroupID    *int64 `json:"group_id,omitempty"`
	SubgroupID *int64 `json:"subgroup_id,omitempty"`
	MatNameID  *int64 `json:"mat_name_id,omitempty"`
	SpecID     *int64 `json:"spec_id,omitempty"`
	BrandID    *int64 `json:"brand_id,omitempty"`
	UnitID     *int64 `json:"unit_id,omitempty"`
	IsActive   *bool  `json:"is_active,omitempty"`
}

type MaterialDetail struct {
	GroupCode        string  `json:"group_code"`
	SubgroupCode     string  `json:"subgroup_code"`
	MatNameCode      string  `json:"Mat_name_code"`
	MatCode          string  `json:"mat_code"`
	MatNameTH        string  `json:"mat_name_th"`
	SpecDescription  *string `json:"spec_description"`
	SpecCode         *string `json:"spec_code"`
	BrandCode        *string `json:"brand_code"`
	BrandName        *string `json:"brand_name"`
	UnitCode         string  `json:"unit_code"`
	UnitName         string  `json:"unit_name"`
	IsActive         bool    `json:"is_active"`
	CostCode         *string `json:"cost_code"`
	CostSubgroupName *string `json:"cost_subgroup_name"`
	CostSubgroupID   *int64  `json:"cost_subgroup_id,omitempty"`
	// StockOnHand is only meaningful when project_code was passed to GetAllMaterial —
	// otherwise it stays 0. LEFT JOIN project_stock, per-project reference qty (never a
	// deduction source — see CLAUDE.md's inventory/stock_item vs project_stock split).
	StockOnHand float64 `json:"stock_on_hand,omitempty"`
}

// ─── Inventory ───────────────────────────────────────────────────────────────

type Inventory struct {
	InventoryID   int64    `json:"inventory_id" db:"inventory_id"`
	MatCode       string   `json:"mat_code" db:"mat_code"`
	WarehouseCode string   `json:"warehouse_code" db:"warehouse_code"`
	ZoneID        *int64   `json:"zone_id,omitempty" db:"zone_id"`
	QtyOnHand     float64  `json:"qty_on_hand" db:"qty_on_hand"`
	QtyReserved   float64  `json:"qty_reserved" db:"qty_reserved"`
	QtyOnOrder    float64  `json:"qty_on_order" db:"qty_on_order"`
	QtyAvailable  float64  `json:"qty_available" db:"qty_available"`
	ReorderPoint  *float64 `json:"reorder_point,omitempty" db:"reorder_point"`
	MinStock      *float64 `json:"min_stock,omitempty" db:"min_stock"`
	MaxStock      *float64 `json:"max_stock,omitempty" db:"max_stock"`
	StockStatus   string   `json:"stock_status" db:"stock_status"` // OK | LOW | CRITICAL
}

type InventoryTransaction struct {
	TxnID         int64     `json:"txn_id" db:"txn_id"`
	TxnNo         string    `json:"txn_no" db:"txn_no"`
	TxnType       string    `json:"txn_type" db:"txn_type"`
	MatCode       string    `json:"mat_code" db:"mat_code"`
	FromWarehouse *string   `json:"from_warehouse,omitempty" db:"from_warehouse"`
	ToWarehouse   *string   `json:"to_warehouse,omitempty" db:"to_warehouse"`
	Qty           float64   `json:"qty" db:"qty"`
	RefDocType    *string   `json:"ref_doc_type,omitempty" db:"ref_doc_type"`
	RefDocNo      *string   `json:"ref_doc_no,omitempty" db:"ref_doc_no"`
	LocationCode  *string   `json:"location_code,omitempty" db:"location_code"`
	Reason        *string   `json:"reason,omitempty" db:"reason"`
	TxnDate       time.Time `json:"txn_date" db:"txn_date"`
	CreatedBy     int64     `json:"created_by" db:"created_by"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type CreateTransactionRequest struct {
	TxnType       string  `json:"txn_type" validate:"required,oneof=ISSUE RETURN TRANSFER_OUT TRANSFER_IN ADJUST_PLUS ADJUST_MINUS BORROW_OUT BORROW_RETURN"`
	MatCode       string  `json:"mat_code" validate:"required"`
	FromWarehouse *string `json:"from_warehouse,omitempty"`
	ToWarehouse   *string `json:"to_warehouse,omitempty"`
	FromZoneID    *int64  `json:"from_zone_id,omitempty"`
	ToZoneID      *int64  `json:"to_zone_id,omitempty"`
	Qty           float64 `json:"qty" validate:"required,gt=0"`
	LocationCode  *string `json:"location_code,omitempty"`
	Reason        *string `json:"reason,omitempty"`
	TxnDate       *string `json:"txn_date,omitempty"`
}

// ─── Purchase Request ─────────────────────────────────────────────────────────

type PurchaseRequest struct {
	PRID          int64         `json:"pr_id" db:"pr_id"`
	PRNo          string        `json:"pr_no" db:"pr_no"`
	PRDate        time.Time     `json:"pr_date" db:"pr_date"`
	RequestedBy   int64         `json:"requested_by" db:"requested_by"`
	LocationText  string        `json:"location_text" db:"location_text"`
	WarehouseCode *string       `json:"warehouse_code,omitempty" db:"warehouse_code"`
	WarehouseName *string       `json:"warehouse_name,omitempty" db:"warehouse_name"`
	ProjectCode   *string       `json:"project_code,omitempty" db:"project_code"`
	DeptCode      *string       `json:"dept_code,omitempty" db:"dept_code"`
	RequiredDate  *string       `json:"required_date,omitempty" db:"required_date"`
	Status        string        `json:"status" db:"status"`
	Priority      string        `json:"priority" db:"priority"`
	OrderType     string        `json:"order_type" db:"order_type"`
	PRType        string        `json:"pr_type" db:"pr_type"`
	JobCode       string        `json:"job_code" db:"job_code"` // required — one of the 12 fixed job type codes, see handlers.JobTypes
	Remarks       *string       `json:"remarks,omitempty" db:"remarks"`
	CreatedAt     time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at" db:"updated_at"`
	MemoID        *int64        `json:"memo_id,omitempty" db:"memo_id"`
	MemoNo        *string       `json:"memo_no,omitempty"`
	MemoTitle     *string       `json:"memo_title,omitempty"`
	Lines         []PRLine      `json:"lines,omitempty"`
	Attachments   PRAttachments `json:"attachments"`
}

type PRAttachments struct {
	PR   []PRAttachment    `json:"pr"`
	Memo *[]MemoAttachment `json:"memo,omitempty"`
}

type PRLine struct {
	LineID       int64   `json:"line_id" db:"line_id"`
	PRID         int64   `json:"pr_id" db:"pr_id"`
	LineNo       int     `json:"line_no" db:"line_no"`
	MatCode      string  `json:"mat_code" db:"mat_code"`
	QtyRequested float64 `json:"qty_requested" db:"qty_requested"`
	QtyReserved  float64 `json:"qty_reserved" db:"qty_reserved"`
	QtyToOrder   float64 `json:"qty_to_order" db:"qty_to_order"`
	QtyOrdered   float64 `json:"qty_ordered" db:"qty_ordered"`
	Remarks      *string `json:"remarks,omitempty" db:"remarks"`
	Status       string  `json:"status" db:"status"`
	DeductStock  bool    `json:"deduct_stock" db:"deduct_stock"`

	// Cost Code — chosen per line item, not tied to the material.
	CostSubgroupID   *int64  `json:"cost_subgroup_id,omitempty" db:"cost_subgroup_id"`
	CostCode         *string `json:"cost_code,omitempty"`          // resolved combined code, e.g. "LE30300"
	CostSubgroupName *string `json:"cost_subgroup_name,omitempty"` // resolved subgroup_name
}

// ReferencedPO is a PO claim against a PR line, returned in PRLineWithPOStatus.referenced_pos.
type ReferencedPO struct {
	POID int64   `json:"po_id"`
	PONo string  `json:"po_no"`
	Qty  float64 `json:"qty"`
}

// PriceHistoryItem is one past purchase of a mat_code, returned in PRLineWithPOStatus.price_history.
type PriceHistoryItem struct {
	Price        float64 `json:"price"`
	Date         string  `json:"date"` // "2006-01-02"
	Qty          float64 `json:"qty"`
	SupplierName string  `json:"supplier_name"`
	PONo         string  `json:"po_no"`
	SourcePRNo   *string `json:"source_pr_no"` // nullable: PO not linked to a PR
	ProjectCode  *string `json:"project_code"` // nullable
	ProjectName  *string `json:"project_name"` // nullable
}

// PRLineWithPOStatus is a purchase_request_line row enriched with PO claim status.
type PRLineWithPOStatus struct {
	PRLineID      int64          `json:"pr_line_id"`
	LineNo        int            `json:"line_no"`
	MatCode       string         `json:"mat_code"`
	MatName       string         `json:"mat_name"`
	Unit          string         `json:"unit"`
	SpecName      *string        `json:"spec_name,omitempty"` // resolved spec_size.spec_description, same field name/join as GET /pr/{id}
	QtyRequested  float64        `json:"qty_requested"`
	QtyReserved   float64        `json:"qty_reserved"`
	QtyOrdered    float64        `json:"qty_ordered"`
	QtyRemaining  float64        `json:"qty_remaining"`
	LineStatus    string         `json:"line_status"`
	ReferencedPOs []ReferencedPO `json:"referenced_pos"`

	LastPrice     *float64           `json:"last_price"`      // nullable: no history
	LastPriceDate *string            `json:"last_price_date"` // nullable, "2006-01-02"
	PriceHistory  []PriceHistoryItem `json:"price_history"`

	// Cost Code — cost_subgroup_id is carried over to the PO line automatically when this PR
	// line is converted. job_code/job_name are the primary display fields for the frontend
	// (job level, not subgroup level); cost_code/cost_subgroup_name are kept for other callers.
	CostSubgroupID   *int64  `json:"cost_subgroup_id,omitempty"`
	CostCode         *string `json:"cost_code,omitempty"`          // resolved combined code, e.g. "LE30300"
	CostSubgroupName *string `json:"cost_subgroup_name,omitempty"` // resolved cost_subgroup.subgroup_name
	JobCode          *string `json:"job_code,omitempty"`           // resolved cost_job.job_code, e.g. "P"
	JobName          *string `json:"job_name,omitempty"`           // resolved cost_job.job_name, e.g. "Metal Structure"
}

type PRLinesWithPOStatusResponse struct {
	PRNo     string               `json:"pr_no"`
	PRStatus string               `json:"pr_status"`
	Lines    []PRLineWithPOStatus `json:"lines"`
}

type CreatePRRequest struct {
	PRNo          string                 `json:"pr_no" validate:"required"`
	PRDate        string                 `json:"pr_date" validate:"required"`
	RequestedBy   int64                  `json:"requested_by" validate:"required"`
	LocationText  string                 `json:"location_text" validate:"required"`
	WarehouseCode *string                `json:"warehouse_code,omitempty"`
	RequiredDate  *string                `json:"required_date,omitempty"`
	ProjectCode   *string                `json:"project_code,omitempty"`
	DeptCode      *string                `json:"dept_code,omitempty" db:"dept_code"`
	MemoID        *int64                 `json:"memo_id,omitempty"`
	OrderType     string                 `json:"order_type,omitempty"` // "stock" | "cost" — defaults to "stock"
	PRType        string                 `json:"pr_type,omitempty"`    // "PO_WO" | "PO_ONLY" | "WO_ONLY" — defaults to "PO_WO"
	JobCode       string                 `json:"job_code" validate:"required"` // required — one of the 12 fixed job type codes, see handlers.JobTypes
	Status        string                 `json:"status"`
	Remarks       *string                `json:"remarks,omitempty"`
	CreatedBy     int64                  `json:"created_by" validate:"required"`
	Lines         []CreatePRLine         `json:"lines" validate:"required,min=1"`
	Attachments   []AddAttachmentRequest `json:"attachments,omitempty"`
}

type CreatePRLine struct {
	LineNo         int     `json:"line_no"`
	MatCode        string  `json:"mat_code" validate:"required"`
	QtyRequested   float64 `json:"qty_requested" validate:"required,gt=0"`
	CostSubgroupID *int64  `json:"cost_subgroup_id,omitempty"`
	// DeductStock: whether Submit should reserve this line against stock_item.qty.
	// Omit (nil) to keep today's default behavior (true).
	DeductStock *bool `json:"deduct_stock,omitempty"`
}

// UpdatePRRequest edits a PR's header and lines. Only allowed while status='DRAFT' (including
// freshly-reopened PRs). pr_no, memo_id, and status are not editable here — pr_no is the
// business key, memo_id linkage and status transitions go through their own dedicated flows.
type UpdatePRRequest struct {
	PRDate        string         `json:"pr_date" validate:"required"`
	RequestedBy   int64          `json:"requested_by" validate:"required"`
	LocationText  string         `json:"location_text" validate:"required"`
	WarehouseCode *string        `json:"warehouse_code,omitempty"`
	RequiredDate  *string        `json:"required_date,omitempty"`
	ProjectCode   *string        `json:"project_code,omitempty"`
	DeptCode      *string        `json:"dept_code,omitempty" db:"dept_code"`
	OrderType     string         `json:"order_type,omitempty"` // "stock" | "cost"
	PRType        string         `json:"pr_type,omitempty"`    // "PO_WO" | "PO_ONLY" | "WO_ONLY"
	JobCode       string         `json:"job_code,omitempty"` // required overall — omit to keep the PR's current value, see PRHandler.Update
	Remarks       *string        `json:"remarks,omitempty"`
	Lines         []UpdatePRLine `json:"lines" validate:"required,min=1"`
}

// UpdatePRLine's ID identifies an existing purchase_request_line row to update in place.
// Omit/nil means this is a new line to insert. Any existing line belonging to the PR that
// isn't present in the request's Lines (by ID) is treated as removed — and removal is
// rejected if a purchase_order_line still references it via pr_line_id.
type UpdatePRLine struct {
	ID             *int64  `json:"id,omitempty"`
	LineNo         int     `json:"line_no"`
	MatCode        string  `json:"mat_code" validate:"required"`
	QtyRequested   float64 `json:"qty_requested" validate:"required,gt=0"`
	CostSubgroupID *int64  `json:"cost_subgroup_id,omitempty"`
	// DeductStock: whether Submit should reserve this line against stock_item.qty.
	// Omit (nil) to keep today's default behavior (true).
	DeductStock *bool `json:"deduct_stock,omitempty"`
}

// ─── Purchase Order ───────────────────────────────────────────────────────────

type PurchaseOrder struct {
	POID             int64     `json:"po_id" db:"po_id"`
	PONo             string    `json:"po_no" db:"po_no"`
	PODate           time.Time `json:"po_date" db:"po_date"`
	SupplierID       *int64    `json:"supplier_id,omitempty" db:"supplier_id"`
	SupplierName     string    `json:"supplier_name,omitempty"`
	PRID             *int64    `json:"pr_id,omitempty" db:"pr_id"`
	PRNo             *string   `json:"pr_no,omitempty"`
	RFQID            *int64    `json:"rfq_id,omitempty" db:"rfq_id"`
	Ref              *string   `json:"ref,omitempty" db:"ref"`
	LocationText     *string   `json:"location_text,omitempty" db:"location_text"`
	WarehouseCode    *string   `json:"warehouse_code,omitempty" db:"warehouse_code"`
	WarehouseAddress *string   `json:"warehouse_address,omitempty"`
	ProjectCode      *string   `json:"project_code,omitempty" db:"project_code"`
	RequestedBy      *int64    `json:"requested_by,omitempty" db:"requested_by"`
	RequestedByName  string    `json:"requested_by_name,omitempty"`
	ApproverID       *int64    `json:"approver_id,omitempty"`
	Currency         string    `json:"currency" db:"currency"`
	TotalAmount      float64   `json:"total_amount" db:"total_amount"`
	VATAmount        float64   `json:"vat_amount" db:"vat_amount"`
	NetAmount        float64   `json:"net_amount" db:"net_amount"`
	ExpectedDate     *string   `json:"expected_date,omitempty" db:"expected_date"`
	UseDiscount      *bool     `json:"use_discount,omitempty"`
	DiscountType     *string   `json:"discount_type,omitempty"`
	DiscountAmount   *float64  `json:"discount_amount,omitempty"`
	UseVAT           *bool     `json:"use_vat,omitempty"`
	UseWHT           *bool     `json:"use_wht,omitempty"`
	WHTAmount        *float64  `json:"wht_amount,omitempty"`
	Status           string    `json:"status" db:"status"`
	StatusReceive    string    `json:"status_receive" db:"status_receive"`
	OrderType        string    `json:"order_type" db:"order_type"`
	JobCode          string    `json:"job_code" db:"job_code"` // required — one of the 12 fixed job type codes, see handlers.JobTypes. Replaces the old work_type (P|E|S|F|G|H) column.
	PaymentTerms     *string   `json:"payment_terms,omitempty" db:"payment_terms"`
	Remarks          *string   `json:"remarks,omitempty" db:"remarks"`
	ReceiverName     *string   `json:"receiver_name,omitempty" db:"receiver_name"`
	ReceiverPhone    *string   `json:"receiver_phone,omitempty" db:"receiver_phone"`
	CreatedBy        int64     `json:"created_by" db:"created_by"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
	CanEditApproved  bool      `json:"can_edit_approved"`
	// RevisionRound is how many times this PO has been edited-and-resent for re-approval
	// (COUNT of po_edit_log rows for this PO). 0 = original, never edited. po_no itself never
	// changes; the frontend composes a display suffix like "#R2" from this when > 0.
	RevisionRound int           `json:"revision_round"`
	OfficePhone   *string       `json:"office_phone,omitempty"`
	SalesPerson   *string       `json:"sales_person,omitempty"`
	ContactEmail  *string       `json:"contact_email,omitempty"`
	ContactPhone  *string       `json:"contact_phone,omitempty"`
	Lines         []POLine      `json:"lines,omitempty"`
	Attachments   POAttachments `json:"attachments"`
}

type POAttachments struct {
	PO   []POAttachment    `json:"po"`
	PR   *[]PRAttachment   `json:"pr,omitempty"`
	Memo *[]MemoAttachment `json:"memo,omitempty"`
}

type POLine struct {
	LineID            int64    `json:"line_id" db:"line_id"`
	POID              int64    `json:"po_id" db:"po_id"`
	LineNo            int      `json:"line_no" db:"line_no"`
	MatCode           string   `json:"mat_code" db:"mat_code"`
	PRLineID          *int64   `json:"pr_line_id,omitempty" db:"pr_line_id"`
	QtyOrdered        float64  `json:"qty_ordered" db:"qty_ordered"`
	QtyReceived       float64  `json:"qty_received" db:"qty_received"`
	UnitPrice         float64  `json:"unit_price" db:"unit_price"`
	DiscType          string   `json:"disc_type" db:"disc_type"`
	Discount          float64  `json:"discount" db:"discount"`
	LineDiscount      float64  `json:"line_discount" db:"line_discount"`
	LineVAT           float64  `json:"line_vat" db:"line_vat"`
	LineWHT           float64  `json:"line_wht" db:"line_wht"`
	LineNet           float64  `json:"line_net" db:"line_net"`
	WhtRate           *float64 `json:"wht_rate,omitempty" db:"wht_rate"`
	Amount            float64  `json:"amount" db:"amount"`
	Description       *string  `json:"description,omitempty" db:"description"`
	Remarks           *string  `json:"remarks,omitempty" db:"remarks"`
	Status            string   `json:"status" db:"status"`
	// CostSubgroupID: explicit value wins; auto-filled from the source PR line's
	// cost_subgroup_id when pr_line_id is set and no explicit value is sent — same
	// precedence pattern as PurchaseOrder.JobCode's auto-fill from PR.
	CostSubgroupID    *int64   `json:"cost_subgroup_id,omitempty" db:"cost_subgroup_id"`
	MatName           *string  `json:"mat_name,omitempty"`
	SpecDescription   *string  `json:"spec,omitempty"`
	BrandName         *string  `json:"brand,omitempty"`
	CurrentStock      float64  `json:"current_stock"`
	PRLineQtyReserved *float64 `json:"pr_line_qty_reserved,omitempty"`
}

type CreatePORequest struct {
	SupplierID    int64          `json:"supplier_id" validate:"required"`
	PRID          *int64         `json:"pr_id,omitempty"`
	RFQID         *int64         `json:"rfq_id,omitempty"`
	LocationText  string         `json:"location_text" validate:"required"`
	WarehouseCode *string        `json:"warehouse_code,omitempty"`
	ProjectCode   *string        `json:"project_code,omitempty"`
	RequestedBy   *int64         `json:"requested_by,omitempty"`
	ApproverID    *int64         `json:"approver_id,omitempty"`
	Ref           *string        `json:"ref,omitempty"`
	Currency      string         `json:"currency"`
	ExpectedDate  *string        `json:"expected_date,omitempty"`
	PaymentTerms  *string        `json:"payment_terms,omitempty"`
	Remarks       *string        `json:"remarks,omitempty"`
	ReceiverName  *string        `json:"receiver_name,omitempty"`
	ReceiverPhone *string        `json:"receiver_phone,omitempty"`
	Status        string         `json:"status" validate:"omitempty,oneof=DRAFT PENDING_APPROVAL"`
	OrderType     string         `json:"order_type,omitempty"` // "stock" | "cost" — defaults to "stock"
	// JobCode: optional here — if omitted, auto-filled from the source PR's job_code when
	// pr_id is set (never silently overwrites an explicit value). Required overall; if
	// omitted with no pr_id (or the PR has no usable value), Create/Update returns 400.
	JobCode       *string        `json:"job_code,omitempty"`
	UseDiscount   *bool          `json:"use_discount,omitempty"`
	DiscountType  *string        `json:"discount_type,omitempty" validate:"omitempty,oneof=pct amt"`
	UseVAT        *bool          `json:"use_vat,omitempty"`
	UseWHT        *bool          `json:"use_wht,omitempty"`
	Lines         []CreatePOLine `json:"lines" validate:"required,min=1,dive"`
}

type CreatePOLine struct {
	MatCode     string   `json:"mat_code" validate:"required"`
	PRLineID    *int64   `json:"pr_line_id,omitempty"`
	// CostSubgroupID: explicit value wins; if omitted and pr_line_id is set, auto-filled
	// from that PR line's cost_subgroup_id. Never silently overwrites an explicit value.
	CostSubgroupID *int64  `json:"cost_subgroup_id,omitempty"`
	QtyOrdered  float64  `json:"qty_ordered" validate:"required,gt=0"`
	UnitPrice   float64  `json:"unit_price" validate:"required,gte=0"`
	DiscType    string   `json:"disc_type" validate:"omitempty,oneof=pct amt"`
	Discount    float64  `json:"discount,omitempty"`
	WhtRate     *float64 `json:"wht_rate,omitempty"`
	Description *string  `json:"description,omitempty"`
	Remarks     *string  `json:"remarks,omitempty"`
}

type AddPOLinesRequest struct {
	Lines []CreatePOLine `json:"lines" validate:"required,min=1,dive"`
}

type UpdatePOLineRequest struct {
	Description *string `json:"description"`
	DiscType    *string `json:"disc_type" validate:"omitempty,oneof=pct amt"`
}

// EditApprovedPOLine is one line in an EditApprovedPORequest. Same shape as CreatePOLine plus
// an optional ID: omitted/null means "insert as a new line," present means "match against an
// existing purchase_order_line.id" so EditApprovedPO can diff per-line instead of replacing the
// whole line set (see EditApprovedPO's per-line rules re: qty_received).
type EditApprovedPOLine struct {
	ID          *int64   `json:"id,omitempty"`
	MatCode     string   `json:"mat_code" validate:"required"`
	PRLineID    *int64   `json:"pr_line_id,omitempty"`
	// CostSubgroupID: same precedence as CreatePOLine — explicit value wins, else
	// auto-filled from the referenced pr_line_id's cost_subgroup_id, else nil.
	CostSubgroupID *int64  `json:"cost_subgroup_id,omitempty"`
	QtyOrdered  float64  `json:"qty_ordered" validate:"required,gt=0"`
	UnitPrice   float64  `json:"unit_price" validate:"required,gte=0"`
	DiscType    string   `json:"disc_type" validate:"omitempty,oneof=pct amt"`
	Discount    float64  `json:"discount,omitempty"`
	WhtRate     *float64 `json:"wht_rate,omitempty"`
	Description *string  `json:"description,omitempty"`
	Remarks     *string  `json:"remarks,omitempty"`
}

// EditApprovedPORequest is the body for PUT /po/{id}/edit-approved — editing a PO that is
// already APPROVED. Replaces header fields wholesale (mirrors CreatePORequest, minus
// pr_id/rfq_id/status which cannot change post-approval) and requires a mandatory reason for
// po_edit_log. Lines are diffed per-line (see EditApprovedPOLine), not replaced wholesale —
// this is a breaking change from the old CreatePOLine-based shape: every line the frontend
// already has must now round-trip its `id`, or it will be (incorrectly) treated as a new line.
type EditApprovedPORequest struct {
	SupplierID    int64                `json:"supplier_id" validate:"required"`
	LocationText  string               `json:"location_text" validate:"required"`
	ProjectCode   *string              `json:"project_code,omitempty"`
	RequestedBy   *int64               `json:"requested_by,omitempty"`
	WarehouseCode *string              `json:"warehouse_code,omitempty"`
	Currency      string               `json:"currency"`
	ExpectedDate  *string              `json:"expected_date,omitempty"`
	PaymentTerms  *string              `json:"payment_terms,omitempty"`
	Remarks       *string              `json:"remarks,omitempty"`
	ReceiverName  *string              `json:"receiver_name,omitempty"`
	ReceiverPhone *string              `json:"receiver_phone,omitempty"`
	UseDiscount   *bool                `json:"use_discount,omitempty"`
	DiscountType  *string              `json:"discount_type,omitempty" validate:"omitempty,oneof=pct amt"`
	UseVAT        *bool                `json:"use_vat,omitempty"`
	UseWHT        *bool                `json:"use_wht,omitempty"`
	Lines         []EditApprovedPOLine `json:"lines" validate:"required,min=1,dive"`
	Reason        string               `json:"reason" validate:"required"`
}

// ─── GRN ─────────────────────────────────────────────────────────────────────

type GRN struct {
	GRNID         int64     `json:"grn_id" db:"grn_id"`
	GRNNo         string    `json:"grn_no" db:"grn_no"`
	GRNDate       time.Time `json:"grn_date" db:"grn_date"`
	POID          int64     `json:"po_id" db:"po_id"`
	WarehouseCode string    `json:"warehouse_code" db:"warehouse_code"`
	SupplierID    *int64    `json:"supplier_id,omitempty" db:"supplier_id"`
	DeliveryNote  *string   `json:"delivery_note,omitempty" db:"delivery_note"`
	Status        string    `json:"status" db:"status"`
	QualityStatus string    `json:"quality_status" db:"quality_status"`
	ReceivedBy    int64     `json:"received_by" db:"received_by"`
	Remarks       *string   `json:"remarks,omitempty" db:"remarks"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	Lines         []GRNLine `json:"lines,omitempty"`
}

type GRNLine struct {
	LineID         int64   `json:"line_id" db:"line_id"`
	GRNID          int64   `json:"grn_id" db:"grn_id"`
	LineNo         int     `json:"line_no" db:"line_no"`
	POLineID       int64   `json:"po_line_id" db:"po_line_id"`
	MatCode        string  `json:"mat_code" db:"mat_code"`
	ZoneID         *int64  `json:"zone_id,omitempty" db:"zone_id"`
	QtyReceived    float64 `json:"qty_received" db:"qty_received"`
	QtyAccepted    float64 `json:"qty_accepted" db:"qty_accepted"`
	QtyRejected    float64 `json:"qty_rejected" db:"qty_rejected"`
	QualityRemarks *string `json:"quality_remarks,omitempty" db:"quality_remarks"`
}

type CreateGRNRequest struct {
	POID          int64           `json:"po_id" validate:"required"`
	WarehouseCode string          `json:"warehouse_code" validate:"required"`
	DeliveryNote  *string         `json:"delivery_note,omitempty"`
	Remarks       *string         `json:"remarks,omitempty"`
	Lines         []CreateGRNLine `json:"lines" validate:"required,min=1,dive"`
}

type CreateGRNLine struct {
	POLineID       int64   `json:"po_line_id" validate:"required"`
	MatCode        string  `json:"mat_code" validate:"required"`
	ZoneID         *int64  `json:"zone_id,omitempty"`
	QtyReceived    float64 `json:"qty_received" validate:"required,gt=0"`
	QtyAccepted    float64 `json:"qty_accepted" validate:"gte=0"`
	QtyRejected    float64 `json:"qty_rejected" validate:"gte=0"`
	QualityRemarks *string `json:"quality_remarks,omitempty"`
}

// ─── Approval ────────────────────────────────────────────────────────────────

type ApprovalRequest struct {
	ApprovalID  int64      `json:"approval_id" db:"approval_id"`
	DocType     string     `json:"doc_type" db:"doc_type"`
	DocID       int64      `json:"doc_id" db:"doc_id"`
	DocNo       string     `json:"doc_no" db:"doc_no"`
	StepNo      int        `json:"step_no" db:"step_no"`
	StepName    string     `json:"step_name" db:"step_name"`
	RequestedBy int64      `json:"requested_by" db:"requested_by"`
	AssignedTo  *int64     `json:"assigned_to,omitempty" db:"assigned_to"`
	Status      string     `json:"status" db:"status"`
	DueDate     *time.Time `json:"due_date,omitempty" db:"due_date"`
	Amount      *float64   `json:"amount,omitempty" db:"amount"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

type ApprovalLog struct {
	LogID        int64     `json:"log_id" db:"log_id"`
	ApprovalID   int64     `json:"approval_id" db:"approval_id"`
	DocType      string    `json:"doc_type" db:"doc_type"`
	DocNo        string    `json:"doc_no" db:"doc_no"`
	StepNo       int       `json:"step_no" db:"step_no"`
	Action       string    `json:"action" db:"action"`
	ActionBy     int64     `json:"action_by" db:"action_by"`
	ActionByName string    `json:"action_by_name,omitempty" db:"action_by_name"`
	ActionAt     time.Time `json:"action_at" db:"action_at"`
	Comments     *string   `json:"comments,omitempty" db:"comments"`
	OldStatus    *string   `json:"old_status,omitempty" db:"old_status"`
	NewStatus    *string   `json:"new_status,omitempty" db:"new_status"`
}

type ApprovalActionRequest struct {
	Action   string  `json:"action" validate:"required,oneof=APPROVE REJECT RETURN COMMENT"`
	Comments *string `json:"comments,omitempty"`
}

// ─── PR Attachment ────────────────────────────────────────────────────────────

type PRAttachment struct {
	ID         int64     `json:"id"`
	PRID       int64     `json:"pr_id"`
	FileName   string    `json:"file_name"`
	FilePath   string    `json:"file_path"`
	FileSize   int64     `json:"file_size"`
	FileType   string    `json:"file_type"`
	UploadedBy *int64    `json:"uploaded_by,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type AddAttachmentRequest struct {
	FilePath string `json:"file_path" validate:"required"`
	FileName string `json:"file_name" validate:"required"`
	FileSize int64  `json:"file_size"`
	FileType string `json:"file_type"`
}

type UploadFileResponse struct {
	FilePath string `json:"file_path"`
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	FileType string `json:"file_type"`
}

// ─── PO Attachment ────────────────────────────────────────────────────────────

type POAttachment struct {
	ID         int64     `json:"id"`
	POID       int64     `json:"po_id"`
	FileName   string    `json:"file_name"`
	FilePath   string    `json:"file_path"`
	FileSize   int64     `json:"file_size"`
	FileType   string    `json:"file_type"`
	UploadedBy *int64    `json:"uploaded_by,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// ─── Common responses ─────────────────────────────────────────────────────────

type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// ─── Memo ──────────────────────────────────────────────────────────────────

type Memo struct {
	ID               int64     `json:"id"`
	MemoNo           string    `json:"memo_no"`
	Title            string    `json:"title"`
	ProjectCode      *string   `json:"project_code"`
	RequestedBy      int64     `json:"requested_by"`
	ApproverID       *int64    `json:"approver_id"`
	Department       *string   `json:"department"`
	DeliveryLocation *string   `json:"delivery_location,omitempty" db:"delivery_location"`
	Note             *string   `json:"note"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	CreatedBy        *int64    `json:"created_by"`
	UpdatedBy        *int64    `json:"updated_by"`

	// populated via JOIN (ไม่ได้เก็บใน table)
	RequestedByName string           `json:"requested_by_name,omitempty"`
	ApproverName    *string          `json:"approver_name,omitempty"`
	ProjectName     *string          `json:"project_name,omitempty"`
	Lines           []MemoLine       `json:"lines,omitempty"`
	Attachments     []MemoAttachment `json:"attachments,omitempty"`
}

type MemoAttachment struct {
	ID         int64     `json:"id"`
	MemoID     int64     `json:"memo_id"`
	FilePath   string    `json:"file_path"`
	FileName   string    `json:"file_name"`
	FileSize   int64     `json:"file_size"`
	FileType   *string   `json:"file_type"`
	UploadedBy *int64    `json:"uploaded_by"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type AttachmentRef struct {
	FilePath string `json:"file_path"`
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	FileType string `json:"file_type"`
}

type MemoLine struct {
	ID          int64   `json:"id"`
	MemoID      int64   `json:"memo_id"`
	LineNo      int     `json:"line_no"`
	Description string  `json:"description"`
	Unit        string  `json:"unit"`
	Quantity    float64 `json:"quantity"`
	Remark      *string `json:"remark"`
}

type CreateMemoRequest struct {
	Title            string            `json:"title"`
	ProjectCode      *string           `json:"project_code"`
	RequestedBy      int64             `json:"requested_by"`
	ApproverID       *int64            `json:"approver_id"`
	Department       *string           `json:"department"`
	DeliveryLocation *string           `json:"delivery_location,omitempty"`
	Note             *string           `json:"note"`
	Status           string            `json:"status,omitempty"` // "DRAFT" | "PENDING_APPROVAL" — defaults to DRAFT
	Lines            []MemoLineRequest `json:"lines"`
	Attachments      []AttachmentRef   `json:"attachments"`
}

type CancelMemoRequest struct {
	Comments string `json:"comments,omitempty"`
}

type MemoLineRequest struct {
	LineNo      int     `json:"line_no"`
	Description string  `json:"description"`
	Unit        string  `json:"unit"`
	Quantity    float64 `json:"quantity"`
	Remark      *string `json:"remark"`
}

type UpdateMemoRequest = CreateMemoRequest

type MemoListFilter struct {
	Search          string `query:"search"`
	ProjectCode     string `query:"project_code"`
	DateFrom        string `query:"date_from"`
	DateTo          string `query:"date_to"`
	Status          string `query:"status"`
	MyApprovals     string `query:"my_approvals"`
	ExcludeUsedByPR string `query:"exclude_used_by_pr"`
	AllUsers        string `query:"all_users"`
	Page            int    `query:"page"`
	PageSize        int    `query:"page_size"`
}

type StatusUpdateRequest struct {
	Status  string  `json:"status" validate:"required"`
	Remarks *string `json:"remarks,omitempty"`
}

// ── Work Order (หนังสือสั่งจ้าง) ───────────────────────────
// Approval goes through the shared generic approval engine (approval_doc_types +
// approval_config + approval_request, see internal/handlers/generic_approval.go) once
// doc_type='WO' config rows exist — WorkOrderHandler only owns Create/List/Get/Submit,
// not Approve/Reject.

type WorkOrder struct {
	ID               int64   `json:"id"`
	WoNo             string  `json:"wo_no"`
	WoDate           string  `json:"wo_date"`
	EmployerName     string  `json:"employer_name"`
	ProjectCode      *string `json:"project_code"`
	ProjectName      *string `json:"project_name,omitempty"`
	ProjectScopeText *string `json:"project_scope_text"`
	SupplierCode     *string `json:"supplier_code"`
	// SupplierID is an optional, decoupled link to supplier.id — unlike PO/GRN,
	// Work Order's supplier is free text (supplier_code/name/address/phone below
	// have no FK to the supplier table), so this stays nullable and non-authoritative.
	SupplierID              *int64    `json:"supplier_id,omitempty"`
	SupplierName            string    `json:"supplier_name"`
	ContactPerson           *string   `json:"contact_person"`
	SupplierAddress         *string   `json:"supplier_address"`
	SupplierPhone           *string   `json:"supplier_phone"`
	ContractType            string    `json:"contract_type"` // LABOR_ONLY | LABOR_MATERIAL
	WorkSystem              string    `json:"work_system"`   // P | E | S
	ContractDescription     *string   `json:"contract_description"`
	ContractAmount          float64   `json:"contract_amount"`
	VatRate                 float64   `json:"vat_rate"`
	WhtRate                 float64   `json:"wht_rate"`
	AdvancePct              float64   `json:"advance_pct"`
	AdvanceAmount           float64   `json:"advance_amount"`
	ProgressPaymentNote     *string   `json:"progress_payment_note"`
	RetentionPct            float64   `json:"retention_pct"`
	AdvanceDeductPct        float64   `json:"advance_deduct_pct"`
	OtherDeductionNote      *string   `json:"other_deduction_note"`
	StartDate               *string   `json:"start_date"`
	DurationDays            *int      `json:"duration_days"`
	EndDate                 *string   `json:"end_date"`
	PenaltyPctPerDay        float64   `json:"penalty_pct_per_day"`
	WarrantyYears           int       `json:"warranty_years"`
	RefNo                   *string   `json:"ref_no"`
	OtherTerms              *string   `json:"other_terms"`
	Status                  string    `json:"status"` // DRAFT|PENDING_APPROVAL|APPROVED|REJECTED|CANCELLED
	EnteredBy               *int64    `json:"entered_by"`
	EnteredByName           *string   `json:"entered_by_name,omitempty"`
	SectionHeadID           *int64    `json:"section_head_id"`
	SectionHeadName         *string   `json:"section_head_name,omitempty"`
	AuthorizedBy            *int64    `json:"authorized_by"`
	AuthorizedByName        *string   `json:"authorized_by_name,omitempty"`
	SubcontractorSignedName *string   `json:"subcontractor_signed_name"`
	Remarks                 *string   `json:"remarks"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`

	// Line-item totals — mirrors purchase_order's use_discount/discount_type/use_vat/use_wht
	// header flags plus aggregate total/discount/vat/wht/net amounts, computed server-side
	// from work_order_line the same way PO aggregates from purchase_order_line.
	UseDiscount    bool            `json:"use_discount"`
	DiscountType   string          `json:"discount_type"` // pct | amt
	UseVAT         bool            `json:"use_vat"`
	UseWHT         bool            `json:"use_wht"`
	TotalAmount    float64         `json:"total_amount"`
	DiscountAmount float64         `json:"discount_amount"`
	VatAmount      float64         `json:"vat_amount"`
	WhtAmount      float64         `json:"wht_amount"`
	NetAmount      float64         `json:"net_amount"`
	Lines          []WorkOrderLine `json:"lines,omitempty"`

	// Deprecated — superseded by Lines/work_order_line. Kept only so any code still
	// reading old drafts doesn't panic; no longer written to by Create/Update.
	CostCodes []WorkOrderCostCode `json:"cost_codes,omitempty"`
}

type CreateWorkOrderRequest struct {
	// Status is only read by Update (PUT /work-order/:id) — Create always hardcodes 'DRAFT'
	// in its INSERT. Update accepts "DRAFT" (no-op save) or "PENDING_APPROVAL" (the "ส่งอนุมัติ"
	// button submitting inline) — anything else, including jumping straight to APPROVED, is
	// rejected; approval decisions only ever happen through the dedicated Approve/Reject flow.
	Status              *string  `json:"status,omitempty"`
	WoDate              *string  `json:"wo_date"`
	EmployerName        string   `json:"employer_name" validate:"required"`
	ProjectCode         *string  `json:"project_code"`
	ProjectScopeText    *string  `json:"project_scope_text"`
	SupplierCode        *string  `json:"supplier_code"`
	SupplierID          *int64   `json:"supplier_id,omitempty"`
	SupplierName        string   `json:"supplier_name" validate:"required"`
	ContactPerson       *string  `json:"contact_person"`
	SupplierAddress     *string  `json:"supplier_address"`
	SupplierPhone       *string  `json:"supplier_phone"`
	ContractType        string   `json:"contract_type" validate:"required,oneof=LABOR_ONLY LABOR_MATERIAL"`
	WorkSystem          string   `json:"work_system" validate:"required,oneof=P E S"`
	ContractDescription *string  `json:"contract_description"`
	ContractAmount      float64  `json:"contract_amount" validate:"required,gt=0"`
	VatRate             *float64 `json:"vat_rate"`
	WhtRate             *float64 `json:"wht_rate"`
	AdvancePct          *float64 `json:"advance_pct"`
	AdvanceAmount       *float64 `json:"advance_amount"`
	ProgressPaymentNote *string  `json:"progress_payment_note"`
	RetentionPct        *float64 `json:"retention_pct"`
	AdvanceDeductPct    *float64 `json:"advance_deduct_pct"`
	OtherDeductionNote  *string  `json:"other_deduction_note"`
	StartDate           *string  `json:"start_date"`
	DurationDays        *int     `json:"duration_days"`
	EndDate             *string  `json:"end_date"`
	PenaltyPctPerDay    *float64 `json:"penalty_pct_per_day"`
	WarrantyYears       *int     `json:"warranty_years"`
	RefNo               *string  `json:"ref_no"`
	OtherTerms          *string  `json:"other_terms"`
	SectionHeadID       *int64   `json:"section_head_id"`
	AuthorizedBy        *int64   `json:"authorized_by"`
	Remarks             *string  `json:"remarks"`

	// Line items + header discount/VAT/WHT flags — mirrors PO's use_discount/discount_type/
	// use_vat/use_wht + purchase_order_line structure, with cost_code replacing mat_code.
	UseDiscount  *bool                `json:"use_discount"`
	DiscountType *string              `json:"discount_type" validate:"omitempty,oneof=pct amt"`
	UseVAT       *bool                `json:"use_vat"`
	UseWHT       *bool                `json:"use_wht"`
	Lines        []WorkOrderLineInput `json:"lines" validate:"required,min=1,dive"`
}

// WorkOrderLineInput is one line-item row of a Work Order (cost_code in place of PO's mat_code).
type WorkOrderLineInput struct {
	CostCode    string   `json:"cost_code" validate:"required"`
	Description *string  `json:"description"`
	Qty         float64  `json:"qty" validate:"required,gt=0"`
	UnitPrice   float64  `json:"unit_price" validate:"gte=0"`
	Disc        *float64 `json:"disc"`
	DiscType    *string  `json:"disc_type" validate:"omitempty,oneof=pct amt"`
	VatRate     *float64 `json:"vat_rate"`
	WhtRate     *float64 `json:"wht_rate" validate:"omitempty,oneof=1 3 5"`
}

// WorkOrderLine mirrors the work_order_line table (db row + response shape).
type WorkOrderLine struct {
	ID          int64     `json:"id"`
	WoID        int64     `json:"wo_id"`
	SortOrder   int       `json:"sort_order"`
	CostCode    string    `json:"cost_code"`
	Description *string   `json:"description"`
	Qty         float64   `json:"qty"`
	UnitPrice   float64   `json:"unit_price"`
	Amount      float64   `json:"amount"` // generated column: qty * unit_price
	Disc        float64   `json:"disc"`
	DiscType    string    `json:"disc_type"`
	VatRate     *float64  `json:"vat_rate"`
	WhtRate     *float64  `json:"wht_rate"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   *int64    `json:"created_by"`
}

// UpdateWorkOrderLinesRequest replaces the full set of lines (+ discount/VAT/WHT flags) for a WO.
type UpdateWorkOrderLinesRequest struct {
	UseDiscount  *bool                `json:"use_discount"`
	DiscountType *string              `json:"discount_type" validate:"omitempty,oneof=pct amt"`
	UseVAT       *bool                `json:"use_vat"`
	UseWHT       *bool                `json:"use_wht"`
	Lines        []WorkOrderLineInput `json:"lines" validate:"required,min=1,dive"`
}

// WorkOrderCostCode mirrors the deprecated work_order_cost_code table — superseded by
// WorkOrderLine, kept only for reading any pre-existing rows.
type WorkOrderCostCode struct {
	ID        int64     `json:"id"`
	WoID      int64     `json:"wo_id"`
	CostCode  string    `json:"cost_code"`
	Remarks   *string   `json:"remarks"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy *int64    `json:"created_by"`
}

type WorkOrderFilter struct {
	Status      string `query:"status"`
	ProjectCode string `query:"project_code"`
	Search      string `query:"search"`
	DateFrom    string `query:"date_from"`
	DateTo      string `query:"date_to"`
	Page        int    `query:"page"`
	PageSize    int    `query:"page_size"`
}

// ─── Finance / Payment Tracking ──────────────────────────────────────────────

type PaymentLog struct {
	Id         int64     `json:"id"`
	DocType    string    `json:"doc_type"`
	DocID      int64     `json:"doc_id"`
	DocNo      string    `json:"doc_no"`
	AmountPaid float64   `json:"amount_paid"`
	PaidDate   time.Time `json:"paid_date"`
	PaidBy     int64     `json:"paid_by"`
	PaidByName *string   `json:"paid_by_name,omitempty"`
	Remark     *string   `json:"remark,omitempty"`
	ReversesID *int64    `json:"reverses_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	CreatedBy  int64     `json:"created_by"`
}

type RecordPaymentRequest struct {
	DocType    string  `json:"doc_type"` // 'PO' | 'WO'
	DocID      int64   `json:"doc_id"`
	AmountPaid float64 `json:"amount_paid"`
	PaidDate   *string `json:"paid_date,omitempty"`
	PaidBy     int64   `json:"paid_by"`
	Remark     *string `json:"remark,omitempty"`
	ReversesID *int64  `json:"reverses_id,omitempty"`
}

// PayableDoc is the shared row shape for both PO and WO in ListPayableDocs —
// same fields regardless of doc_type, since both tables have net_amount/status/project_code.
type PayableDoc struct {
	Id             int64   `json:"id"`
	DocNo          string  `json:"doc_no"`
	DocType        string  `json:"doc_type"`
	ProjectCode    *string `json:"project_code,omitempty"`
	NetAmount      float64 `json:"net_amount"`
	Status         string  `json:"status"`
	PaidAmount     float64 `json:"paid_amount"`
	RemainingToPay float64 `json:"remaining_to_pay"`
}

type PayableDocFilter struct {
	DocType     string `query:"doc_type"` // required: PO | WO
	ProjectCode string `query:"project_code"`
	// Status is intentionally not filterable — ListPayableDocs always fixes it to
	// APPROVED, since only approved docs are payable.
	Search   string `query:"search"`
	Page     int    `query:"page"`
	PageSize int    `query:"page_size"`
}

// ── Petty Cash Requisition (ใบเบิกเงินสดย่อย) ───────────────────────────
// Not a stock-issue document — mat_code on each line is reference-only (for the material
// picker / display of project_stock.qty_on_hand). Nothing is deducted from stock_item,
// stock_inventory, or project_stock here. Approval follows the same per-document
// approver_id model as PO/MEMO (see isEligibleApprover's doc comment in approval_status.go)
// and reuses the generic approval engine (approval_doc_types already has a PETTY_CASH row).
//
// One document can span multiple projects — project_code lives on each line, not the header
// (petty_cash_requisition has no project_code column; petty_cash_requisition_line.project_code
// is NOT NULL). The header only ever shows an aggregate (ProjectCodes) built from its lines.

type PettyCashRequisition struct {
	ID             int64     `json:"id" db:"id"`
	PCNo           string    `json:"pc_no" db:"pc_no"`
	PCDate         time.Time `json:"pc_date" db:"pc_date"`
	RequestedBy    int64     `json:"requested_by" db:"requested_by"`
	Purpose        *string   `json:"purpose,omitempty" db:"purpose"`
	Currency       string    `json:"currency" db:"currency"`
	TotalAmount    float64   `json:"total_amount" db:"total_amount"`
	UseDiscount    bool      `json:"use_discount" db:"use_discount"`
	DiscountType   string    `json:"discount_type" db:"discount_type"`
	DiscountAmount float64   `json:"discount_amount" db:"discount_amount"`
	UseVat         bool      `json:"use_vat" db:"use_vat"`
	VatAmount      float64   `json:"vat_amount" db:"vat_amount"`
	UseWht         bool      `json:"use_wht" db:"use_wht"`
	WhtAmount      float64   `json:"wht_amount" db:"wht_amount"`
	NetAmount      float64   `json:"net_amount" db:"net_amount"`
	Status         string    `json:"status" db:"status"`
	ApproverID     *int64    `json:"approver_id,omitempty" db:"approver_id"`
	Remarks        *string   `json:"remarks,omitempty" db:"remarks"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
	CreatedBy      int64     `json:"created_by" db:"created_by"`
	UpdatedBy      *int64    `json:"updated_by,omitempty" db:"updated_by"`

	// populated via JOIN/aggregate (ไม่ได้เก็บใน table)
	ProjectCodes    []string        `json:"project_codes,omitempty"`
	RequestedByName *string         `json:"requested_by_name,omitempty"`
	ApproverName    *string         `json:"approver_name,omitempty"`
	Lines           []PettyCashLine `json:"lines,omitempty"`
}

type PettyCashLine struct {
	ID             int64    `json:"id" db:"id"`
	PCID           int64    `json:"pc_id" db:"pc_id"`
	LineNo         int      `json:"line_no" db:"line_no"`
	ProjectCode    string   `json:"project_code" db:"project_code"`
	MatCode        string   `json:"mat_code" db:"mat_code"`
	CostSubgroupID *int64   `json:"cost_subgroup_id,omitempty" db:"cost_subgroup_id"`
	Description    *string  `json:"description,omitempty" db:"description"`
	Qty            float64  `json:"qty" db:"qty"`
	UnitPrice      float64  `json:"unit_price" db:"unit_price"`
	Amount         float64  `json:"amount" db:"amount"`
	Discount       float64  `json:"discount" db:"discount"`
	DiscType       string   `json:"disc_type" db:"disc_type"`
	LineDiscount   float64  `json:"line_discount" db:"line_discount"`
	LineVat        float64  `json:"line_vat" db:"line_vat"`
	LineWht        float64  `json:"line_wht" db:"line_wht"`
	LineNet        float64  `json:"line_net" db:"line_net"`
	WhtRate        *float64 `json:"wht_rate,omitempty" db:"wht_rate"`
	Remarks        *string  `json:"remarks,omitempty" db:"remarks"`

	// display-only, joined at read time (never persisted on this table) — all resolved
	// per-LINE from this line's own project_code, not the header.
	ProjectName *string `json:"project_name,omitempty"`
	ItemName    *string `json:"item_name,omitempty"`
	Unit        *string `json:"unit,omitempty"`
	StockOnHand float64 `json:"stock_on_hand"`
}

type CreatePettyCashRequest struct {
	Purpose      string                    `json:"purpose"`
	Currency     string                    `json:"currency"`
	UseDiscount  bool                      `json:"use_discount"`
	DiscountType string                    `json:"discount_type"`
	DiscountAmt  float64                   `json:"discount_amount"`
	UseVat       bool                      `json:"use_vat"`
	UseWht       bool                      `json:"use_wht"`
	ApproverID   *int64                    `json:"approver_id"`
	Status       string                    `json:"status,omitempty"` // "DRAFT" | "PENDING_APPROVAL" — defaults to DRAFT
	Remarks      string                    `json:"remarks"`
	Lines        []CreatePettyCashLineItem `json:"lines" validate:"required,min=1"`
}

type CreatePettyCashLineItem struct {
	ProjectCode    string   `json:"project_code" validate:"required"`
	MatCode        string   `json:"mat_code" validate:"required"`
	CostSubgroupID *int64   `json:"cost_subgroup_id"`
	Description    string   `json:"description"`
	Qty            float64  `json:"qty" validate:"required,gt=0"`
	UnitPrice      float64  `json:"unit_price" validate:"required,gte=0"`
	Discount       float64  `json:"discount"`
	DiscType       string   `json:"disc_type"`
	WhtRate        *float64 `json:"wht_rate"`
	Remarks        *string  `json:"remarks"`
}

type UpdatePettyCashRequest = CreatePettyCashRequest

type PettyCashListFilter struct {
	Status      string `query:"status"`
	ProjectCode string `query:"project_code"` // matches any line's project_code
	DateFrom    string `query:"date_from"`
	DateTo      string `query:"date_to"`
	Page        int    `query:"page"`
	PageSize    int    `query:"page_size"`
}
