package com.kasinlo.erp.client;

import com.kasinlo.erp.common.result.PageResult;

import java.math.BigDecimal;

public interface InventoryQuery {
    PageResult<Row> list(String keyword, long page, long pageSize);

    record Row(long productId, String productCode, String productName, long specId,
               String spec, String unit, BigDecimal stock) {}
}
