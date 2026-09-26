package handlers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
	"erp-api/internal/models"
)

var nonDigitRe = regexp.MustCompile(`[^0-9]`)

// parsePaymentTermsDays strips all non-digit characters from a payment_terms string
// (e.g. "30  วัน") and parses the remainder as an integer day count. Empty/unparseable
// input returns nil.
func parsePaymentTermsDays(paymentTerms *string) *int {
	if paymentTerms == nil {
		return nil
	}
	digits := nonDigitRe.ReplaceAllString(*paymentTerms, "")
	if digits == "" {
		return nil
	}
	days, err := strconv.Atoi(digits)
	if err != nil {
		return nil
	}
	return &days
}

// ICHandler serves the Inventory Control (IC) module.
type ICHandler struct {
	db *pgxpool.Pool
}

func NewICHandler(db *pgxpool.Pool) *ICHandler {
	return &ICHandler{db: db}
}

type ICProject struct {
	ID           int64   `json:"id"`
	ProjectCode  string  `json:"project_code"`
	ProjectName  string  `json:"project_name"`
	CustomerName *string `json:"customer_name"`
}

// GetProject godoc
// @Summary      Get a single project's basic info (IC context header)
// @Description  Returns id, project_code, project_name, customer_name for a project by id.
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        projectId  path  int  true  "project.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectId} [get]
func (h *ICHandler) GetProject(c *fiber.Ctx) error {
	projectID, err := strconv.ParseInt(c.Params("projectId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid projectId")
	}

	var p ICProject
	err = h.db.QueryRow(context.Background(), `
		SELECT p.id, p.project_code, p.project_name, c.customer_name
		FROM project p
		LEFT JOIN customer c ON p.customer_id::integer = c.cus_id
		WHERE p.id = $1`, projectID,
	).Scan(&p.ID, &p.ProjectCode, &p.ProjectName, &p.CustomerName)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}

	return c.JSON(fiber.Map{"success": true, "data": p})
}

// ListProjects godoc
// @Summary      List projects for Inventory Control
// @Description  Returns projects joined with their customer, paginated and optionally filtered by search keyword.
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        search           query  string  false  "search by project_code or project_name"
// @Param        include_inactive query  bool    false  "include inactive projects (default false)"
// @Param        page             query  int     false  "page number"  default(1)
// @Param        page_size        query  int     false  "page size"    default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects [get]
func (h *ICHandler) ListProjects(c *fiber.Ctx) error {
	search := strings.TrimSpace(c.Query("search"))
	includeInactive := c.QueryBool("include_inactive", false)
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	where := []string{"1=1"}
	args := []interface{}{}
	idx := 1

	if !includeInactive {
		where = append(where, "p.is_active = true")
	}
	if search != "" {
		where = append(where, fmt.Sprintf("(p.project_code ILIKE $%d OR p.project_name ILIKE $%d)", idx, idx))
		args = append(args, "%"+search+"%")
		idx++
	}

	whereStr := strings.Join(where, " AND ")
	fromJoin := `FROM project p LEFT JOIN customer c ON p.customer_id = c.cus_id`

	var total int64
	if err := h.db.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT COUNT(*) %s WHERE %s`, fromJoin, whereStr), args...,
	).Scan(&total); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to count projects: "+err.Error())
	}

	args = append(args, size, offset)
	rows, err := h.db.Query(context.Background(),
		fmt.Sprintf(`SELECT p.id, p.project_code, p.project_name, c.customer_name
			%s WHERE %s
			ORDER BY p.project_code
			LIMIT $%d OFFSET $%d`, fromJoin, whereStr, idx, idx+1), args...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list projects: "+err.Error())
	}
	defer rows.Close()

	items := []ICProject{}
	for rows.Next() {
		var p ICProject
		if err := rows.Scan(&p.ID, &p.ProjectCode, &p.ProjectName, &p.CustomerName); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to scan project: "+err.Error())
		}
		items = append(items, p)
	}

	totalPages := int(total) / size
	if int(total)%size != 0 || total == 0 {
		totalPages++
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages,
		},
	})
}

type ICProjectPO struct {
	POID          int64   `json:"po_id"`
	ProjectCode   string  `json:"project_code"`
	ProjectName   *string `json:"project_name"`
	PONo          string  `json:"po_no"`
	PODate        *string `json:"po_date"`
	PRNo          *string `json:"pr_no"`
	SupplierName  *string `json:"supplier_name"`
	ExpectedDate  *string `json:"expected_date"`
	StatusReceive string  `json:"status_receive"`
	DueDate       *string `json:"due_date"`
	CreditDays    *int    `json:"credit_days"`
	ScoreQuality  *int    `json:"score_quality"`
	ScoreQuantity *int    `json:"score_quantity"`
	ScoreOntime   *int    `json:"score_ontime"`
	ScoreNotes    *string `json:"score_notes"`
	RatedAt       *string `json:"rated_at"`
	RatedBy       *int64  `json:"rated_by"`
}

// resolveProjectCode looks up project.project_code (and project_name) by project.id, 404ing if not found.
func (h *ICHandler) resolveProjectCode(ctx context.Context, projectID int64) (string, string, error) {
	var projectCode, projectName string
	err := h.db.QueryRow(ctx, `SELECT project_code, project_name FROM project WHERE id = $1`, projectID).
		Scan(&projectCode, &projectName)
	if err != nil {
		return "", "", fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	return projectCode, projectName, nil
}

// ListProjectPOs godoc
// @Summary      List Purchase Orders under a project (IC PO Receive list)
// @Description  Resolves :projectId to project_code, then lists purchase_order rows for that project. Only APPROVED POs (purchase_order.status = 'APPROVED') are eligible — this is independent of status_receive. Includes due_date/credit_days from ic_po_receive_document (null until Tab 1 is submitted for that PO). A PO can now have many receive documents — due_date/credit_days/score fields shown here are from one representative document (the newest finalized one, i.e. receive_no NOT NULL, falling back to the newest draft); use GET /ic/pos/{poId}/receive-documents for the full list.
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        projectId       path   int     true   "project.id"
// @Param        search          query  string  false  "search by po_no or pr_no"
// @Param        receive_status  query  string  false  "pending (default) or completed"
// @Param        page            query  int     false  "page number"  default(1)
// @Param        page_size       query  int     false  "page size"    default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectId}/pos [get]
func (h *ICHandler) ListProjectPOs(c *fiber.Ctx) error {
	ctx := context.Background()

	projectID, err := strconv.ParseInt(c.Params("projectId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid projectId")
	}

	projectCode, projectName, err := h.resolveProjectCode(ctx, projectID)
	if err != nil {
		return err
	}

	search := strings.TrimSpace(c.Query("search"))
	receiveStatus := c.Query("receive_status", "pending")
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	where := []string{"po.project_code = $1", "po.status = 'APPROVED'"}
	args := []interface{}{projectCode}
	idx := 2

	switch receiveStatus {
	case "completed":
		where = append(where, "po.status_receive = 'RECEIVED'")
	default:
		where = append(where, "po.status_receive IN ('NOT_SENT','SENT','PARTIALLY_RECEIVED')")
	}

	if search != "" {
		where = append(where, fmt.Sprintf("(po.po_no ILIKE $%d OR pr.pr_no ILIKE $%d)", idx, idx))
		args = append(args, "%"+search+"%")
		idx++
	}

	whereStr := strings.Join(where, " AND ")
	// ic_po_receive_document.po_id is no longer unique (a PO can now have many receive
	// documents) — a plain LEFT JOIN would duplicate PO rows in this list. LATERAL picks one
	// representative document per PO instead: prefer the most recently *finalized* one
	// (receive_no NOT NULL), falling back to the newest draft if none is finalized yet.
	fromJoin := `FROM purchase_order po
		LEFT JOIN purchase_request pr ON po.pr_id = pr.id
		LEFT JOIN supplier s ON po.supplier_id = s.id
		LEFT JOIN LATERAL (
			SELECT due_date, credit_days, score_quality, score_quantity, score_ontime, score_notes, rated_at, rated_by
			FROM ic_po_receive_document
			WHERE po_id = po.id
			ORDER BY (receive_no IS NULL) ASC, created_at DESC
			LIMIT 1
		) rd ON true`

	var total int64
	if err := h.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) %s WHERE %s`, fromJoin, whereStr), args...,
	).Scan(&total); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to count POs: "+err.Error())
	}

	args = append(args, size, offset)
	rows, err := h.db.Query(ctx,
		fmt.Sprintf(`SELECT po.id, po.po_no, po.po_date::text, pr.pr_no, s.supplier_name,
			po.expected_date::text, po.status_receive, rd.due_date::text, rd.credit_days,
			rd.score_quality, rd.score_quantity, rd.score_ontime, rd.score_notes, rd.rated_at::text, rd.rated_by
			%s WHERE %s
			ORDER BY po.po_date DESC, po.po_no DESC
			LIMIT $%d OFFSET $%d`, fromJoin, whereStr, idx, idx+1), args...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list POs: "+err.Error())
	}
	defer rows.Close()

	items := []ICProjectPO{}
	for rows.Next() {
		var p ICProjectPO
		if err := rows.Scan(&p.POID, &p.PONo, &p.PODate, &p.PRNo, &p.SupplierName,
			&p.ExpectedDate, &p.StatusReceive, &p.DueDate, &p.CreditDays,
			&p.ScoreQuality, &p.ScoreQuantity, &p.ScoreOntime, &p.ScoreNotes, &p.RatedAt, &p.RatedBy); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to scan PO: "+err.Error())
		}
		p.ProjectCode = projectCode
		p.ProjectName = &projectName
		items = append(items, p)
	}

	totalPages := int(total) / size
	if int(total)%size != 0 || total == 0 {
		totalPages++
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages,
		},
	})
}

