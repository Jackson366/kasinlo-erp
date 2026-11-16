package com.kasinlo.erp.common.error;

public enum CommonErrorType implements ErpError {
    INVALID_ARGUMENT(40000, "请求参数不合法"),
    RESOURCE_NOT_FOUND(40400, "资源不存在"),
    LOGIN_REQUIRED(40100, "登录状态已失效，请重新登录"),
    USER_NOT_FOUND(40100, "用户不存在"),
    ACCOUNT_DISABLED(40102, "账号已被禁用，请联系管理员"),
    INVALID_TOKEN(40105, "身份凭证无效"),
    FORBIDDEN(40300, "没有操作权限"),
    PERMISSION_REQUIRED(40300, "缺少权限: %s"),
    INTERNAL_ERROR(50000, "系统内部错误"),
    DATABASE_FAILURE(50100, "数据库操作失败");

    private final Details details;

    CommonErrorType(int code, String template) {
        this.details = new Details(code, template);
    }

    @Override
    public Details details() {
        return details;
    }
}
