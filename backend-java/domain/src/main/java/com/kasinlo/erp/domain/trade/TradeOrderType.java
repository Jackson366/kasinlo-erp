package com.kasinlo.erp.domain.trade;

import com.kasinlo.erp.common.enums.CodeEnum;

public enum TradeOrderType implements CodeEnum<String> {
    PURCHASE("purchase", "采购"),
    SALES("sales", "销售");

    private final String code;
    private final String description;

    TradeOrderType(String code, String description) {
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