type ICSearchOption struct {
	ID   int64  `json:"id"`
	No   string `json:"no"`
	Type string `json:"type"`
}

// POSearchOptions godoc
// @Summary      Lightweight PR/PO number list for the IC PO-search dropdown
// @Description  Returns PR numbers and PO numbers for one project, tagged with type ('PR' or 'PO') for a grouped dropdown.
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        projectId  path  int  true  "project.id"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectId}/po-search-options [get]
func (h *ICHandler) POSearchOptions(c *fiber.Ctx) error {
	ctx := context.Background()

	projectID, err := strconv.ParseInt(c.Params("projectId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid projectId")
	}

	projectCode, _, err := h.resolveProjectCode(ctx, projectID)
	if err != nil {
		return err
	}

	options := []ICSearchOption{}

	prRows, err := h.db.Query(ctx,
		`SELECT id, pr_no FROM purchase_request WHERE project_code = $1 ORDER BY pr_no`, projectCode)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list PRs: "+err.Error())
	}
	for prRows.Next() {
		var o ICSearchOption
		if err := prRows.Scan(&o.ID, &o.No); err != nil {
			prRows.Close()
			return fiber.NewError(fiber.StatusInternalServerError, "failed to scan PR: "+err.Error())
		}
		o.Type = "PR"
		options = append(options, o)
	}
	prRows.Close()

	poRows, err := h.db.Query(ctx,
		`SELECT id, po_no FROM purchase_order WHERE project_code = $1 ORDER BY po_no`, projectCode)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list POs: "+err.Error())
	}
	for poRows.Next() {
		var o ICSearchOption
		if err := poRows.Scan(&o.ID, &o.No); err != nil {
			poRows.Close()
			return fiber.NewError(fiber.StatusInternalServerError, "failed to scan PO: "+err.Error())
		}
		o.Type = "PO"
		options = append(options, o)
	}
	poRows.Close()

	return c.JSON(fiber.Map{"success": true, "data": options})
}

type ICReceiveDocumentContext struct {
	PONo                  string  `json:"po_no"`
	SupplierName          *string `json:"supplier_name"`
	ProjectName           *string `json:"project_name"`
	JobCode               *string `json:"job_code"`
	Currency              string  `json:"currency"`
	UseVAT                bool    `json:"use_vat"`
	VATAmount             float64 `json:"vat_amount"`
	CreditDaysFromSupplier *int   `json:"credit_days_from_supplier"`
}

type ICPOReceiveDocument struct {
	ID                int64      `json:"id"`
	POID              int64      `json:"po_id"`
	TaxInvoiceNo      *string    `json:"tax_invoice_no"`
	TaxInvoiceDate    *string    `json:"tax_invoice_date"`
	TempDeliveryNo    *string    `json:"temp_delivery_no"`
	TempDeliveryDate  *string    `json:"temp_delivery_date"`
	ReceiveNo         *string    `json:"receive_no"`
	CreditDays        *int       `json:"credit_days"`
	DueDate           *string    `json:"due_date"`
	ExchangeRate      *float64   `json:"exchange_rate"`
	Remarks           *string    `json:"remarks"`
	ScoreQuality      *int       `json:"score_quality"`
	ScoreQuantity     *int       `json:"score_quantity"`
	ScoreOntime       *int       `json:"score_ontime"`
	ScoreNotes        *string    `json:"score_notes"`
	RatedAt           *string    `json:"rated_at"`
	RatedBy           *int64     `json:"rated_by"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
	CreatedBy         *int64     `json:"created_by"`
	UpdatedBy         *int64     `json:"updated_by"`
}

// GetReceiveDocument godoc
// @Summary      Get PO receive document context + the current draft document (if any)
// @Description  Returns read-only PO/supplier/project context plus this PO's current DRAFT receive document (receive_no IS NULL, newest such row), or null if there is no draft — i.e. every existing document for this PO already has a receive_no, or none exist yet. A PO can have many receive_document rows now (one per delivery/invoice round); use GET /ic/pos/{poId}/receive-documents to list all of them, or GET /ic/pos/{poId}/receive-documents/{docId} for one specific (already-finalized) document.
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        poId  path  int  true  "purchase_order.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/pos/{poId}/receive-document [get]
func (h *ICHandler) GetReceiveDocument(c *fiber.Ctx) error {
	ctx := context.Background()

	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}

	var ctxRow ICReceiveDocumentContext
	var paymentTerms *string
	var poStatus string
	err = h.db.QueryRow(ctx, `
		SELECT po.po_no, s.supplier_name, pj.project_name, po.job_code, po.currency, po.use_vat, po.vat_amount, s.payment_terms, po.status
		FROM purchase_order po
		LEFT JOIN supplier s ON po.supplier_id = s.id
		LEFT JOIN project pj ON po.project_code = pj.project_code
		WHERE po.id = $1`, poID,
	).Scan(&ctxRow.PONo, &ctxRow.SupplierName, &ctxRow.ProjectName, &ctxRow.JobCode,
		&ctxRow.Currency, &ctxRow.UseVAT, &ctxRow.VATAmount, &paymentTerms, &poStatus)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if poStatus != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not approved")
	}
	ctxRow.CreditDaysFromSupplier = parsePaymentTermsDays(paymentTerms)

	var doc *ICPOReceiveDocument
	var d ICPOReceiveDocument
	err = h.db.QueryRow(ctx, `
		SELECT id, po_id, tax_invoice_no, tax_invoice_date::text, temp_delivery_no, temp_delivery_date::text,
		       receive_no, credit_days, due_date::text, exchange_rate, remarks,
		       score_quality, score_quantity, score_ontime, score_notes, rated_at::text, rated_by,
		       created_at::text, updated_at::text, created_by, updated_by
		FROM ic_po_receive_document
		WHERE po_id = $1 AND receive_no IS NULL
		ORDER BY created_at DESC LIMIT 1`, poID,
	).Scan(&d.ID, &d.POID, &d.TaxInvoiceNo, &d.TaxInvoiceDate, &d.TempDeliveryNo, &d.TempDeliveryDate,
		&d.ReceiveNo, &d.CreditDays, &d.DueDate, &d.ExchangeRate, &d.Remarks,
		&d.ScoreQuality, &d.ScoreQuantity, &d.ScoreOntime, &d.ScoreNotes, &d.RatedAt, &d.RatedBy,
		&d.CreatedAt, &d.UpdatedAt, &d.CreatedBy, &d.UpdatedBy)
	if err == nil {
		doc = &d
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"context":  ctxRow,
			"document": doc,
		},
	})
}

type SubmitReceiveDocumentRequest struct {
	TaxInvoiceNo     string   `json:"tax_invoice_no"`
	TaxInvoiceDate   string   `json:"tax_invoice_date"`
	TempDeliveryNo   *string  `json:"temp_delivery_no"`
	TempDeliveryDate *string  `json:"temp_delivery_date"`
	ExchangeRate     *float64 `json:"exchange_rate"`
	Remarks          *string  `json:"remarks"`
}

// SubmitReceiveDocument godoc
// @Summary      Submit the PO receive document
// @Description  Creates a NEW ic_po_receive_document row for this PO — a PO can have many receive documents now (one per delivery/invoice round). 409s "this PO has nothing left to receive" when SUM(qty_ordered - qty_received) over the PO's lines is <= 0. 409s "an empty receive document already exists" if this PO already has an empty draft document (receive_no IS NULL) so empty drafts can't stack; that draft must be submitted (via receive-lines/submit) or deleted first. Computes due_date = (business date this row is created, Asia/Bangkok) + credit_days (credit_days parsed from the PO's supplier payment_terms). due_date is NULL if credit_days can't be parsed. Computed once at creation and never recomputed on later partial-receive rounds.
// @Tags         IC
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        poId  path  int  true  "purchase_order.id"
// @Param        body  body  ICSubmitReceiveDocumentSwagger  true  "Receive document"
// @Success      201  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/pos/{poId}/receive-document [post]
func (h *ICHandler) SubmitReceiveDocument(c *fiber.Ctx) error {
	ctx := context.Background()

	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}

	var req SubmitReceiveDocumentRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	req.TaxInvoiceNo = strings.TrimSpace(req.TaxInvoiceNo)
	req.TaxInvoiceDate = strings.TrimSpace(req.TaxInvoiceDate)
	// tax_invoice_no/date are optional (a document may carry only temp_delivery_no/date, or
	// neither yet) — empty values are stored as NULL, not ''. A non-empty date must still parse.
	var taxInvoiceNo *string
	if req.TaxInvoiceNo != "" {
		taxInvoiceNo = &req.TaxInvoiceNo
	}
	var taxInvoiceDate *time.Time
	if req.TaxInvoiceDate != "" {
		t, err := time.Parse("2006-01-02", req.TaxInvoiceDate)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "tax_invoice_date must be in YYYY-MM-DD format")
		}
		taxInvoiceDate = &t
	}

	var supplierID *int64
	var poStatus string
	if err := h.db.QueryRow(ctx, `SELECT supplier_id, status FROM purchase_order WHERE id = $1`, poID).Scan(&supplierID, &poStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if poStatus != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not approved")
	}

	var remainingQty float64
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(qty_ordered - COALESCE(qty_received, 0)), 0)
		FROM purchase_order_line WHERE po_id = $1`, poID,
	).Scan(&remainingQty); err != nil {
		return err
	}
	if remainingQty <= 0 {
		return fiber.NewError(fiber.StatusConflict, "this PO has nothing left to receive")
	}

	var paymentTerms *string
	if supplierID != nil {
		h.db.QueryRow(ctx, `SELECT payment_terms FROM supplier WHERE id = $1`, *supplierID).Scan(&paymentTerms)
	}
	creditDays := parsePaymentTermsDays(paymentTerms)

	// due_date = the business date this row is created on (Asia/Bangkok), not tax_invoice_date —
	// computed once here and never recomputed on later partial-receive rounds (nothing else
	// writes ic_po_receive_document.due_date).
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		loc = time.FixedZone("Asia/Bangkok", 7*60*60)
	}
	now := time.Now().In(loc)
	creationDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	var dueDate *time.Time
	if creditDays != nil {
		dd := creationDate.AddDate(0, 0, *creditDays)
		dueDate = &dd
	}

	// Many documents per PO are now allowed, but empty (never-submitted) drafts must not stack —
	// only reject if an existing document for this PO still has no receive_no.
	var existingDraftID int64
	if err := h.db.QueryRow(ctx, `
		SELECT id FROM ic_po_receive_document WHERE po_id = $1 AND receive_no IS NULL`, poID,
	).Scan(&existingDraftID); err == nil {
		return fiber.NewError(fiber.StatusConflict, "an empty receive document already exists")
	}

	claims := middleware.GetClaims(c)
	var createdBy *int64
	if claims != nil {
		uid := claims.UserID
		createdBy = &uid
	}

	var d ICPOReceiveDocument
	err = h.db.QueryRow(ctx, `
		INSERT INTO ic_po_receive_document
		    (po_id, tax_invoice_no, tax_invoice_date, temp_delivery_no, temp_delivery_date,
		     credit_days, due_date, exchange_rate, remarks, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
		RETURNING id, po_id, tax_invoice_no, tax_invoice_date::text, temp_delivery_no, temp_delivery_date::text,
		          credit_days, due_date::text, exchange_rate, remarks,
		          created_at::text, updated_at::text, created_by, updated_by`,
		poID, taxInvoiceNo, taxInvoiceDate, req.TempDeliveryNo, req.TempDeliveryDate,
		creditDays, dueDate, req.ExchangeRate, req.Remarks, createdBy,
	).Scan(&d.ID, &d.POID, &d.TaxInvoiceNo, &d.TaxInvoiceDate, &d.TempDeliveryNo, &d.TempDeliveryDate,
		&d.CreditDays, &d.DueDate, &d.ExchangeRate, &d.Remarks,
		&d.CreatedAt, &d.UpdatedAt, &d.CreatedBy, &d.UpdatedBy)
	if err != nil {
		// po_id is no longer unique on this table (many documents per PO), so a duplicate-key
		// error here can only come from the partial unique index on receive_no — which this
		// INSERT never sets (always NULL at creation) — so this is a genuine unexpected failure.
		return fiber.NewError(fiber.StatusInternalServerError, "failed to submit receive document: "+err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": d})
}

type ICReceiveDocumentSummary struct {
	ID               int64   `json:"id"`
	ReceiveNo        *string `json:"receive_no"`
	TaxInvoiceNo     *string `json:"tax_invoice_no"`
	TaxInvoiceDate   *string `json:"tax_invoice_date"`
	TempDeliveryNo   *string `json:"temp_delivery_no"`
	TempDeliveryDate *string `json:"temp_delivery_date"`
	CreditDays       *int    `json:"credit_days"`
	DueDate          *string `json:"due_date"`
	CreatedAt        string  `json:"created_at"`
	LineCount        int64   `json:"line_count"`
	Deletable        bool    `json:"deletable"`
}

// ListReceiveDocuments godoc
// @Summary      List all receive documents for a PO (one PO can now have many), plus create-eligibility
// @Description  data.documents: newest first (created_at DESC). line_count = COUNT of ic_project_cost_item_transaction rows PLUS stock_transaction rows linked to that document via receive_document_id — both tables now carry that FK, so this is accurate for order_type='cost' AND order_type='stock' POs (stock_transaction rows written before this FK existed, or for a PO that historically had more than one document, may still be unlinked — those don't count here). deletable = (receive_no IS NULL AND line_count = 0). data.remaining_qty_total = SUM(qty_ordered - qty_received) over the PO's lines. data.can_create_new = purchase_order.status = 'APPROVED' AND remaining_qty_total > 0 AND no document for this PO has receive_no IS NULL — mirrors exactly the checks POST /ic/pos/{poId}/receive-document enforces.
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        poId  path  int  true  "purchase_order.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/pos/{poId}/receive-documents [get]
func (h *ICHandler) ListReceiveDocuments(c *fiber.Ctx) error {
	ctx := context.Background()

	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}

	var poStatus string
	if err := h.db.QueryRow(ctx, `SELECT status FROM purchase_order WHERE id = $1`, poID).Scan(&poStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}

	var remainingQtyTotal float64
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(qty_ordered - COALESCE(qty_received, 0)), 0)
		FROM purchase_order_line WHERE po_id = $1`, poID,
	).Scan(&remainingQtyTotal); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute remaining qty: "+err.Error())
	}

	var hasOpenDraft bool
	if err := h.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM ic_po_receive_document WHERE po_id = $1 AND receive_no IS NULL)`, poID,
	).Scan(&hasOpenDraft); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to check open drafts: "+err.Error())
	}
	canCreateNew := poStatus == "APPROVED" && remainingQtyTotal > 0 && !hasOpenDraft

	rows, err := h.db.Query(ctx, `
		SELECT rd.id, rd.receive_no, rd.tax_invoice_no, rd.tax_invoice_date::text,
		       rd.temp_delivery_no, rd.temp_delivery_date::text, rd.credit_days, rd.due_date::text,
		       rd.created_at::text,
		       COALESCE(t.line_count, 0)
		FROM ic_po_receive_document rd
		LEFT JOIN (
			SELECT receive_document_id, COUNT(*) AS line_count FROM (
				SELECT receive_document_id FROM ic_project_cost_item_transaction WHERE receive_document_id IS NOT NULL
				UNION ALL
				SELECT receive_document_id FROM ic_project_receipt_pool_transaction WHERE receive_document_id IS NOT NULL
				UNION ALL
				SELECT receive_document_id FROM stock_transaction WHERE receive_document_id IS NOT NULL
			) combined
			GROUP BY receive_document_id
		) t ON t.receive_document_id = rd.id
		WHERE rd.po_id = $1
		ORDER BY rd.created_at DESC`, poID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list receive documents: "+err.Error())
	}
	defer rows.Close()

	items := []ICReceiveDocumentSummary{}
	for rows.Next() {
		var s ICReceiveDocumentSummary
		if err := rows.Scan(&s.ID, &s.ReceiveNo, &s.TaxInvoiceNo, &s.TaxInvoiceDate,
			&s.TempDeliveryNo, &s.TempDeliveryDate, &s.CreditDays, &s.DueDate,
			&s.CreatedAt, &s.LineCount); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to scan receive document: "+err.Error())
		}
		s.Deletable = s.ReceiveNo == nil && s.LineCount == 0
		items = append(items, s)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"documents":           items,
			"can_create_new":      canCreateNew,
			"remaining_qty_total": remainingQtyTotal,
		},
	})
}

