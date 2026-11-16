package com.kasinlo.erp.app.inventory;

import com.kasinlo.erp.client.InventoryQuery;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

@Service
public class QueryInventory implements InventoryQuery {
    private final InventoryQueryGateway inventory;

    public QueryInventory(InventoryQueryGateway inventory) {
        this.inventory = inventory;
    }

    @Override
    @Transactional(readOnly = true)
    public Result list(String keyword, long page, long pageSize) {
        int limit = pageSize < 1 ? 10 : (int) Math.min(pageSize, 200);
        long current = Math.clamp(page, 1, 1_000_000L / limit + 1);
        String pattern = keyword == null || keyword.isEmpty() ? null
                : "%" + keyword.replace("\\", "\\\\").replace("%", "\\%").replace("_", "\\_") + "%";
        long total = inventory.count(pattern);
        long offset = (current - 1) * limit;
        return new Result(offset >= total ? List.of() : inventory.list(pattern, limit, offset), total, current, limit);
    }
}
