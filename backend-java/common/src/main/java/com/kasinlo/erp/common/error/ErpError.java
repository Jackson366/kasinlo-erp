package com.kasinlo.erp.common.error;

/** Application error contract. Distinct error reasons may share the same legacy code. */
public interface ErpError {
    Details details();

    default int code() {
        return details().code();
    }

    default String message(Object... arguments) {
        return details().template().formatted(arguments);
    }

    record Details(int code, String template) {}
}