// GetReceiveDocumentByID godoc
// @Summary      Get one specific receive document of a PO by id
// @Description  For viewing/printing a specific (possibly already-finalized) document, as opposed to GET /ic/pos/{poId}/receive-document which only ever returns the current empty draft.
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        poId   path  int  true  "purchase_order.id"
// @Param        docId  path  int  true  "ic_po_receive_document.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/pos/{poId}/receive-documents/{docId} [get]
func (h *ICHandler) GetReceiveDocumentByID(c *fiber.Ctx) error {
	ctx := context.Background()

	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}
	docID, err := strconv.ParseInt(c.Params("docId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid docId")
	}

	var d ICPOReceiveDocument
	err = h.db.QueryRow(ctx, `
		SELECT id, po_id, tax_invoice_no, tax_invoice_date::text, temp_delivery_no, temp_delivery_date::text,
		       receive_no, credit_days, due_date::text, exchange_rate, remarks,
		       score_quality, score_quantity, score_ontime, score_notes, rated_at::text, rated_by,
		       created_at::text, updated_at::text, created_by, updated_by
		FROM ic_po_receive_document WHERE id = $1 AND po_id = $2`, docID, poID,
	).Scan(&d.ID, &d.POID, &d.TaxInvoiceNo, &d.TaxInvoiceDate, &d.TempDeliveryNo, &d.TempDeliveryDate,
		&d.ReceiveNo, &d.CreditDays, &d.DueDate, &d.ExchangeRate, &d.Remarks,
		&d.ScoreQuality, &d.ScoreQuantity, &d.ScoreOntime, &d.ScoreNotes, &d.RatedAt, &d.RatedBy,
		&d.CreatedAt, &d.UpdatedAt, &d.CreatedBy, &d.UpdatedBy)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "receive document not found for this PO")
	}

	return c.JSON(fiber.Map{"success": true, "data": d})
}

