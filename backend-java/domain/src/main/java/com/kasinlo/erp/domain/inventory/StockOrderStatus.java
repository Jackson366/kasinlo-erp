package com.kasinlo.erp.domain.inventory;

import com.kasinlo.erp.common.enums.CodeEnum;

public enum StockOrderStatus implements CodeEnum<Integer> {
    POSTED(1, "已过账"),
    VOIDED(2, "已作废");

    private final Integer code;
    private final String description;

    StockOrderStatus(Integer code, String description) {
        this.code = code;
        this.description = description;
    }

    @Override
    public Integer getCode() {
        return code;
    }

    @Override
    public String getDescription() {
        return description;
    }
}
