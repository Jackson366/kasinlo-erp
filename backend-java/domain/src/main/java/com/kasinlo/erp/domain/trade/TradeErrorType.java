package com.kasinlo.erp.domain.trade;

import com.kasinlo.erp.common.error.ErpError;

public enum TradeErrorType implements ErpError {
    ORDER_NOT_FOUND(40400, "单据不存在"),
    ORDER_TYPE_INVALID(40000, "单据业务类型非法"),
    ORDER_LINES_EMPTY(40000, "单据明细为空，无法过账"),
    ORDER_ALREADY_APPROVED(40900, "已审核单据不可再流转状态"),
    ORDER_NOT_PENDING(40900, "只有待处理的单据才能审核"),
    ORDER_APPROVAL_CONFLICT(40900, "单据已被审核，请勿重复操作");

    private final Details details;

    TradeErrorType(int code, String template) {
        this.details = new Details(code, template);
    }

    @Override
    public Details details() {
        return details;
    }
}
