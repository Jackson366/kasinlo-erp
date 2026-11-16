package com.kasinlo.erp.infrastructure.settlement;

import com.kasinlo.erp.domain.settlement.SettlementGateway;
import org.springframework.stereotype.Repository;

import java.math.BigDecimal;
import java.time.OffsetDateTime;

@Repository
public class MyBatisSettlementGateway implements SettlementGateway {
    private final SettlementMapper mapper;

    public MyBatisSettlementGateway(SettlementMapper mapper) {
        this.mapper = mapper;
    }

    @Override
    public boolean payOnApproval(long orderId, String bizType, long partyId, BigDecimal amount,
                                 long userId, OffsetDateTime at) {
        mapper.record(orderId, bizType, partyId, amount, userId, at.toLocalDate(), at);
        return mapper.settle(orderId, amount);
    }
}
