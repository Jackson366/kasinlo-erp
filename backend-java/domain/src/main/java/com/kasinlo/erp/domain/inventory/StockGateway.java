package com.kasinlo.erp.domain.inventory;

import java.math.BigDecimal;
import java.time.OffsetDateTime;

public interface StockGateway {
    long save(StockPosting posting, OffsetDateTime at);

    boolean changeStock(long specId, BigDecimal delta);

    BigDecimal stock(long specId);
}
