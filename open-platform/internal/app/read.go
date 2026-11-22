package app

import (
	"log/slog"
	"net/http"

	"kasinlo-open-platform/internal/erpclient"
	"kasinlo-open-platform/internal/oauth"
)

type readResource struct {
	path             string
	scope            string
	list             bool
	unpaginated      bool
	detail           bool
	filters          []string
	sourceScopes     map[string]string
	projectInventory bool
}

var (
	partyFilters   = []string{"status"}
	orderFilters   = []string{"status", "settleStatus", "startDate", "endDate"}
	financeFilters = []string{"settle", "source"}

	contractFilters = []string{"bizType", "status", "stage", "partyId", "startDate", "endDate"}

	readResources = []readResource{
		{path: "contracts", scope: oauth.ContractRead, list: true, detail: true, filters: contractFilters},
		{path: "contract-report", scope: oauth.ContractReportRead, list: true, unpaginated: true, filters: contractFilters},
		{path: "contract-activities", scope: oauth.ContractHistoryRead, detail: true},
		{path: "contract-plan-settlements", scope: oauth.ContractHistoryRead, detail: true},
		{path: "inventory", scope: oauth.Scope, list: true, projectInventory: true},
		{path: "product-categories", scope: oauth.ProductRead, list: true, detail: true, filters: []string{"status", "parentId"}},
		{path: "products", scope: oauth.ProductRead, list: true, detail: true, filters: []string{"status", "categoryId"}},
		{path: "customers", scope: oauth.CustomerRead, list: true, detail: true, filters: partyFilters},
		{path: "suppliers", scope: oauth.SupplierRead, list: true, detail: true, filters: partyFilters},
		{path: "purchase-orders", scope: oauth.PurchaseRead, list: true, detail: true, filters: orderFilters},
		{path: "sales-orders", scope: oauth.SalesRead, list: true, detail: true, filters: orderFilters},
		{
			path: "stock-orders", scope: oauth.StockRead, list: true, detail: true,
			filters:      []string{"direction", "includeVoided", "sourceType", "sourceId"},
			sourceScopes: map[string]string{"purchase": oauth.PurchaseRead, "sales": oauth.SalesRead},
		},
		{path: "receivables", scope: oauth.FinanceRead, list: true, filters: financeFilters},
		{path: "payables", scope: oauth.FinanceRead, list: true, filters: financeFilters},
		{path: "order-settlements", scope: oauth.FinanceRead, detail: true},
	}
)

func (s Server) registerReads(mux *http.ServeMux) {
	for _, entry := range readResources {
		pattern := "GET /openapi/v1/" + entry.path
		if entry.list {
			resource := erpclient.Resource{
				Path: entry.path, Scope: entry.scope, Filters: entry.filters, ProjectInventory: entry.projectInventory, Unpaginated: entry.unpaginated,
			}
			mux.HandleFunc(pattern, s.read(resource, entry.sourceScopes))
		}
		if entry.detail {
			resource := erpclient.Resource{Path: entry.path, Scope: entry.scope, Detail: true}
			mux.HandleFunc(pattern+"/{id}", s.read(resource, nil))
		}
	}
}

func (s Server) read(resource erpclient.Resource, sourceScopes map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		grant, ok := s.activeGrant(w, r)
		if !ok {
			return
		}
		query := r.URL.Query()
		sourceScope := sourceScopes[query.Get("sourceType")]
		if !oauth.HasScope(grant.Scope, resource.Scope) ||
			(sourceScope != "" && !oauth.HasScope(grant.Scope, sourceScope)) {
			reject(w, http.StatusForbidden, "insufficient_scope")
			return
		}

		data, status, err := s.ERP.Read(r.Context(), grant, resource, r.PathValue("id"), query)
		if err != nil {
			slog.Warn("ERP read failed", "resource", resource.Path, "grant_id", grant.ID)
			reject(w, http.StatusBadGateway, "erp_unavailable")
			return
		}
		slog.Info("ERP read requested", "resource", resource.Path, "grant_id", grant.ID, "user_id", grant.UserID, "status", status)
		switch status {
		case http.StatusUnauthorized:
			reject(w, status, "erp_identity_invalid")
		case http.StatusForbidden:
			reject(w, status, "insufficient_permission")
		case http.StatusBadRequest:
			reject(w, status, "invalid_request")
		case http.StatusNotFound:
			reject(w, status, "not_found")
		default:
			writeJSON(w, http.StatusOK, data)
		}
	}
}
