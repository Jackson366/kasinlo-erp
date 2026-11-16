package com.kasinlo.erp.adapter;

final class ApiFailure extends RuntimeException {
    private final int code;

    ApiFailure(int code, String message) {
        super(message);
        this.code = code;
    }

    int code() {
        return code;
    }
}
