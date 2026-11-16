package com.kasinlo.erp.domain.inventory;

import com.kasinlo.erp.common.error.ErpError;

public enum InventoryErrorType implements ErpError {
    STOCK_SPEC_REQUIRED(40000, 400, "出入库明细缺少规格，无法过账"),
    STOCK_SPEC_REMOVED(40900, 409, "「%s」商品规格已不存在，无法更新库存"),
    STOCK_INSUFFICIENT(40900, 409, "「%s」库存不足（现有 %s）"),
    STOCK_ORDER_NUMBER_CONFLICT(40900, 409, "编号已被占用，请重试");

    private final Details details;

    InventoryErrorType(int code, int httpStatus, String template) {
        this.details = new Details(code, httpStatus, template);
    }

    @Override
    public Details details() {
        return details;
    }
}
