package com.kasinlo.erp.domain.settlement;

import com.kasinlo.erp.common.error.ErpException;

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
            throw new ErpException(SettlementErrorType.SETTLEMENT_OVERPAID);
        }
        return Optional.of(amount);
    }
}
