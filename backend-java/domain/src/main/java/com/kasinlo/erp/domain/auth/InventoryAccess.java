package com.kasinlo.erp.domain.auth;

public interface InventoryAccess {
    InventoryAccessStatus check(long userId, long tokenEpoch);
}
