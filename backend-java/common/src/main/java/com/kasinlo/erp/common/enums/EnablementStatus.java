package com.kasinlo.erp.common.enums;

public enum EnablementStatus implements CodeEnum<Integer> {
    DISABLED(0, "停用"),
    ENABLED(1, "启用");

    private final Integer code;
    private final String description;

    EnablementStatus(Integer code, String description) {
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