// DeleteReceiveDocument godoc
// @Summary      Delete an empty (never-submitted) receive document
// @Description  Allowed only when receive_no IS NULL and it has no linked ic_project_cost_item_transaction or stock_transaction rows (by receive_document_id) — otherwise 409. Locks the document row FOR UPDATE and re-checks inside the transaction before deleting.
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        poId   path  int  true  "purchase_order.id"
// @Param        docId  path  int  true  "ic_po_receive_document.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/pos/{poId}/receive-documents/{docId} [delete]
func (h *ICHandler) DeleteReceiveDocument(c *fiber.Ctx) error {
	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}
	docID, err := strconv.ParseInt(c.Params("docId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid docId")
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var docPOID int64
	var receiveNo *string
	if err := tx.QueryRow(ctx, `
		SELECT po_id, receive_no FROM ic_po_receive_document WHERE id = $1 FOR UPDATE`, docID,
	).Scan(&docPOID, &receiveNo); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "receive document not found")
	}
	if docPOID != poID {
		return fiber.NewError(fiber.StatusNotFound, "receive document not found for this PO")
	}
	if receiveNo != nil {
		return fiber.NewError(fiber.StatusConflict, "this receive document has already been submitted (receive_no issued) and cannot be deleted")
	}

	var linkedCount int64
	if err := tx.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM ic_project_cost_item_transaction WHERE receive_document_id = $1) +
			(SELECT COUNT(*) FROM ic_project_receipt_pool_transaction WHERE receive_document_id = $1) +
			(SELECT COUNT(*) FROM stock_transaction WHERE receive_document_id = $1)`, docID,
	).Scan(&linkedCount); err != nil {
		return err
	}
	if linkedCount > 0 {
		return fiber.NewError(fiber.StatusConflict, "this receive document has linked transactions and cannot be deleted")
	}

	if _, err := tx.Exec(ctx, `DELETE FROM ic_po_receive_document WHERE id = $1`, docID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to delete receive document: "+err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"deleted_id": docID}})
}

type icRateReceiveDocumentRequest struct {
	ScoreQuality  int     `json:"score_quality" validate:"required,min=1,max=5"`
	ScoreQuantity int     `json:"score_quantity" validate:"required,min=1,max=5"`
	ScoreOntime   int     `json:"score_ontime" validate:"required,min=1,max=5"`
	ScoreNotes    *string `json:"score_notes,omitempty"`
}

// RateReceiveDocument godoc
// @Summary      Rate the supplier on one specific receive document (quality/quantity/ontime, 1-5)
// @Description  Same validation and 1-5 scoring rules as the legacy GRN Score endpoint (POST /grn/{id}/score): all three scores required, each 1-5, score_notes optional. Requires the specific docId to exist for this PO (404) and to have a receive_no already issued (400 — line items must be submitted via receive-lines/submit before rating, same as Tab 1 must precede Tab 2). Unlike GRN (which blocks re-scoring once POSTED), this endpoint rejects a second rating attempt on the same document with 409 rather than allowing overwrite. Sets rated_at=NOW() and rated_by=claims.UserID in one transaction. A PO can now have many receive documents — this endpoint rates exactly the document named by docId, not "the PO" as a whole (superseded the old PO-scoped PUT /ic/pos/{poId}/receive-document/rating, which has been removed — nothing in this repo called it).
// @Tags         IC
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        poId   path  int                            true  "purchase_order.id"
// @Param        docId  path  int                            true  "ic_po_receive_document.id"
// @Param        body   body  icRateReceiveDocumentRequest  true  "Supplier rating"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/pos/{poId}/receive-documents/{docId}/rating [put]
func (h *ICHandler) RateReceiveDocument(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "no claims found")
	}

	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}
	docID, err := strconv.ParseInt(c.Params("docId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid docId")
	}

	var req icRateReceiveDocumentRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.ScoreQuality < 1 || req.ScoreQuality > 5 ||
		req.ScoreQuantity < 1 || req.ScoreQuantity > 5 ||
		req.ScoreOntime < 1 || req.ScoreOntime > 5 {
		return fiber.NewError(fiber.StatusBadRequest, "score_quality, score_quantity, score_ontime must each be 1-5")
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var docPOID int64
	var receiveNo, existingRatedAt *string
	if err := tx.QueryRow(ctx, `
		SELECT po_id, receive_no, rated_at::text FROM ic_po_receive_document
		WHERE id = $1 FOR UPDATE`,
		docID,
	).Scan(&docPOID, &receiveNo, &existingRatedAt); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "receive document not found")
	}
	if docPOID != poID {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("receive_document_id %d does not belong to po_id %d", docID, poID))
	}
	if receiveNo == nil {
		return fiber.NewError(fiber.StatusBadRequest, "line items have not been submitted yet (receive_no not issued) — nothing to rate")
	}
	if existingRatedAt != nil {
		return fiber.NewError(fiber.StatusConflict, "this receive document has already been rated")
	}

	var d ICPOReceiveDocument
	err = tx.QueryRow(ctx, `
		UPDATE ic_po_receive_document
		SET score_quality = $1, score_quantity = $2, score_ontime = $3, score_notes = $4,
		    rated_at = NOW(), rated_by = $5, updated_at = NOW(), updated_by = $5
		WHERE id = $6
		RETURNING id, po_id, tax_invoice_no, tax_invoice_date::text, temp_delivery_no, temp_delivery_date::text,
		          receive_no, credit_days, due_date::text, exchange_rate, remarks,
		          score_quality, score_quantity, score_ontime, score_notes, rated_at::text, rated_by,
		          created_at::text, updated_at::text, created_by, updated_by`,
		req.ScoreQuality, req.ScoreQuantity, req.ScoreOntime, req.ScoreNotes, claims.UserID, docID,
	).Scan(&d.ID, &d.POID, &d.TaxInvoiceNo, &d.TaxInvoiceDate, &d.TempDeliveryNo, &d.TempDeliveryDate,
		&d.ReceiveNo, &d.CreditDays, &d.DueDate, &d.ExchangeRate, &d.Remarks,
		&d.ScoreQuality, &d.ScoreQuantity, &d.ScoreOntime, &d.ScoreNotes, &d.RatedAt, &d.RatedBy,
		&d.CreatedAt, &d.UpdatedAt, &d.CreatedBy, &d.UpdatedBy)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save rating: "+err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "data": d})
}

// ICSubmitReceiveDocumentSwagger documents the POST /ic/pos/{poId}/receive-document body for Swagger.
type ICSubmitReceiveDocumentSwagger struct {
	TaxInvoiceNo     string   `json:"tax_invoice_no"`
	TaxInvoiceDate   string   `json:"tax_invoice_date" example:"2026-09-18"`
	TempDeliveryNo   *string  `json:"temp_delivery_no"`
	TempDeliveryDate *string  `json:"temp_delivery_date" example:"2026-09-18"`
	ExchangeRate     *float64 `json:"exchange_rate"`
	Remarks          *string  `json:"remarks"`
}

// ─── PO receive line-items ──────────────────────────────────────────────────

type ICReceiveLine struct {
	LineID      int64   `json:"line_id"`
	LineNo      int     `json:"line_no"`
	CostCode    *string `json:"cost_code"`
	MatCode     string  `json:"mat_code"`
	Description string  `json:"description"`
	QtyOrdered  float64 `json:"qty_ordered"`
	QtyReceived float64 `json:"qty_received"`
	BalReceive  float64 `json:"bal_receive"`
	Unit        *string `json:"unit"`
	UnitPrice   float64 `json:"unit_price"`
}

const icReceiveLinesQuery = `
	SELECT pol.id, pol.line_no,
	       csub.subject_code || cj.job_code || cg.group_code || cs.subgroup_code,
	       pol.mat_code,
	       NULLIF(TRIM(COALESCE(mn.mat_name, '') || ' ' || COALESCE(sp.spec_description, '')), ''),
	       pol.description,
	       pol.qty_ordered, pol.qty_received, u.unit_name, pol.unit_price
	FROM purchase_order_line pol
	LEFT JOIN cost_subgroup cs ON pol.cost_subgroup_id = cs.id
	LEFT JOIN cost_group    cg ON cg.id = cs.group_id
	LEFT JOIN cost_job      cj ON cj.id = cg.job_id
	LEFT JOIN cost_subject  csub ON csub.id = cj.subject_id
	JOIN material_code mc      ON pol.mat_code = mc.mat_code
	LEFT JOIN mat_name mn      ON mc.mat_name_id = mn.id
	LEFT JOIN spec_size sp     ON mc.spec_id = sp.id
	LEFT JOIN unit u           ON mc.unit_id = u.id
	WHERE pol.po_id = $1
	ORDER BY pol.line_no`

// fetchICReceiveLines loads the receive-lines list for a PO. querier is either h.db or a tx,
// so it can be reused by both the GET endpoint and the POST submit response (post-commit read).
func fetchICReceiveLines(ctx context.Context, querier interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
}, poID int64) ([]ICReceiveLine, error) {
	rows, err := querier.Query(ctx, icReceiveLinesQuery, poID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []ICReceiveLine{}
	for rows.Next() {
		var l ICReceiveLine
		var composedDesc, rawDesc *string
		if err := rows.Scan(&l.LineID, &l.LineNo, &l.CostCode, &l.MatCode, &composedDesc,
			&rawDesc, &l.QtyOrdered, &l.QtyReceived, &l.Unit, &l.UnitPrice); err != nil {
			return nil, err
		}
		if composedDesc != nil {
			l.Description = *composedDesc
		} else if rawDesc != nil {
			l.Description = *rawDesc
		}
		l.BalReceive = l.QtyOrdered - l.QtyReceived
		lines = append(lines, l)
	}
	return lines, nil
}

// validateReceiveDocumentBelongsToPO 404s if the receive document doesn't exist or belongs to a
// different PO. querier is either h.db or a tx so it can be reused inside SubmitReceiveLines' tx.
func validateReceiveDocumentBelongsToPO(ctx context.Context, querier interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}, docID, poID int64) error {
	var actualPOID int64
	if err := querier.QueryRow(ctx, `SELECT po_id FROM ic_po_receive_document WHERE id = $1`, docID).Scan(&actualPOID); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "receive document not found")
	}
	if actualPOID != poID {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("receive_document_id %d does not belong to po_id %d", docID, poID))
	}
	return nil
}

// ListReceiveLines godoc
// @Summary      List PO line items for the receiving ("รายการสินค้า") tab
// @Description  Returns po_context (order_type, project_code), lines, and distinct mat_code/cost_code filter options for this PO's lines. receive_document_id is required and validated to belong to this PO — the remaining-balance figures (qty_received, bal_receive) are still computed cumulatively across ALL of the PO's receive documents (they read purchase_order_line.qty_received, a single PO-line-level running total), not scoped to just this one document.
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        poId                path   int  true  "purchase_order.id"
// @Param        receive_document_id query  int  true  "ic_po_receive_document.id (must belong to this PO)"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/pos/{poId}/receive-lines [get]
func (h *ICHandler) ListReceiveLines(c *fiber.Ctx) error {
	ctx := context.Background()

	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}
	docID, err := strconv.ParseInt(c.Query("receive_document_id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "receive_document_id is required")
	}

	var orderType string
	var projectCode *string
	var poStatus string
	if err := h.db.QueryRow(ctx, `SELECT order_type, project_code, status FROM purchase_order WHERE id = $1`, poID).
		Scan(&orderType, &projectCode, &poStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if poStatus != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not approved")
	}
	if err := validateReceiveDocumentBelongsToPO(ctx, h.db, docID, poID); err != nil {
		return err
	}

	lines, err := fetchICReceiveLines(ctx, h.db, poID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list receive lines: "+err.Error())
	}

	matCodeSeen := map[string]bool{}
	costCodeSeen := map[string]bool{}
	matCodeOptions := []string{}
	costCodeOptions := []string{}
	for _, l := range lines {
		if !matCodeSeen[l.MatCode] {
			matCodeSeen[l.MatCode] = true
			matCodeOptions = append(matCodeOptions, l.MatCode)
		}
		if l.CostCode != nil && !costCodeSeen[*l.CostCode] {
			costCodeSeen[*l.CostCode] = true
			costCodeOptions = append(costCodeOptions, *l.CostCode)
		}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"po_context": fiber.Map{
				"order_type":   orderType,
				"project_code": projectCode,
			},
			"lines":             lines,
			"mat_code_options":  matCodeOptions,
			"cost_code_options": costCodeOptions,
		},
	})
}

type icSubmitReceiveLineInput struct {
	LineID     int64   `json:"line_id"`
	ReceiveQty float64 `json:"receive_qty"`
}

type icSubmitReceiveLinesRequest struct {
	ReceiveDocumentID int64                      `json:"receive_document_id"`
	Lines             []icSubmitReceiveLineInput `json:"lines"`
}

// SubmitReceiveLines godoc
// @Summary      Submit this round's PO line-item receiving (stock or cost destination)
// @Description  One DB transaction. receive_document_id is required. The target document is locked FOR UPDATE first, before anything else — a document accepts exactly ONE successful submit: if it already has a receive_no, this 409s with "this receive document has already been received — create a new receive document" rather than accepting more lines against it. Then locks the PO and each touched purchase_order_line (SELECT ... FOR UPDATE), requires purchase_order.status = 'APPROVED' (400 otherwise), validates receive_qty against the line's remaining balance (computed from purchase_order_line.qty_received, a running total across ALL of the PO's receive documents — so total received can never exceed qty ordered no matter how many documents are used), then posts to stock_item/stock_transaction (order_type='stock', tagging each stock_transaction row with this receive_document_id) or ic_project_cost_item/ic_project_cost_item_transaction (order_type='cost', tagging each new transaction row with this receive_document_id), and recomputes purchase_order.status_receive. On success, issues receive_no (por_number_counter, monthly reset) for this document — generated exactly once, at this its one allowed submit.
// @Tags         IC
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        poId  path  int  true  "purchase_order.id"
// @Param        body  body  icSubmitReceiveLinesRequest  true  "receive_document_id + lines to receive (only receive_qty > 0 are processed)"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/pos/{poId}/receive-lines/submit [post]
func (h *ICHandler) SubmitReceiveLines(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "no claims found")
	}

	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}

	var req icSubmitReceiveLinesRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.ReceiveDocumentID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "receive_document_id is required")
	}

	// Only lines with receive_qty > 0 are processed; others are skipped, not an error.
	var toProcess []icSubmitReceiveLineInput
	for _, l := range req.Lines {
		if l.ReceiveQty > 0 {
			toProcess = append(toProcess, l)
		}
	}
	if len(toProcess) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "no lines with receive_qty > 0 to process")
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Lock the target receive document FIRST, before anything else in this transaction — one
	// document accepts exactly one successful submit. If it already has a receive_no, this
	// document was already received; the caller must create a new document instead.
	var docPOID int64
	var lockedReceiveNo *string
	if err := tx.QueryRow(ctx, `
		SELECT po_id, receive_no FROM ic_po_receive_document WHERE id = $1 FOR UPDATE`,
		req.ReceiveDocumentID,
	).Scan(&docPOID, &lockedReceiveNo); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "receive document not found")
	}
	if docPOID != poID {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("receive_document_id %d does not belong to po_id %d", req.ReceiveDocumentID, poID))
	}
	if lockedReceiveNo != nil {
		return fiber.NewError(fiber.StatusConflict, "this receive document has already been received — create a new receive document")
	}

	var orderType string
	var projectCode, warehouseCode *string
	var poStatus string
	if err := tx.QueryRow(ctx, `
		SELECT order_type, project_code, warehouse_code, status FROM purchase_order WHERE id = $1 FOR UPDATE`,
		poID,
	).Scan(&orderType, &projectCode, &warehouseCode, &poStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if poStatus != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not approved")
	}

	for _, in := range toProcess {
		var linePOID, lineNo int64
		var matCode string
		var qtyOrdered, qtyReceived, unitPrice float64
		var costSubgroupID *int64
		if err := tx.QueryRow(ctx, `
			SELECT po_id, line_no, mat_code, qty_ordered, qty_received, unit_price, cost_subgroup_id
			FROM purchase_order_line WHERE id = $1 FOR UPDATE`,
			in.LineID,
		).Scan(&linePOID, &lineNo, &matCode, &qtyOrdered, &qtyReceived, &unitPrice, &costSubgroupID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line_id %d: not found", in.LineID))
		}
		if linePOID != poID {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line_id %d: does not belong to po_id %d", in.LineID, poID))
		}

		remaining := qtyOrdered - qtyReceived
		if in.ReceiveQty > remaining {
			return fiber.NewError(fiber.StatusBadRequest,
				fmt.Sprintf("line_no %d: receive_qty %.4f exceeds remaining balance %.4f", lineNo, in.ReceiveQty, remaining))
		}

		newQtyReceived := qtyReceived + in.ReceiveQty
		lineStatus := "PARTIAL"
		if newQtyReceived >= qtyOrdered {
			lineStatus = "RECEIVED"
		}
		if _, err := tx.Exec(ctx, `
			UPDATE purchase_order_line SET qty_received = $1, status = $2 WHERE id = $3`,
			newQtyReceived, lineStatus, in.LineID,
		); err != nil {
			return err
		}

		switch orderType {
		case "stock":
			if err := icReceiveToStock(ctx, tx, matCode, lineNo, in.ReceiveQty, unitPrice, poID, claims.UserID, req.ReceiveDocumentID); err != nil {
				return err
			}
		case "cost":
			if costSubgroupID == nil {
				return fiber.NewError(fiber.StatusBadRequest,
					fmt.Sprintf("line_no %d: has no cost_subgroup_id — a cost-type PO line must have a cost code to receive into project cost items", lineNo))
			}
			if projectCode == nil || *projectCode == "" {
				return fiber.NewError(fiber.StatusBadRequest,
					fmt.Sprintf("line_no %d: PO has no project_code set — cannot receive into a project cost item", lineNo))
			}
			if err := icReceiveToProjectCost(ctx, tx, *projectCode, matCode, *costSubgroupID, in.ReceiveQty, unitPrice, poID, in.LineID, claims.UserID, req.ReceiveDocumentID); err != nil {
				return err
			}
		default:
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("PO has unsupported order_type %q — expected 'stock' or 'cost'", orderType))
		}
	}

	// Recompute status_receive from the full set of this PO's lines, not just the ones just submitted.
	rows, err := tx.Query(ctx, `SELECT status FROM purchase_order_line WHERE po_id = $1 AND status <> 'CANCELLED'`, poID)
	if err != nil {
		return err
	}
	var allReceived, anyNonOpen = true, false
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			rows.Close()
			return err
		}
		if status != "RECEIVED" {
			allReceived = false
		}
		if status == "RECEIVED" || status == "PARTIAL" {
			anyNonOpen = true
		}
	}
	rows.Close()

	if allReceived {
		if _, err := tx.Exec(ctx, `UPDATE purchase_order SET status_receive = 'RECEIVED', updated_at = NOW() WHERE id = $1`, poID); err != nil {
			return err
		}
	} else if anyNonOpen {
		if _, err := tx.Exec(ctx, `UPDATE purchase_order SET status_receive = 'PARTIALLY_RECEIVED', updated_at = NOW() WHERE id = $1`, poID); err != nil {
			return err
		}
	}
	// else: all lines still OPEN — leave status_receive unchanged.

	// This document is generated its receive_no here, at its one and only successful submit —
	// we already rejected with 409 above if it had a receive_no, so this document is guaranteed
	// to still be un-received at this point (still under the same FOR UPDATE lock taken above).
	ym := time.Now().Format("200601")
	var seq int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO por_number_counter (year_month, last_seq) VALUES ($1, 1)
		ON CONFLICT (year_month) DO UPDATE SET last_seq = por_number_counter.last_seq + 1
		RETURNING last_seq`, ym,
	).Scan(&seq); err != nil {
		return err
	}
	receiveNoVal := fmt.Sprintf("POR-%s-%04d", ym, seq)
	receiveNo := &receiveNoVal

	if _, err := tx.Exec(ctx, `UPDATE ic_po_receive_document SET receive_no = $1 WHERE id = $2`, receiveNoVal, req.ReceiveDocumentID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	lines, err := fetchICReceiveLines(ctx, h.db, poID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "receive committed but failed to reload lines: "+err.Error())
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"receive_no": *receiveNo, "lines": lines}})
}

