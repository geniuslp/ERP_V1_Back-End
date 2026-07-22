package handlers

// Stock transaction types — the single source of truth for stock_transaction.txn_type,
// shared by every handler that writes stock_transaction rows (e.g. stock_transaction.go's
// generic Create endpoint, pr.go's deductStockOnSubmit). Kept as plain Go constants rather
// than a DB CHECK constraint so new types can be added by a code change alone, without a
// migration.
const (
	TxnTypeReceive      = "RECEIVE"       // stock received in (e.g. from PO/GRN)
	TxnTypeIssue        = "ISSUE"         // stock issued/consumed out (normal usage, not a correction)
	TxnTypeReturn       = "RETURN"        // stock returned in
	TxnTypeAdjustPlus   = "ADJUST_PLUS"   // manual correction, increases qty
	TxnTypeAdjustMinus  = "ADJUST_MINUS"  // manual correction, decreases qty
	TxnTypeTransfer     = "TRANSFER"      // moved between warehouses
	TxnTypeBorrowOut    = "BORROW_OUT"    // borrowed out
	TxnTypeBorrowReturn = "BORROW_RETURN" // borrowed item returned
)
