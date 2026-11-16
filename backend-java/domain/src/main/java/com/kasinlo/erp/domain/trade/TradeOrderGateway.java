package com.kasinlo.erp.domain.trade;

import java.time.OffsetDateTime;
import java.util.List;

public interface TradeOrderGateway {
    TradeOrder lock(long orderId);

    List<TradeOrder.Item> lines(long orderId);

    boolean approve(long orderId, long userId, OffsetDateTime at);

    void recordApproval(long orderId, String orderNo, long userId, String actorName,
                        long stockOrderId, OffsetDateTime at);
}
