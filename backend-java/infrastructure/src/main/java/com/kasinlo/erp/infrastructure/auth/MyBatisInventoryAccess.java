package com.kasinlo.erp.infrastructure.auth;

import com.kasinlo.erp.client.InventoryAccess;
import org.springframework.stereotype.Repository;

@Repository
public class MyBatisInventoryAccess implements InventoryAccess {
    private final InventoryAccessMapper mapper;

    public MyBatisInventoryAccess(InventoryAccessMapper mapper) {
        this.mapper = mapper;
    }

    @Override
    public Decision check(long userId, long tokenEpoch) {
        var user = mapper.find(userId);
        if (user == null) {
            return Decision.USER_MISSING;
        }
        if (user.status() != 1) {
            return Decision.ACCOUNT_DISABLED;
        }
        if (user.tokenEpoch() != tokenEpoch) {
            return Decision.TOKEN_REVOKED;
        }
        return user.allowed() ? Decision.ALLOWED : Decision.FORBIDDEN;
    }
}
