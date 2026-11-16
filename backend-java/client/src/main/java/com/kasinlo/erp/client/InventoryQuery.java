package com.kasinlo.erp.client;

import java.math.BigDecimal;
import java.util.List;

public interface InventoryQuery {
    Result list(String keyword, long page, long pageSize);

    record Row(long productId, String productCode, String productName, long specId,
               String spec, String unit, BigDecimal stock) {}

    record Result(List<Row> list, long total, long page, int pageSize) {}
}
