package com.kasinlo.erp.domain.inventory;

import com.kasinlo.erp.common.enums.CodeEnum;

public enum StockMovementType implements CodeEnum<String> {
    IN("in", "入库"),
    OUT("out", "出库");

    private final String code;
    private final String description;

    StockMovementType(String code, String description) {
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
