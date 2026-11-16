package com.kasinlo.erp.common.error;

public enum CommonErrorType implements ErpError {
    RESOURCE_NOT_FOUND(40400, 404, "资源不存在"),
    LOGIN_REQUIRED(40100, 401, "登录状态已失效，请重新登录"),
    USER_NOT_FOUND(40100, 401, "用户不存在"),
    ACCOUNT_DISABLED(40102, 401, "账号已被禁用，请联系管理员"),
    INVALID_TOKEN(40105, 401, "身份凭证无效"),
    FORBIDDEN(40300, 403, "没有操作权限"),
    PERMISSION_REQUIRED(40300, 403, "缺少权限: %s"),
    DATABASE_FAILURE(50100, 500, "数据库操作失败");

    private final Details details;

    CommonErrorType(int code, int httpStatus, String template) {
        this.details = new Details(code, httpStatus, template);
    }

    @Override
    public Details details() {
        return details;
    }
}
