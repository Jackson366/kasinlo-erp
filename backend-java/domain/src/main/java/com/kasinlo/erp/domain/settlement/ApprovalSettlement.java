package com.kasinlo.erp.domain.settlement;

import com.alibaba.cola.exception.BizException;

import java.math.BigDecimal;
import java.math.RoundingMode;
import java.util.Optional;

public final class ApprovalSettlement {
    private ApprovalSettlement() {}

    public static Optional<BigDecimal> fullPayment(boolean settleNow, BigDecimal total, BigDecimal settled) {
        if (!settleNow || total.signum() <= 0) {
            return Optional.empty();
        }
        BigDecimal amount = total.setScale(2, RoundingMode.HALF_UP);
        if (settled.add(amount).compareTo(total) > 0) {
            throw overpaid();
        }
        return Optional.of(amount);
    }

    public static BizException overpaid() {
        return new BizException("40900", "结算金额超出未结余额，审核已一并回滚");
    }
}
