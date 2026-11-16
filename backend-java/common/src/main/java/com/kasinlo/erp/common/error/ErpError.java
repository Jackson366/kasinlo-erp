package com.kasinlo.erp.common.error;

/** API-compatible error details. Distinct error reasons may share the same legacy response code. */
public interface ErpError {
    Details details();

    default int code() {
        return details().code();
    }

    default int httpStatus() {
        return details().httpStatus();
    }

    default String message(Object... arguments) {
        return details().template().formatted(arguments);
    }

    record Details(int code, int httpStatus, String template) {}
}
