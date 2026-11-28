package oauth

import (
	"slices"
	"strings"
)

const (
	Scope                = "inventory.read"
	ProductRead          = "product.read"
	CustomerRead         = "customer.read"
	SupplierRead         = "supplier.read"
	PurchaseRead         = "purchase.read"
	StockRead            = "stock.read"
	BusinessReportRead   = "business.report.read"
	BusinessTrendRead    = "business.trend.read"
	InventoryRankingRead = "inventory.ranking.read"
	InventoryReportRead  = "inventory.report.read"
	FinanceRead          = "finance.read"
	SalesRead            = "sales.read"
	ContractRead         = "contract.read"
	ContractLedgerRead   = "contract.ledger.read"
	ContractReportRead   = "contract.report.read"
	ContractHistoryRead  = "contract.history.read"
)

// NormalizeScope canonicalizes a space-separated request so the exact approved
// scope can be bound to a pending device request and its resulting grant.
func NormalizeScope(value string) (string, bool) {
	if value == "" {
		return Scope, true
	}
	var scopes []string
	for _, scope := range strings.Split(value, " ") {
		switch scope {
		case Scope, BusinessReportRead, BusinessTrendRead, InventoryRankingRead, InventoryReportRead, ProductRead, CustomerRead, SupplierRead, PurchaseRead, SalesRead, StockRead, FinanceRead, ContractRead, ContractHistoryRead, ContractReportRead, ContractLedgerRead:
			if !slices.Contains(scopes, scope) {
				scopes = append(scopes, scope)
			}
		default:
			return "", false
		}
	}
	return strings.Join(scopes, " "), true
}

func HasScope(value, scope string) bool {
	return slices.Contains(strings.Split(value, " "), scope)
}
