package handlers

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// claimMemoLines validates and locks the memo_line rows a PR wants to take. It must run inside the
// same transaction that writes purchase_request_line.memo_line_id: the candidate memo_line rows are
// locked FOR UPDATE (ordered by id), so two concurrent PRs can't both take the same line — the
// second one waits, then sees the first one's committed row in the "already taken" check.
// A line counts as taken only by a PR that is not CANCELLED and not soft-deleted.
func claimMemoLines(ctx context.Context, tx pgx.Tx, memoID *int64, selfPRID int64, lineIDs []*int64) error {
	seen := map[int64]bool{}
	ids := make([]int64, 0, len(lineIDs))
	for i, p := range lineIDs {
		if p == nil {
			continue
		}
		if seen[*p] {
			return fiber.NewError(fiber.StatusBadRequest,
				fmt.Sprintf("lines[%d]: รายการ Memo เดียวกันถูกเลือกซ้ำในคำขอเดียว", i))
		}
		seen[*p] = true
		ids = append(ids, *p)
	}
	if len(ids) == 0 {
		return nil
	}
	if memoID == nil {
		return fiber.NewError(fiber.StatusBadRequest, "ไม่สามารถเลือกรายการ Memo ได้ เนื่องจาก PR นี้ไม่ได้อ้างอิง Memo")
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })

	rows, err := tx.Query(ctx, `
		SELECT id, memo_id FROM memo_line WHERE id = ANY($1) ORDER BY id FOR UPDATE`, ids)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบรายการ Memo ได้ กรุณาลองใหม่")
	}
	found := map[int64]bool{}
	for rows.Next() {
		var id, mID int64
		if err := rows.Scan(&id, &mID); err != nil {
			rows.Close()
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบรายการ Memo ได้ กรุณาลองใหม่")
		}
		if mID != *memoID {
			rows.Close()
			return fiber.NewError(fiber.StatusBadRequest,
				fmt.Sprintf("memo_line_id %d ไม่ได้อยู่ใน Memo ของ PR นี้", id))
		}
		found[id] = true
	}
	rows.Close()
	for _, id := range ids {
		if !found[id] {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("ไม่พบรายการ Memo (memo_line_id %d)", id))
		}
	}

	var prNo string
	var lineNo int
	err = tx.QueryRow(ctx, `
		SELECT pr.pr_no, ml.line_no
		FROM purchase_request_line prl
		JOIN purchase_request pr ON pr.id = prl.pr_id
		JOIN memo_line ml        ON ml.id = prl.memo_line_id
		WHERE prl.memo_line_id = ANY($1)
		  AND pr.status <> 'CANCELLED' AND pr.deleted_at IS NULL
		  AND pr.id <> $2
		ORDER BY pr.id LIMIT 1`, ids, selfPRID).Scan(&prNo, &lineNo)
	if err == nil {
		return fiber.NewError(fiber.StatusConflict,
			fmt.Sprintf("รายการ Memo ลำดับที่ %d ถูกใช้ใน PR %s แล้ว", lineNo, prNo))
	}
	if err != pgx.ErrNoRows {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบรายการ Memo ได้ กรุณาลองใหม่")
	}
	return nil
}

