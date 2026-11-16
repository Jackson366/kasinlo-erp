package com.kasinlo.erp.adapter;

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

    @ExceptionHandler(ApiFailure.class)
    ResponseEntity<ApiResponse> clientError(ApiFailure error) {
        return ResponseEntity.status(error.code() / 100).cacheControl(CacheControl.noStore())
                .body(new ApiResponse(error.code(), error.getMessage(), null));
    }

    @ExceptionHandler(DataAccessException.class)
    ResponseEntity<ApiResponse> databaseError(DataAccessException error) {
        LOG.error("库存查询数据库操作失败", error);
        return ResponseEntity.internalServerError().cacheControl(CacheControl.noStore())
                .body(new ApiResponse(50100, "数据库操作失败", null));
    }
}
