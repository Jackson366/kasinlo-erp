package com.kasinlo.erp.domain.settlement;

import com.kasinlo.erp.common.error.ErpError;

public enum SettlementErrorType implements ErpError {
    SETTLEMENT_OVERPAID(40900, 409, "结算金额超出未结余额，审核已一并回滚");

    private final Details details;

    SettlementErrorType(int code, int httpStatus, String template) {
        this.details = new Details(code, httpStatus, template);
    }

    @Override
    public Details details() {
        return details;
    }
}
