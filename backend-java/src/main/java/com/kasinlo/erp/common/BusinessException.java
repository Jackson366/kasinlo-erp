package com.kasinlo.erp.common;

public class BusinessException extends RuntimeException {
    public final int status;
    public final int code;

    public BusinessException(int status, String message) {
        this(status, status * 100, message);
    }

    public BusinessException(int status, int code, String message) {
        super(message);
        this.status = status;
        this.code = code;
    }

    public static void require(boolean condition, String message) {
        if (!condition) throw new BusinessException(400, message);
    }

    public static void conflict(boolean condition, String message) {
        if (!condition) throw new BusinessException(409, message);
    }
}
