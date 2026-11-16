package com.kasinlo.erp.infrastructure.auth;

import com.kasinlo.erp.common.enums.EnablementStatus;
import com.kasinlo.erp.domain.auth.InventoryAccess;
import com.kasinlo.erp.domain.auth.InventoryAccessConstants;
import com.kasinlo.erp.domain.auth.InventoryAccessStatus;
import org.springframework.stereotype.Repository;

@Repository
public class MyBatisInventoryAccess implements InventoryAccess {
    private final InventoryAccessMapper mapper;

    public MyBatisInventoryAccess(InventoryAccessMapper mapper) {
        this.mapper = mapper;
    }

    @Override
    public InventoryAccessStatus check(long userId, long tokenEpoch) {
        int enabled = EnablementStatus.ENABLED.getCode();
        var user = mapper.find(userId, enabled, InventoryAccessConstants.STOCK_LIST_PERMISSION);
        if (user == null) {
            return InventoryAccessStatus.USER_MISSING;
        }
        if (user.status() != enabled) {
            return InventoryAccessStatus.ACCOUNT_DISABLED;
        }
        if (user.tokenEpoch() != tokenEpoch) {
            return InventoryAccessStatus.TOKEN_REVOKED;
        }
        return user.allowed() ? InventoryAccessStatus.ALLOWED : InventoryAccessStatus.FORBIDDEN;
    }
}
