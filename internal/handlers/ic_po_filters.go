package handlers

// msgPONotApproved is returned by every IC receive/return write when the PO is not APPROVED
// (DRAFT, PENDING_APPROVAL, REJECTED, CANCELLED, PENDING_REAPPROVAL, ...).
const msgPONotApproved = "ใบสั่งซื้อนี้ยังไม่ได้รับการอนุมัติ ไม่สามารถรับของได้"

// poLinePRNoMatchSQL: some line of the outer PO (alias po) was ordered from a PR whose pr_no matches the
// ILIKE placeholder — use with fmt's %[1]d indexed verb for the placeholder number.
const poLinePRNoMatchSQL = `EXISTS (
			SELECT 1 FROM purchase_order_line lx
			JOIN purchase_request_line rlx ON rlx.id = lx.pr_line_id
			JOIN purchase_request prx ON prx.id = rlx.pr_id
			WHERE lx.po_id = po.id AND prx.pr_no ILIKE $%[1]d)`

// prFullyOrderedSQL (outer alias pr): the PR is fully converted to POs.
//   - at least one live PO is linked (header po.pr_id, or a line's pr_line_id), live = not deleted and
//     status NOT IN (CANCELLED, REJECTED);
//   - NOT EXISTS a non-cancelled line that still needs ordering: need = qty_requested - qty_reserved > 0
//     (the same definition as sqlPRLineRemainingExpr / reconcilePRLineQty; qty_to_order is 0 on legacy PRs)
//     whose live PO-line quantity is below need.
//
// The ordered quantity is summed from the live purchase_order_line rows rather than read from
// purchase_request_line.qty_ordered, because a REJECTED PO keeps its claim in that stored column (reject
// never calls revertPOLineClaims), which would make a PR whose only PO was rejected look fully ordered.
const prFullyOrderedSQL = `(
	EXISTS (
		SELECT 1 FROM purchase_order lp
		WHERE lp.deleted_at IS NULL AND lp.status NOT IN ('CANCELLED','REJECTED')
		  AND (lp.pr_id = pr.id OR EXISTS (
		        SELECT 1 FROM purchase_order_line l1
		        JOIN purchase_request_line r1 ON r1.id = l1.pr_line_id
		        WHERE l1.po_id = lp.id AND r1.pr_id = pr.id)))
	AND NOT EXISTS (
		SELECT 1 FROM purchase_request_line prl
		WHERE prl.pr_id = pr.id AND prl.status <> 'CANCELLED'
		  AND prl.qty_requested - COALESCE(prl.qty_reserved, 0) > 0
		  AND COALESCE((
		        SELECT SUM(pol.qty_ordered)
		        FROM purchase_order_line pol
		        JOIN purchase_order po2 ON po2.id = pol.po_id
		        WHERE pol.pr_line_id = prl.id AND pol.status <> 'CANCELLED'
		          AND po2.deleted_at IS NULL AND po2.status NOT IN ('CANCELLED','REJECTED')), 0)
		      < prl.qty_requested - COALESCE(prl.qty_reserved, 0))
)`
