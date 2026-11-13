package handlers

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Who sees prices (unit cost) on the Stock pages (warehouse and project balances, warehouse item list): users of the Supply Chain or Finance department (by their own
// department or by a role of that department), the Board roles and ADMIN_CENTER. Everyone else gets the balance
// without prices: the API sends unit_cost = null and show_price = false, so the price is never in the response.
const stockPriceSQL = `SELECT EXISTS (
	SELECT 1 FROM users u WHERE u.id = $1 AND u.dept_code IN ('SC', 'FN')
	UNION ALL
	SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
	 WHERE ur.user_id = $1
	   AND (r.dept_code IN ('SC', 'FN') OR r.role_code IN ('ADMIN_CENTER', 'ROLE_B', 'ROLE_D', 'ROLE_E', 'ROLE_MD')))`

// canViewStockPrice reports whether the caller may see stock prices. An unknown caller or a lookup failure
// means "no" (the safe answer).
func canViewStockPrice(c *fiber.Ctx, db *pgxpool.Pool) bool {
	uid := searchUserID(c)
	if uid == 0 {
		return false
	}
	var ok bool
	if err := db.QueryRow(context.Background(), stockPriceSQL, uid).Scan(&ok); err != nil {
		log.Printf("stock price access: %v", err)
		return false
	}
	return ok
}

// maskStockPrices drops the unit cost of every row (used when the caller may not see prices).
func maskStockPrices(rows []stockBalanceRow) {
	for i := range rows {
		rows[i].UnitCost = nil
	}
}
