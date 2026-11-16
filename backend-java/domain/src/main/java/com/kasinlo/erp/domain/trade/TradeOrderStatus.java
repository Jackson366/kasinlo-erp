package com.kasinlo.erp.domain.trade;

import com.kasinlo.erp.common.enums.CodeEnum;

public enum TradeOrderStatus implements CodeEnum<Integer> {
    PENDING(1, "待处理"),
    APPROVED(2, "已审核"),
    CANCELED(3, "已取消"),
    RED_REVERSED(4, "已红冲"),
    RED_REVERSAL(5, "红冲单");

    private final Integer code;
    private final String description;

    TradeOrderStatus(Integer code, String description) {
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
