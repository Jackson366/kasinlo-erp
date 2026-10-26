package com.kasinlo.erp.web;

import static org.junit.jupiter.api.Assertions.assertEquals;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.Test;

class RelatedPurchaseFilterTest {
    @Test
    void matchesSourceIdAndFinanceOrderIdExactly() {
        List<Map<String, Object>> stocks = List.of(
            Map.of("sourceType", "purchase", "sourceId", 12L),
            Map.of("sourceType", "purchase", "sourceId", 123L),
            Map.of("sourceType", "sales", "sourceId", 12L));
        assertEquals(1, ApiController.filter(stocks, Map.of("sourceType", "purchase", "sourceId", "12")).size());
        List<Map<String, Object>> finance = List.of(Map.of("orderId", 12L), Map.of("orderId", 123L));
        assertEquals(1, ApiController.filter(finance, Map.of("orderId", "12")).size());
    }
}
