package models

import "time"

// ── StockCategory ──────────────────────────────────────────

type StockCategory struct {
	ID          int64     `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

// ── StockItem ──────────────────────────────────────────────

type StockItem struct {
	ID           int64     `json:"id"`
	MatCode      string    `json:"mat_code"`
	ItemName     string    `json:"item_name"`
	Description  *string   `json:"description"`
	CategoryID   *int64    `json:"category_id"`
	CategoryName *string   `json:"category_name"`
	ItemType     string    `json:"item_type"`     // RETURNABLE | CONSUMABLE
	TrackingType string    `json:"tracking_type"` // sku | serial
	Unit         string    `json:"unit"`
	Qty          float64   `json:"qty"`
	UnitCost     float64   `json:"unit_cost"`
	QRCode       *string   `json:"qr_code"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateStockItemRequest struct {
	MatCode     string  `json:"mat_code"`
	ItemName    string  `json:"item_name"`
	Description *string `json:"description"`
	CategoryID  *int64  `json:"category_id"`
	ItemType    string  `json:"item_type"`
	Unit        string  `json:"unit"`
	Qty         float64 `json:"qty"`
	UnitCost    float64 `json:"unit_cost"`
}

type UpdateStockItemRequest = CreateStockItemRequest

type StockItemListFilter struct {
	Search   string `query:"search"`
	ItemType string `query:"item_type"`
	IsActive string `query:"is_active"`
	Page     int    `query:"page"`
	PageSize int    `query:"page_size"`
}

// ── StockInventory ─────────────────────────────────────────

type StockInventory struct {
	ID            int64   `json:"id"`
	ItemID        int64   `json:"item_id"`
	MatCode       string  `json:"mat_code"`
	ItemName      string  `json:"item_name"`
	Unit          string  `json:"unit"`
	MinQty        float64 `json:"min_qty"`
	LocationCode  string  `json:"location_code"`
	LocationName  string  `json:"location_name"`
	WarehouseCode string  `json:"warehouse_code"`
	QtyOnHand     float64 `json:"qty_on_hand"`
	QtyReserved   float64 `json:"qty_reserved"`
	QtyAvailable  float64 `json:"qty_available"`
}

type StockInventoryFilter struct {
	Search        string `query:"search"`
	WarehouseCode string `query:"warehouse_code"`
	LocationCode  string `query:"location_code"`
	LowStockOnly  bool   `query:"low_stock_only"`
	Page          int    `query:"page"`
	PageSize      int    `query:"page_size"`
}

type BatchStockLookupRequest struct {
	Codes []string `json:"codes"`
}

type AdjustStockRequest struct {
	ItemID       int64   `json:"item_id"`
	LocationCode string  `json:"location_code"`
	Qty          float64 `json:"qty"`
	TxnType      string  `json:"txn_type"`
	Remarks      *string `json:"remarks"`
}

type TransferStockRequest struct {
	ItemID       int64   `json:"item_id"`
	FromLocation string  `json:"from_location"`
	ToLocation   string  `json:"to_location"`
	Qty          float64 `json:"qty"`
	Remarks      *string `json:"remarks"`
}

// ── StockTransaction ───────────────────────────────────────

type StockTransaction struct {
	ID            int64     `json:"id"`
	TxnNo         string    `json:"txn_no"`
	TxnType       string    `json:"txn_type"`
	ItemID        int64     `json:"item_id"`
	MatCode       string    `json:"mat_code"`
	ItemName      string    `json:"item_name"`
	FromLocation  *string   `json:"from_location"`
	ToLocation    *string   `json:"to_location"`
	Qty           float64   `json:"qty"`
	QtyBefore     *float64  `json:"qty_before,omitempty"`
	QtyAfter      *float64  `json:"qty_after,omitempty"`
	RefDocType    *string   `json:"ref_doc_type"`
	RefDocID      *int64    `json:"ref_doc_id"`
	Remarks       *string   `json:"remarks"`
	TxnDate       string    `json:"txn_date"`
	CreatedByName string    `json:"created_by_name"`
	CreatedAt     time.Time `json:"created_at"`

	// Populated only when ref_doc_type = 'GRN' — joined through grn -> purchase_order.
	GRNPOID          *int64   `json:"po_id,omitempty"`
	GRNPONo          *string  `json:"po_no,omitempty"`
	GRNScoreQuality  *int     `json:"score_quality,omitempty"`
	GRNScoreQuantity *int     `json:"score_quantity,omitempty"`
	GRNScoreOntime   *int     `json:"score_ontime,omitempty"`
	GRNScoreNotes    *string  `json:"score_notes,omitempty"`
}

type CreateStockTransactionRequest struct {
	TxnType      string  `json:"txn_type"`
	ItemID       int64   `json:"item_id"`
	FromLocation *string `json:"from_location"`
	ToLocation   *string `json:"to_location"`
	Qty          float64 `json:"qty"`
	Remarks      *string `json:"remarks"`
}

type StockTransactionFilter struct {
	TxnType  string `query:"txn_type"`
	Type     string `query:"type"` // alias for txn_type, per GET /stock/transactions?type=IN
	DateFrom string `query:"date_from"`
	DateTo   string `query:"date_to"`
	Search   string `query:"search"`
	MatCode  string `query:"mat_code"`
	PONo     string `query:"po_no"`
	Page     int    `query:"page"`
	PageSize int    `query:"page_size"`
}

// ── BorrowRequest ──────────────────────────────────────────

type BorrowRequest struct {
	ID             int64        `json:"id"`
	BorrowNo       string       `json:"borrow_no"`
	Status         string       `json:"status"`
	Purpose        *string      `json:"purpose"`
	BorrowerName   string       `json:"borrower_name"`
	BorrowerID     int64        `json:"borrower_id"`
	ApprovedByName *string      `json:"approved_by_name"`
	BorrowDate     string       `json:"borrow_date"`
	ExpectedReturn *string      `json:"expected_return"`
	ActualReturn   *string      `json:"actual_return"`
	Remarks        *string      `json:"remarks"`
	Lines          []BorrowLine `json:"lines,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
}

type BorrowLine struct {
	ID           int64    `json:"id"`
	BorrowID     int64    `json:"borrow_id"`
	LineNo       int      `json:"line_no"`
	StockItemID  int64    `json:"stock_item_id"`
	MatCode      string   `json:"mat_code"`
	ItemName     string   `json:"item_name"`
	ItemType     string   `json:"item_type"`
	LocationCode string   `json:"location_code"`
	LocationName string   `json:"location_name"`
	Unit         string   `json:"unit"`
	QtyRequested float64  `json:"qty_requested"`
	QtyApproved  *float64 `json:"qty_approved"`
	QtyBorrowed  float64  `json:"qty_borrowed"`
	QtyReturned  float64  `json:"qty_returned"`
	ConditionOut *string  `json:"condition_out"`
	ConditionIn  *string  `json:"condition_in"`
	Remarks      *string  `json:"remarks"`
}

type CreateBorrowRequest struct {
	Purpose        *string         `json:"purpose"`
	ExpectedReturn *string         `json:"expected_return"`
	Remarks        *string         `json:"remarks"`
	Lines          []BorrowLineReq `json:"lines"`
}

type BorrowLineReq struct {
	LineNo       int     `json:"line_no"`
	StockItemID  int64   `json:"stock_item_id"`
	LocationCode string  `json:"location_code"`
	QtyRequested float64 `json:"qty_requested"`
	Remarks      *string `json:"remarks"`
}

type BorrowApprovalRequest struct {
	Action   string  `json:"action"`
	Comments *string `json:"comments"`
}

type BorrowReceiveLine struct {
	BorrowLineID int64   `json:"borrow_line_id"`
	QtyBorrowed  float64 `json:"qty_borrowed"`
	ConditionOut *string `json:"condition_out"`
}

type BorrowReceiveRequest struct {
	Lines []BorrowReceiveLine `json:"lines"`
}

type BorrowReturnLine struct {
	BorrowLineID int64   `json:"borrow_line_id"`
	QtyReturned  float64 `json:"qty_returned"`
	ConditionIn  *string `json:"condition_in"`
}

type BorrowReturnRequest struct {
	Lines []BorrowReturnLine `json:"lines"`
}

type BorrowFilter struct {
	Status   string `query:"status"`
	DateFrom string `query:"date_from"`
	DateTo   string `query:"date_to"`
	Search   string `query:"search"`
	Page     int    `query:"page"`
	PageSize int    `query:"page_size"`
}

// ── Reservation ────────────────────────────────────────────

type StockReservation struct {
	ID            int64     `json:"id"`
	ReservationNo string    `json:"reservation_no"`
	Status        string    `json:"status"`
	ItemID        int64     `json:"item_id"`
	MatCode       string    `json:"mat_code"`
	ItemName      string    `json:"item_name"`
	LocationCode  string    `json:"location_code"`
	LocationName  string    `json:"location_name"`
	QtyReserved   float64   `json:"qty_reserved"`
	QtyFulfilled  float64   `json:"qty_fulfilled"`
	RequestedBy   string    `json:"requested_by"`
	NeededBy      *string   `json:"needed_by"`
	Purpose       *string   `json:"purpose"`
	CreatedAt     time.Time `json:"created_at"`
}

type CreateReservationRequest struct {
	ItemID       int64   `json:"item_id"`
	LocationCode string  `json:"location_code"`
	QtyReserved  float64 `json:"qty_reserved"`
	NeededBy     *string `json:"needed_by"`
	Purpose      *string `json:"purpose"`
}

type StockReservationFilter struct {
	Status   string `query:"status"`
	ItemID   int64  `query:"item_id"`
	DateFrom string `query:"date_from"`
	DateTo   string `query:"date_to"`
	Page     int    `query:"page"`
	PageSize int    `query:"page_size"`
}
