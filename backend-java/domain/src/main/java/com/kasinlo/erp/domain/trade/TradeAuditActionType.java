package com.kasinlo.erp.domain.trade;

import com.kasinlo.erp.common.enums.CodeEnum;

public enum TradeAuditActionType implements CodeEnum<String> {
    AUDIT("audit", "审核"),
    REVERSE("reverse", "红冲");

    private final String code;
    private final String description;

    TradeAuditActionType(String code, String description) {
        this.code = code;
        this.description = description;
    }

    @Override
    public String getCode() {
        return code;
    }

    @Override
    public String getDescription() {
        return description;
    }
}
