package com.kasinlo.erp.domain.settlement;

import com.kasinlo.erp.common.enums.CodeEnum;

public enum SettlementSourceType implements CodeEnum<String> {
    ORDER("order", "单据结算"),
    PLAN("plan", "合同期次收付");

    private final String code;
    private final String description;

    SettlementSourceType(String code, String description) {
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
