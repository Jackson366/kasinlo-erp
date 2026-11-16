package com.kasinlo.erp.domain.auth;

import com.kasinlo.erp.common.enums.CodeEnum;

public enum InventoryAccessStatus implements CodeEnum<String> {
    ALLOWED("allowed", "允许访问"),
    USER_MISSING("user_missing", "用户不存在"),
    ACCOUNT_DISABLED("account_disabled", "账号已停用"),
    TOKEN_REVOKED("token_revoked", "令牌已撤销"),
    FORBIDDEN("forbidden", "缺少权限");

    private final String code;
    private final String description;

    InventoryAccessStatus(String code, String description) {
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