// syncMemoLines updates a memo's lines in place so memo_line ids stay stable (PR lines reference
// them). Incoming lines match existing rows by id when the payload carries one, otherwise by
// line_no. Matched rows are UPDATEd, unmatched incoming lines INSERTed, and existing rows absent
// from the payload DELETEd — except that deleting a line used by an active PR (not CANCELLED, not
// soft-deleted) is a 409. References from CANCELLED/soft-deleted PRs are cleared first.
func syncMemoLines(ctx context.Context, tx pgx.Tx, memoID int64, lines []models.MemoLineRequest) error {
	type existing struct {
		id     int64
		lineNo int
	}
	rows, err := tx.Query(ctx, `SELECT id, line_no FROM public.memo_line WHERE memo_id=$1 ORDER BY id FOR UPDATE`, memoID)
	if err != nil {
		return err
	}
	byID := map[int64]existing{}
	byNo := map[int]existing{}
	for rows.Next() {
		var e existing
		if err := rows.Scan(&e.id, &e.lineNo); err != nil {
			rows.Close()
			return err
		}
		byID[e.id] = e
		byNo[e.lineNo] = e
	}
	rows.Close()

	// 1. Payload sanity: positive, unique line_no; ids must belong to this memo and be used once.
	usedNo := map[int]bool{}
	claimedID := map[int64]bool{}
	for i, l := range lines {
		if l.LineNo <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: line_no must be > 0", i))
		}
		if usedNo[l.LineNo] {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: line_no %d ซ้ำกัน", i, l.LineNo))
		}
		usedNo[l.LineNo] = true
		if l.ID != nil {
			if _, ok := byID[*l.ID]; !ok {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: ไม่พบรายการ Memo id %d ใน Memo นี้", i, *l.ID))
			}
			if claimedID[*l.ID] {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: id %d ถูกใช้ซ้ำในคำขอเดียว", i, *l.ID))
			}
			claimedID[*l.ID] = true
		}
	}

	// 2. Resolve each incoming line to an existing id (nil = new). Lines without an id fall back
	// to line_no, but never steal a row another incoming line claimed by id.
	matched := make([]*int64, len(lines))
	for i, l := range lines {
		if l.ID != nil {
			id := *l.ID
			matched[i] = &id
			continue
		}
		if e, ok := byNo[l.LineNo]; ok && !claimedID[e.id] {
			id := e.id
			claimedID[id] = true
			matched[i] = &id
		}
	}

	// 3. Removed = existing rows nobody claimed.
	var removed []int64
	for id := range byID {
		if !claimedID[id] {
			removed = append(removed, id)
		}
	}
	sort.Slice(removed, func(a, b int) bool { return removed[a] < removed[b] })
	if len(removed) > 0 {
		var prNo string
		var lineNo int
		err := tx.QueryRow(ctx, `
			SELECT pr.pr_no, ml.line_no
			FROM purchase_request_line prl
			JOIN purchase_request pr ON pr.id = prl.pr_id
			JOIN memo_line ml        ON ml.id = prl.memo_line_id
			WHERE prl.memo_line_id = ANY($1)
			  AND pr.status <> 'CANCELLED' AND pr.deleted_at IS NULL
			ORDER BY ml.line_no, pr.id LIMIT 1`, removed).Scan(&prNo, &lineNo)
		if err == nil {
			return fiber.NewError(fiber.StatusConflict,
				fmt.Sprintf("รายการที่ %d ถูกใช้ใน PR %s ไม่สามารถลบได้", lineNo, prNo))
		}
		if err != pgx.ErrNoRows {
			return err
		}
		// Only CANCELLED / soft-deleted PRs still point here: release them, then delete.
		if _, err := tx.Exec(ctx,
			`UPDATE purchase_request_line SET memo_line_id = NULL WHERE memo_line_id = ANY($1)`, removed); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM public.memo_line WHERE id = ANY($1)`, removed); err != nil {
			return err
		}
	}

	// 4. Park matched rows on temporary line_no values so reorders/swaps can't trip
	// UNIQUE (memo_id, line_no) mid-way, then write final values.
	var matchedIDs []int64
	for _, m := range matched {
		if m != nil {
			matchedIDs = append(matchedIDs, *m)
		}
	}
	if len(matchedIDs) > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE public.memo_line SET line_no = line_no + 1000000 WHERE id = ANY($1)`, matchedIDs); err != nil {
			return err
		}
	}
	for i, l := range lines {
		if matched[i] != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE public.memo_line
				SET line_no=$1, description=$2, unit=$3, quantity=$4, remark=$5
				WHERE id=$6 AND memo_id=$7`,
				l.LineNo, l.Description, l.Unit, l.Quantity, l.Remark, *matched[i], memoID); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO public.memo_line (memo_id, line_no, description, unit, quantity, remark)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			memoID, l.LineNo, l.Description, l.Unit, l.Quantity, l.Remark); err != nil {
			return err
		}
	}
	return nil
}

// AvailableLines godoc
// @Summary      List a Memo's lines with their "taken by PR" state
// @Description  Every line of the Memo plus taken / taken_by_pr_id / taken_by_pr_no. A line is taken when a purchase_request_line references it (memo_line_id) on a PR that is not CANCELLED and not soft-deleted. exclude_pr_id (optional) makes lines taken by that PR report as not taken — used when editing that PR.
// @Tags         Memo
// @Security     BearerAuth
// @Produce      json
// @Param        id            path   int  true   "Memo ID"
// @Param        exclude_pr_id query  int  false  "Treat lines taken by this PR as free (PR edit screen)"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /memo/{id}/available-lines [get]
func (h *MemoHandler) AvailableLines(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var excludePR *int64
	if v := strings.TrimSpace(c.Query("exclude_pr_id")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, "exclude_pr_id must be a positive number")
		}
		excludePR = &n
	}
	ctx := context.Background()

	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.memo WHERE id=$1)`, id).Scan(&exists); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการ Memo ได้ กรุณาลองใหม่")
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "memo not found")
	}

	rows, err := h.db.Query(ctx, `
		SELECT ml.id, ml.line_no, ml.description, ml.unit, ml.quantity, ml.remark, t.pr_id, t.pr_no
		FROM public.memo_line ml
		LEFT JOIN LATERAL (
			SELECT pr.id AS pr_id, pr.pr_no
			FROM purchase_request_line prl
			JOIN purchase_request pr ON pr.id = prl.pr_id
			WHERE prl.memo_line_id = ml.id
			  AND pr.status <> 'CANCELLED' AND pr.deleted_at IS NULL
			  AND ($2::bigint IS NULL OR pr.id <> $2)
			ORDER BY pr.id LIMIT 1
		) t ON TRUE
		WHERE ml.memo_id = $1
		ORDER BY ml.line_no`, id, excludePR)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการ Memo ได้ กรุณาลองใหม่")
	}
	defer rows.Close()

	type lineOut struct {
		ID          int64   `json:"id"`
		LineNo      int     `json:"line_no"`
		Description string  `json:"description"`
		Unit        string  `json:"unit"`
		Quantity    float64 `json:"quantity"`
		Remark      *string `json:"remark"`
		Taken       bool    `json:"taken"`
		TakenByPRID *int64  `json:"taken_by_pr_id"`
		TakenByPRNo *string `json:"taken_by_pr_no"`
	}
	out := []lineOut{}
	for rows.Next() {
		var l lineOut
		if err := rows.Scan(&l.ID, &l.LineNo, &l.Description, &l.Unit, &l.Quantity, &l.Remark, &l.TakenByPRID, &l.TakenByPRNo); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถอ่านรายการ Memo ได้ กรุณาลองใหม่")
		}
		l.Taken = l.TakenByPRID != nil
		out = append(out, l)
	}
	return c.JSON(fiber.Map{"success": true, "data": out})
}
