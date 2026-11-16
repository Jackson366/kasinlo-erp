package com.kasinlo.erp.domain.trade;

import com.kasinlo.erp.common.enums.EnumUtils;
import com.kasinlo.erp.common.error.ErpException;

import java.math.BigDecimal;
import java.time.LocalDate;

public record TradeOrder(long id, String orderNo, String bizType, long partyId, LocalDate orderDate,
                         int status, BigDecimal totalAmount, BigDecimal settledAmount, String salesman) {
    public TradeOrderType type() {
        return EnumUtils.getByCode(TradeOrderType.class, bizType)
                .orElseThrow(() -> new ErpException(TradeErrorType.ORDER_TYPE_INVALID));
    }

    public void checkApprovable() {
        if (status == TradeOrderStatus.APPROVED.getCode()) {
            throw new ErpException(TradeErrorType.ORDER_ALREADY_APPROVED);
        }
        if (status != TradeOrderStatus.PENDING.getCode()) {
            throw new ErpException(TradeErrorType.ORDER_NOT_PENDING);
        }
    }

    public record Item(long productId, String productName, long specId, String spec, long unitId,
                       String unit, BigDecimal rate, BigDecimal qty, BigDecimal qtyBase) {}
}
