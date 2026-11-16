package com.kasinlo.erp.domain.trade;

import com.alibaba.cola.exception.BizException;

import java.math.BigDecimal;
import java.time.LocalDate;

public record TradeOrder(long id, String orderNo, String bizType, long partyId, LocalDate orderDate,
                         int status, BigDecimal totalAmount, BigDecimal settledAmount, String salesman) {
    public static final int PENDING = 1;
    public static final int APPROVED = 2;

    public void checkApprovable() {
        if (status == APPROVED) {
            throw new BizException("40900", "已审核单据不可再流转状态");
        }
        if (status != PENDING) {
            throw new BizException("40900", "只有待处理的单据才能审核");
        }
    }

    public record Item(long productId, String productName, long specId, String spec, long unitId,
                       String unit, BigDecimal rate, BigDecimal qty, BigDecimal qtyBase) {}
}
