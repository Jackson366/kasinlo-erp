package inv

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"nova-erp/internal/pkg/page"
)

func TestStockHistoryQueryAndExport(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		history bool
		invalid bool
	}{
		{"", false, false}, {"false", false, false}, {"true", true, false}, {"wrong", false, true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/inv/stock-orders?includeVoided="+tc.raw, nil)
			got, err := stockHistoryQuery(c)
			if (err != nil) != tc.invalid || got != tc.history {
				t.Fatalf("history=%v err=%v", got, err)
			}
			exported, exportErr := exportQuery(c, exportStock, page.Query{})
			if (exportErr != nil) != tc.invalid || exported.IncludeVoided != tc.history {
				t.Fatalf("export history=%v err=%v", exported.IncludeVoided, exportErr)
			}
		})
	}
}
