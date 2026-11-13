package oauth

import (
	"slices"
	"strings"
)

const (
	Scope        = "inventory.read"
	ProductRead  = "product.read"
	PurchaseRead = "purchase.read"
	SalesRead    = "sales.read"
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
		case Scope, ProductRead, PurchaseRead, SalesRead:
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