type ICDocumentLine struct {
	CostCode    *string `json:"cost_code"`
	MatCode     string  `json:"mat_code"`
	Description *string `json:"description"`
	Unit        *string `json:"unit"`
	UnitPrice   float64 `json:"unit_price"`
	Qty         float64 `json:"qty"`
}

// GetReceiveDocumentLines godoc
// @Summary      List the lines actually received under one specific receive document (print feed)
// @Description  Returns only the lines posted under this one receive document — not all lines of the PO. Both order_type='cost' (ic_project_cost_item_transaction.receive_document_id) and order_type='stock' (stock_transaction.receive_document_id) now carry this FK and are scoped the same way, returning scoped_by_document=true. FALLBACK for stock-type only: rows written before this FK existed (or for a PO that historically had more than one document, since the backfill only linked POs with exactly one document) have receive_document_id IS NULL — if the scoped query returns nothing, this falls back to every 'IN' stock_transaction row for the whole PO that still has receive_document_id IS NULL, returning scoped_by_document=false with a warning field. This fallback path never mixes in rows that are already linked to a *different* document.
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        poId   path  int  true  "purchase_order.id"
// @Param        docId  path  int  true  "ic_po_receive_document.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/pos/{poId}/receive-documents/{docId}/lines [get]
func (h *ICHandler) GetReceiveDocumentLines(c *fiber.Ctx) error {
	ctx := context.Background()

	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}
	docID, err := strconv.ParseInt(c.Params("docId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid docId")
	}

	var orderType string
	if err := h.db.QueryRow(ctx, `SELECT order_type FROM purchase_order WHERE id = $1`, poID).Scan(&orderType); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if err := validateReceiveDocumentBelongsToPO(ctx, h.db, docID, poID); err != nil {
		return err
	}

	if orderType == "cost" {
		rows, err := h.db.Query(ctx, `
			SELECT csub.subject_code || cj.job_code || cg.group_code || cs.subgroup_code,
			       t.qty, pool.mat_code, pool.item_name, COALESCE(u.unit_name, pool.unit), COALESCE(pol.unit_price, 0)
			FROM ic_project_receipt_pool_transaction t
			JOIN ic_project_receipt_pool pool ON pool.id = t.receipt_pool_id
			LEFT JOIN purchase_order_line pol ON pol.id = t.po_line_id
			LEFT JOIN material_code mc ON mc.mat_code = pool.mat_code
			LEFT JOIN unit u ON u.id = mc.unit_id
			LEFT JOIN cost_subgroup cs ON pool.cost_subgroup_id = cs.id
			LEFT JOIN cost_group    cg ON cg.id = cs.group_id
			LEFT JOIN cost_job      cj ON cj.id = cg.job_id
			LEFT JOIN cost_subject  csub ON csub.id = cj.subject_id
			WHERE t.receive_document_id = $1 AND t.txn_type = 'RECEIVE' AND t.qty > 0
			ORDER BY t.id`, docID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to list document lines: "+err.Error())
		}
		defer rows.Close()

		lines := []ICDocumentLine{}
		for rows.Next() {
			var l ICDocumentLine
			if err := rows.Scan(&l.CostCode, &l.Qty, &l.MatCode, &l.Description, &l.Unit, &l.UnitPrice); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to scan document line: "+err.Error())
			}
			lines = append(lines, l)
		}

		return c.JSON(fiber.Map{
			"success": true,
			"data": fiber.Map{
				"scoped_by_document": true,
				"lines":              lines,
			},
		})
	}

	// order_type == 'stock': scoped query first — stock_transaction.receive_document_id now
	// exists and is written on every new receive (icReceiveToStock).
	scopedRows, err := h.db.Query(ctx, `
		SELECT NULL::text AS cost_code, si.mat_code, si.item_name, u.unit_name, 0::numeric AS unit_price, st.qty
		FROM stock_transaction st
		JOIN stock_item si ON si.id = st.item_id
		LEFT JOIN material_code mc ON mc.mat_code = si.mat_code
		LEFT JOIN unit u ON mc.unit_id = u.id
		WHERE st.receive_document_id = $1 AND st.txn_type = 'IN'
		ORDER BY st.id`, docID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list document lines: "+err.Error())
	}
	scopedLines := []ICDocumentLine{}
	for scopedRows.Next() {
		var l ICDocumentLine
		if err := scopedRows.Scan(&l.CostCode, &l.MatCode, &l.Description, &l.Unit, &l.UnitPrice, &l.Qty); err != nil {
			scopedRows.Close()
			return fiber.NewError(fiber.StatusInternalServerError, "failed to scan document line: "+err.Error())
		}
		scopedLines = append(scopedLines, l)
	}
	scopedRows.Close()

	if len(scopedLines) > 0 {
		return c.JSON(fiber.Map{
			"success": true,
			"data": fiber.Map{
				"scoped_by_document": true,
				"lines":              scopedLines,
			},
		})
	}

	// Nothing linked to this document specifically — fall back to this PO's still-unlinked
	// (receive_document_id IS NULL) rows only, never rows already linked to a different document.
	rows, err := h.db.Query(ctx, `
		SELECT NULL::text AS cost_code, si.mat_code, si.item_name, u.unit_name, 0::numeric AS unit_price, st.qty
		FROM stock_transaction st
		JOIN stock_item si ON si.id = st.item_id
		LEFT JOIN material_code mc ON mc.mat_code = si.mat_code
		LEFT JOIN unit u ON mc.unit_id = u.id
		WHERE st.ref_doc_type = 'PO' AND st.ref_doc_id = $1 AND st.txn_type = 'IN' AND st.receive_document_id IS NULL
		ORDER BY st.id`, poID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list document lines: "+err.Error())
	}
	defer rows.Close()

	lines := []ICDocumentLine{}
	for rows.Next() {
		var l ICDocumentLine
		if err := rows.Scan(&l.CostCode, &l.MatCode, &l.Description, &l.Unit, &l.UnitPrice, &l.Qty); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to scan document line: "+err.Error())
		}
		lines = append(lines, l)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"scoped_by_document": false,
			"warning":            "no stock_transaction rows are linked to this specific receive document — these are this PO's unlinked (receive_document_id IS NULL) 'IN' transactions, likely written before this document existed or before the receive_document_id column was added",
			"lines":              lines,
		},
	})
}

