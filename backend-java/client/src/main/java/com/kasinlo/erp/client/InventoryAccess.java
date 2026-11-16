package com.kasinlo.erp.client;

public interface InventoryAccess {
    Decision check(long userId, long tokenEpoch);

    enum Decision {
        ALLOWED, USER_MISSING, ACCOUNT_DISABLED, TOKEN_REVOKED, FORBIDDEN
    }
}
