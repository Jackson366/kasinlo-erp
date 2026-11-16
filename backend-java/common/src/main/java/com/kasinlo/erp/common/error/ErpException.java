package com.kasinlo.erp.common.error;

import com.alibaba.cola.exception.BizException;

public final class ErpException extends BizException {
    private static final long serialVersionUID = 1L;

    private final ErpError error;

    public ErpException(ErpError error, Object... arguments) {
        super(Integer.toString(error.code()), error.message(arguments));
        this.error = error;
    }

    public ErpException(ErpError error, Throwable cause) {
        super(Integer.toString(error.code()), error.message(), cause);
        this.error = error;
    }

    public ErpError error() {
        return error;
    }
}