// icReceiveToStock posts a PO-line receive into stock_item/stock_transaction. Every mat_code is
// expected to already have a stock_item row created at material-master-creation time — this
// function never auto-creates one. A missing row is a data setup problem (a material exists
// without its stock_item counterpart) and is reported as an error naming mat_code and line_no,
// not silently papered over. receiveDocumentID is stamped on the new stock_transaction row so it
// can be attributed back to the specific receive document it was submitted under (a PO can have
// many), mirroring icReceiveToProjectCost's receive_document_id tagging on the cost side.
func icReceiveToStock(ctx context.Context, tx pgx.Tx, matCode string, lineNo int64, receiveQty, unitPrice float64, poID, userID, receiveDocumentID int64) error {
	var itemID int64
	var locationCode string
	if err := tx.QueryRow(ctx, `SELECT id, location_code FROM stock_item WHERE mat_code = $1 FOR UPDATE`, matCode).
		Scan(&itemID, &locationCode); err != nil {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("line_no %d: mat_code %s has no stock_item row — material master is missing its stock_item counterpart", lineNo, matCode))
	}

	var qtyBefore float64
	if err := tx.QueryRow(ctx, `SELECT qty FROM stock_item WHERE id = $1 FOR UPDATE`, itemID).Scan(&qtyBefore); err != nil {
		return err
	}
	qtyAfter := qtyBefore + receiveQty

	if _, err := tx.Exec(ctx, `
		UPDATE stock_item SET qty = $1, unit_cost = $2, updated_at = NOW() WHERE id = $3`,
		qtyAfter, unitPrice, itemID,
	); err != nil {
		return fmt.Errorf("mat_code %s: failed to update stock_item qty: %w", matCode, err)
	}

	txnNo, err := generateTxnNo(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO stock_transaction (txn_no, txn_type, item_id, qty, qty_before, qty_after, ref_doc_type, ref_doc_id, to_location, txn_date, created_by, receive_document_id)
		VALUES ($1, 'IN', $2, $3, $4, $5, 'PO', $6, $7, CURRENT_DATE, $8, $9)`,
		txnNo, itemID, receiveQty, qtyBefore, qtyAfter, poID, locationCode, userID, receiveDocumentID,
	); err != nil {
		return fmt.Errorf("mat_code %s: failed to insert stock_transaction: %w", matCode, err)
	}

	return nil
}

// icReceiveToProjectCost posts a PO-line receive into ic_project_cost_item/ic_project_cost_item_transaction,
// auto-creating the cost item row if one doesn't yet exist for (project_code, mat_code, cost_subgroup_id).
// receiveDocumentID is stamped on the new transaction row so it can be attributed back to the
// specific receive document it was submitted under (a PO can have many).
func icReceiveToProjectCost(ctx context.Context, tx pgx.Tx, projectCode, matCode string, costSubgroupID int64, receiveQty, unitPrice float64, poID, poLineID, userID, receiveDocumentID int64) error {
	var poolID int64
	err := tx.QueryRow(ctx, `
		SELECT id FROM ic_project_receipt_pool
		WHERE project_code = $1 AND mat_code = $2 AND cost_subgroup_id = $3 FOR UPDATE`,
		projectCode, matCode, costSubgroupID,
	).Scan(&poolID)
	if err != nil {
		var itemName string
		var unitName *string
		if err := tx.QueryRow(ctx, `
			SELECT NULLIF(TRIM(COALESCE(mn.mat_name, '') || ' ' || COALESCE(sp.spec_description, '')), mc.mat_code),
			       u.unit_name
			FROM material_code mc
			LEFT JOIN mat_name mn  ON mc.mat_name_id = mn.id
			LEFT JOIN spec_size sp ON mc.spec_id = sp.id
			LEFT JOIN unit u       ON mc.unit_id = u.id
			WHERE mc.mat_code = $1`, matCode,
		).Scan(&itemName, &unitName); err != nil {
			return fmt.Errorf("mat_code %s: not found in material_code — cannot auto-create ic_project_receipt_pool", matCode)
		}
		if itemName == "" {
			itemName = matCode
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO ic_project_receipt_pool (project_code, mat_code, cost_subgroup_id, item_name, unit, qty_received, qty_issued, last_unit_cost)
			VALUES ($1, $2, $3, $4, $5, 0, 0, 0)
			RETURNING id`,
			projectCode, matCode, costSubgroupID, itemName, unitName,
		).Scan(&poolID); err != nil {
			return fmt.Errorf("mat_code %s: failed to auto-create ic_project_receipt_pool: %w", matCode, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE ic_project_receipt_pool
		SET qty_received = qty_received + $1, last_unit_cost = $2, updated_at = NOW() WHERE id = $3`,
		receiveQty, unitPrice, poolID,
	); err != nil {
		return fmt.Errorf("mat_code %s: failed to update ic_project_receipt_pool: %w", matCode, err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO ic_project_receipt_pool_transaction (receipt_pool_id, txn_type, qty, ref_type, po_id, po_line_id, receive_document_id, created_by)
		VALUES ($1, 'RECEIVE', $2, 'PO', $3, $4, $5, $6)`,
		poolID, receiveQty, poID, poLineID, receiveDocumentID, userID,
	); err != nil {
		return fmt.Errorf("mat_code %s: failed to insert ic_project_receipt_pool_transaction: %w", matCode, err)
	}

	return nil
}

