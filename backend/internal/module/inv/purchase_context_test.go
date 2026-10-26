package inv

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"nova-erp/internal/pkg/page"
)

func TestFinanceOrderFilterSharedBySummaryAndRows(t *testing.T) {
	s := &Service{}
	where, args := s.financeWhere(nil, page.Query{}, "", time.Now(), 42)
	if !strings.Contains(where, "t.source = 'order' AND t.order_id = ?") || len(args) == 0 || args[0] != int64(42) {
		t.Fatalf("missing exact source filter: %s %v", where, args)
	}
	where, _ = s.financeWhere(nil, page.Query{}, "", time.Now())
	if strings.Contains(where, "t.order_id =") {
		t.Fatal("ordinary lists must not be filtered by order")
	}
}

func TestRelatedStockRejectsInvalidSourceBeforeQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{"sourceId=-1&sourceType=purchase", "sourceId=1&sourceType=other", "sourceId=oops&sourceType=purchase"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/inv/stock-orders?"+query, nil)
		(&Handler{}).StockOrderList(c)
		if w.Code != 400 {
			t.Fatalf("%s: got %d", query, w.Code)
		}
	}
}
