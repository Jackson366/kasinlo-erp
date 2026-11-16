package com.kasinlo.erp.adapter;

import com.kasinlo.erp.common.error.CommonErrorType;
import com.kasinlo.erp.common.error.ErpError;
import com.kasinlo.erp.common.error.ErpException;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.dao.DataAccessException;
import org.springframework.http.CacheControl;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;

@RestControllerAdvice
class ApiErrorHandler {
    private static final Logger LOG = LoggerFactory.getLogger(ApiErrorHandler.class);

    @ExceptionHandler(ErpException.class)
    ResponseEntity<Result<Void>> businessError(ErpException exception) {
        return errorResponse(exception.error(), exception.getMessage());
    }

    @ExceptionHandler(DataAccessException.class)
    ResponseEntity<Result<Void>> databaseError(DataAccessException error) {
        LOG.error("库存查询数据库操作失败", error);
        return errorResponse(CommonErrorType.DATABASE_FAILURE, CommonErrorType.DATABASE_FAILURE.message());
    }

    private ResponseEntity<Result<Void>> errorResponse(ErpError error, String message) {
        return ResponseEntity.status(error.httpStatus()).cacheControl(CacheControl.noStore())
                .body(new Result<>(error.code(), message, null));
    }
}