// ─── PO return ──────────────────────────────────────────────────────────────

// ListReturnPOs godoc
// @Summary      List Purchase Orders under a project eligible for return
// @Description  Same shape as ListProjectPOs, but filters status_receive IN ('PARTIALLY_RECEIVED','RECEIVED') (no pending/completed toggle) and includes receive_no. Also requires purchase_order.status = 'APPROVED', same as ListProjectPOs. Same representative-document caveat as ListProjectPOs applies (a PO can have many receive documents; this list shows one representative document's fields per PO).
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        projectId  path   int     true   "project.id"
// @Param        search     query  string  false  "search by po_no or pr_no"
// @Param        page       query  int     false  "page number"  default(1)
// @Param        page_size  query  int     false  "page size"    default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectId}/return-pos [get]
func (h *ICHandler) ListReturnPOs(c *fiber.Ctx) error {
	ctx := context.Background()

	projectID, err := strconv.ParseInt(c.Params("projectId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid projectId")
	}

	projectCode, projectName, err := h.resolveProjectCode(ctx, projectID)
	if err != nil {
		return err
	}

	search := strings.TrimSpace(c.Query("search"))
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	where := []string{"po.project_code = $1", "po.status = 'APPROVED'", "po.status_receive IN ('PARTIALLY_RECEIVED','RECEIVED')"}
	args := []interface{}{projectCode}
	idx := 2

	if search != "" {
		where = append(where, fmt.Sprintf("(po.po_no ILIKE $%d OR pr.pr_no ILIKE $%d)", idx, idx))
		args = append(args, "%"+search+"%")
		idx++
	}

	whereStr := strings.Join(where, " AND ")
	// Same LATERAL fix as ListProjectPOs — ic_po_receive_document.po_id is no longer unique.
	fromJoin := `FROM purchase_order po
		LEFT JOIN purchase_request pr ON po.pr_id = pr.id
		LEFT JOIN supplier s ON po.supplier_id = s.id
		LEFT JOIN LATERAL (
			SELECT due_date, credit_days, score_quality, score_quantity, score_ontime, score_notes, rated_at, rated_by, receive_no
			FROM ic_po_receive_document
			WHERE po_id = po.id
			ORDER BY (receive_no IS NULL) ASC, created_at DESC
			LIMIT 1
		) rd ON true`

	var total int64
	if err := h.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) %s WHERE %s`, fromJoin, whereStr), args...,
	).Scan(&total); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to count POs: "+err.Error())
	}

	args = append(args, size, offset)
	rows, err := h.db.Query(ctx,
		fmt.Sprintf(`SELECT po.id, po.po_no, po.po_date::text, pr.pr_no, s.supplier_name,
			po.expected_date::text, po.status_receive, rd.due_date::text, rd.credit_days,
			rd.score_quality, rd.score_quantity, rd.score_ontime, rd.score_notes, rd.rated_at::text, rd.rated_by,
			rd.receive_no
			%s WHERE %s
			ORDER BY po.po_date DESC, po.po_no DESC
			LIMIT $%d OFFSET $%d`, fromJoin, whereStr, idx, idx+1), args...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list POs: "+err.Error())
	}
	defer rows.Close()

	type icReturnPO struct {
		ICProjectPO
		ReceiveNo *string `json:"receive_no"`
	}

	items := []icReturnPO{}
	for rows.Next() {
		var p icReturnPO
		if err := rows.Scan(&p.POID, &p.PONo, &p.PODate, &p.PRNo, &p.SupplierName,
			&p.ExpectedDate, &p.StatusReceive, &p.DueDate, &p.CreditDays,
			&p.ScoreQuality, &p.ScoreQuantity, &p.ScoreOntime, &p.ScoreNotes, &p.RatedAt, &p.RatedBy,
			&p.ReceiveNo); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to scan PO: "+err.Error())
		}
		p.ProjectCode = projectCode
		p.ProjectName = &projectName
		items = append(items, p)
	}

	totalPages := int(total) / size
	if int(total)%size != 0 || total == 0 {
		totalPages++
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages,
		},
	})
}

type ICReturnLine struct {
	LineID      int64   `json:"line_id"`
	LineNo      int     `json:"line_no"`
	CostCode    *string `json:"cost_code"`
	MatCode     string  `json:"mat_code"`
	Description string  `json:"description"`
	Unit        *string `json:"unit"`
	UnitPrice   float64 `json:"unit_price"`
	QtyReceived float64 `json:"qty_received"`
}

const icReturnLinesQuery = `
	SELECT pol.id, pol.line_no,
	       csub.subject_code || cj.job_code || cg.group_code || cs.subgroup_code,
	       pol.mat_code,
	       NULLIF(TRIM(COALESCE(mn.mat_name, '') || ' ' || COALESCE(sp.spec_description, '')), ''),
	       pol.description,
	       u.unit_name, pol.unit_price, pol.qty_received
	FROM purchase_order_line pol
	LEFT JOIN cost_subgroup cs ON pol.cost_subgroup_id = cs.id
	LEFT JOIN cost_group    cg ON cg.id = cs.group_id
	LEFT JOIN cost_job      cj ON cj.id = cg.job_id
	LEFT JOIN cost_subject  csub ON csub.id = cj.subject_id
	JOIN material_code mc      ON pol.mat_code = mc.mat_code
	LEFT JOIN mat_name mn      ON mc.mat_name_id = mn.id
	LEFT JOIN spec_size sp     ON mc.spec_id = sp.id
	LEFT JOIN unit u           ON mc.unit_id = u.id
	WHERE pol.po_id = $1
	ORDER BY pol.line_no`

func fetchICReturnLines(ctx context.Context, querier interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
}, poID int64) ([]ICReturnLine, error) {
	rows, err := querier.Query(ctx, icReturnLinesQuery, poID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []ICReturnLine{}
	for rows.Next() {
		var l ICReturnLine
		var composedDesc, rawDesc *string
		if err := rows.Scan(&l.LineID, &l.LineNo, &l.CostCode, &l.MatCode, &composedDesc,
			&rawDesc, &l.Unit, &l.UnitPrice, &l.QtyReceived); err != nil {
			return nil, err
		}
		if composedDesc != nil {
			l.Description = *composedDesc
		} else if rawDesc != nil {
			l.Description = *rawDesc
		}
		lines = append(lines, l)
	}
	return lines, nil
}

// ListReturnLines godoc
// @Summary      List PO line items for the return ("คืนสินค้า") tab
// @Description  Same shape as receive-lines, but for return: qty_received is the current cumulative received qty, which is the max returnable amount for the line. Requires purchase_order.status = 'APPROVED' (400 otherwise).
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        poId  path  int  true  "purchase_order.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/pos/{poId}/return-lines [get]
func (h *ICHandler) ListReturnLines(c *fiber.Ctx) error {
	ctx := context.Background()

	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}

	var orderType string
	var projectCode *string
	var poStatus string
	if err := h.db.QueryRow(ctx, `SELECT order_type, project_code, status FROM purchase_order WHERE id = $1`, poID).
		Scan(&orderType, &projectCode, &poStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if poStatus != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not approved")
	}

	lines, err := fetchICReturnLines(ctx, h.db, poID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list return lines: "+err.Error())
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"po_context": fiber.Map{
				"order_type":   orderType,
				"project_code": projectCode,
			},
			"lines": lines,
		},
	})
}

type icSubmitReturnLineInput struct {
	LineID    int64   `json:"line_id"`
	ReturnQty float64 `json:"return_qty"`
}

type icSubmitReturnLinesRequest struct {
	Remarks *string                   `json:"remarks"`
	Lines   []icSubmitReturnLineInput `json:"lines"`
}

// SubmitReturnLines godoc
// @Summary      Submit this round's PO line-item return (stock or cost source)
// @Description  One DB transaction. Locks the PO and each touched purchase_order_line (SELECT ... FOR UPDATE), requires purchase_order.status = 'APPROVED' (400 otherwise), validates return_qty against the line's current qty_received, then subtracts from stock_item/stock_transaction (order_type='stock') or ic_project_cost_item/ic_project_cost_item_transaction (order_type='cost'), and recomputes purchase_order.status_receive. Does not touch receive_no.
// @Tags         IC
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        poId  path  int  true  "purchase_order.id"
// @Param        body  body  icSubmitReturnLinesRequest  true  "Lines to return (only return_qty > 0 are processed)"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/pos/{poId}/return-lines/submit [post]
func (h *ICHandler) SubmitReturnLines(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "no claims found")
	}

	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}

	var req icSubmitReturnLinesRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	var toProcess []icSubmitReturnLineInput
	for _, l := range req.Lines {
		if l.ReturnQty > 0 {
			toProcess = append(toProcess, l)
		}
	}
	if len(toProcess) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "no lines with return_qty > 0 to process")
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var orderType string
	var projectCode *string
	var poStatus string
	if err := tx.QueryRow(ctx, `
		SELECT order_type, project_code, status FROM purchase_order WHERE id = $1 FOR UPDATE`,
		poID,
	).Scan(&orderType, &projectCode, &poStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if poStatus != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not approved")
	}

	for _, in := range toProcess {
		var linePOID, lineNo int64
		var matCode string
		var qtyOrdered, qtyReceived, unitPrice float64
		var costSubgroupID *int64
		if err := tx.QueryRow(ctx, `
			SELECT po_id, line_no, mat_code, qty_ordered, qty_received, unit_price, cost_subgroup_id
			FROM purchase_order_line WHERE id = $1 FOR UPDATE`,
			in.LineID,
		).Scan(&linePOID, &lineNo, &matCode, &qtyOrdered, &qtyReceived, &unitPrice, &costSubgroupID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line_id %d: not found", in.LineID))
		}
		if linePOID != poID {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line_id %d: does not belong to po_id %d", in.LineID, poID))
		}
		if in.ReturnQty > qtyReceived {
			return fiber.NewError(fiber.StatusBadRequest,
				fmt.Sprintf("line_no %d: return_qty %.4f exceeds qty_received %.4f", lineNo, in.ReturnQty, qtyReceived))
		}

		newQtyReceived := qtyReceived - in.ReturnQty
		var lineStatus string
		switch {
		case newQtyReceived <= 0:
			lineStatus = "OPEN"
		case newQtyReceived >= qtyOrdered:
			lineStatus = "RECEIVED"
		default:
			lineStatus = "PARTIAL"
		}
		if _, err := tx.Exec(ctx, `
			UPDATE purchase_order_line SET qty_received = $1, status = $2 WHERE id = $3`,
			newQtyReceived, lineStatus, in.LineID,
		); err != nil {
			return err
		}

		switch orderType {
		case "stock":
			if err := icReturnFromStock(ctx, tx, matCode, lineNo, in.ReturnQty, poID, claims.UserID, req.Remarks); err != nil {
				return err
			}
		case "cost":
			if costSubgroupID == nil {
				return fiber.NewError(fiber.StatusBadRequest,
					fmt.Sprintf("line_no %d: has no cost_subgroup_id — a cost-type PO line must have a cost code to return from project cost items", lineNo))
			}
			if projectCode == nil || *projectCode == "" {
				return fiber.NewError(fiber.StatusBadRequest,
					fmt.Sprintf("line_no %d: PO has no project_code set — cannot return from a project cost item", lineNo))
			}
			if err := icReturnFromProjectCost(ctx, tx, *projectCode, matCode, *costSubgroupID, in.ReturnQty, unitPrice, poID, in.LineID, claims.UserID, req.Remarks); err != nil {
				return err
			}
		default:
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("PO has unsupported order_type %q — expected 'stock' or 'cost'", orderType))
		}
	}

	// Recompute status_receive from the full set of this PO's lines, same aggregation rule as receive-lines.
	rows, err := tx.Query(ctx, `SELECT status FROM purchase_order_line WHERE po_id = $1 AND status <> 'CANCELLED'`, poID)
	if err != nil {
		return err
	}
	var allReceived, allOpen, anyNonOpen = true, true, false
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			rows.Close()
			return err
		}
		if status != "RECEIVED" {
			allReceived = false
		}
		if status != "OPEN" {
			allOpen = false
		}
		if status == "RECEIVED" || status == "PARTIAL" {
			anyNonOpen = true
		}
	}
	rows.Close()

	switch {
	case allReceived:
		if _, err := tx.Exec(ctx, `UPDATE purchase_order SET status_receive = 'RECEIVED', updated_at = NOW() WHERE id = $1`, poID); err != nil {
			return err
		}
	case allOpen:
		// Fully returned back to unreceived. Defaulting to 'SENT' (the PO was already sent to the
		// supplier and is presumed still an open order awaiting receipt again) rather than
		// 'NOT_SENT' (which would incorrectly imply the PO was never sent at all). Flagged to the
		// user for confirmation before this ships — see the accompanying message.
		if _, err := tx.Exec(ctx, `UPDATE purchase_order SET status_receive = 'SENT', updated_at = NOW() WHERE id = $1`, poID); err != nil {
			return err
		}
	case anyNonOpen:
		if _, err := tx.Exec(ctx, `UPDATE purchase_order SET status_receive = 'PARTIALLY_RECEIVED', updated_at = NOW() WHERE id = $1`, poID); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	lines, err := fetchICReturnLines(ctx, h.db, poID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "return committed but failed to reload lines: "+err.Error())
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"lines": lines}})
}

// icReturnFromStock posts a PO-line return by subtracting from stock_item/stock_transaction.
// Mirrors icReceiveToStock but never auto-creates a stock_item row — a missing row for a mat_code
// that was previously received into stock would indicate a deeper data problem, not something to
// paper over here.
func icReturnFromStock(ctx context.Context, tx pgx.Tx, matCode string, lineNo int64, returnQty float64, poID, userID int64, remarks *string) error {
	var itemID int64
	var locationCode string
	var qtyBefore float64
	if err := tx.QueryRow(ctx, `SELECT id, location_code, qty FROM stock_item WHERE mat_code = $1 FOR UPDATE`, matCode).
		Scan(&itemID, &locationCode, &qtyBefore); err != nil {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("line_no %d: mat_code %s has no stock_item row", lineNo, matCode))
	}
	if returnQty > qtyBefore {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("line_no %d: mat_code %s return_qty %.4f exceeds available stock qty %.4f", lineNo, matCode, returnQty, qtyBefore))
	}

	qtyAfter := qtyBefore - returnQty
	if _, err := tx.Exec(ctx, `UPDATE stock_item SET qty = $1, updated_at = NOW() WHERE id = $2`, qtyAfter, itemID); err != nil {
		return fmt.Errorf("mat_code %s: failed to update stock_item qty: %w", matCode, err)
	}

	txnNo, err := generateTxnNo(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO stock_transaction (txn_no, txn_type, item_id, qty, qty_before, qty_after, ref_doc_type, ref_doc_id, from_location, txn_date, remarks, created_by)
		VALUES ($1, 'OUT', $2, $3, $4, $5, 'PO_RETURN', $6, $7, CURRENT_DATE, $8, $9)`,
		txnNo, itemID, returnQty, qtyBefore, qtyAfter, poID, locationCode, remarks, userID,
	); err != nil {
		return fmt.Errorf("mat_code %s: failed to insert stock_transaction: %w", matCode, err)
	}

	return nil
}

