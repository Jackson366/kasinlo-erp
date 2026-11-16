package com.kasinlo.erp.app.inventory;

import com.kasinlo.erp.client.InventoryQuery.Row;

import java.util.List;

public interface InventoryQueryGateway {
    long count(String pattern);

    List<Row> list(String pattern, int limit, long offset);
}
