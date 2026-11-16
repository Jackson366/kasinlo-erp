package com.kasinlo.erp.domain.settlement;

import java.math.BigDecimal;
import java.time.OffsetDateTime;

public interface SettlementGateway {
    boolean payOnApproval(long orderId, String bizType, long partyId, BigDecimal amount,
                          long userId, OffsetDateTime at);
}