// icReturnFromProjectCost posts a PO-line return by subtracting from
// ic_project_cost_item/ic_project_cost_item_transaction. Never auto-creates the cost item row —
// a return implies the item must already exist from a prior receive.
func icReturnFromProjectCost(ctx context.Context, tx pgx.Tx, projectCode, matCode string, costSubgroupID int64, returnQty, unitPrice float64, poID, poLineID, userID int64, remarks *string) error {
	var costItemID int64
	var qtyBefore float64
	if err := tx.QueryRow(ctx, `
		SELECT id, qty_on_hand FROM ic_project_cost_item
		WHERE project_code = $1 AND mat_code = $2 AND cost_subgroup_id = $3 FOR UPDATE`,
		projectCode, matCode, costSubgroupID,
	).Scan(&costItemID, &qtyBefore); err != nil {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("mat_code %s: no ic_project_cost_item row for project %s / cost_subgroup_id %d — cannot return an item that was never received", matCode, projectCode, costSubgroupID))
	}
	if returnQty > qtyBefore {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("mat_code %s: return_qty %.4f exceeds available qty_on_hand %.4f", matCode, returnQty, qtyBefore))
	}

	qtyAfter := qtyBefore - returnQty
	if _, err := tx.Exec(ctx, `
		UPDATE ic_project_cost_item SET qty_on_hand = $1, updated_at = NOW() WHERE id = $2`,
		qtyAfter, costItemID,
	); err != nil {
		return fmt.Errorf("mat_code %s: failed to update ic_project_cost_item: %w", matCode, err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO ic_project_cost_item_transaction (project_cost_item_id, po_id, po_line_id, qty, qty_before, qty_after, unit_cost, remarks, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		costItemID, poID, poLineID, -returnQty, qtyBefore, qtyAfter, unitPrice, remarks, userID,
	); err != nil {
		return fmt.Errorf("mat_code %s: failed to insert ic_project_cost_item_transaction: %w", matCode, err)
	}

	return nil
}
