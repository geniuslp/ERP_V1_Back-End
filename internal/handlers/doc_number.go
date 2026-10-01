package handlers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// Save-time document numbering for PR / PO / Memo. The number is allocated inside the same
// transaction as the header INSERT via a monthly-reset counter row (UPSERT ... RETURNING). The
// counter row stays locked until commit, so concurrent creates serialize (no duplicates) and a
// rolled-back create gives its number back (no new gaps). Clients never send or reserve numbers.

// monthlyCounterTables is the whitelist of counter tables nextMonthlyNumber may touch; the table
// name is interpolated into SQL, so it must never come from caller input.
var monthlyCounterTables = map[string]bool{
	"pr_number_counter":   true,
	"po_number_counter":   true,
	"memo_number_counter": true,
}

var (
	bangkokLocOnce sync.Once
	bangkokLoc     *time.Location
)

// bangkokNow returns the current time in Asia/Bangkok (fixed +07:00 if tzdata is missing), so
// year_month and the YYMM/YYYYMM text never depend on the server's local timezone.
func bangkokNow() time.Time {
	bangkokLocOnce.Do(func() {
		loc, err := time.LoadLocation("Asia/Bangkok")
		if err != nil {
			loc = time.FixedZone("ICT", 7*60*60)
		}
		bangkokLoc = loc
	})
	return time.Now().In(bangkokLoc)
}

// nextMonthlyNumber increments the counter row for now's year_month (YYYYMM) on tx and returns the
// new sequence. Must be called on the document's own create transaction.
func nextMonthlyNumber(ctx context.Context, tx pgx.Tx, counterTable string, now time.Time) (int64, error) {
	if !monthlyCounterTables[counterTable] {
		return 0, fmt.Errorf("nextMonthlyNumber: counter table %q is not allowed", counterTable)
	}
	var seq int64
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s (year_month, last_seq) VALUES ($1, 1)
		ON CONFLICT (year_month) DO UPDATE SET last_seq = %s.last_seq + 1
		RETURNING last_seq`, counterTable, counterTable), now.Format("200601"),
	).Scan(&seq)
	return seq, err
}

// Existing formats, unchanged: PRYYYYMM-NNNN, PO-YYYYMM-NNNN, MEM-YYMM-NNNN.
func formatPRNo(now time.Time, seq int64) string   { return fmt.Sprintf("PR%s-%04d", now.Format("200601"), seq) }
func formatPONo(now time.Time, seq int64) string   { return fmt.Sprintf("PO-%s-%04d", now.Format("200601"), seq) }
func formatMemoNo(now time.Time, seq int64) string { return fmt.Sprintf("MEM-%s-%04d", now.Format("0601"), seq) }
