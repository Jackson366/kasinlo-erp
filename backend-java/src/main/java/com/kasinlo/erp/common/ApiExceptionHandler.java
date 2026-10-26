package com.kasinlo.erp.common;

import org.slf4j.*;
import org.springframework.dao.*;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;
import java.util.UUID;

import static com.kasinlo.erp.common.Values.*;

@RestControllerAdvice
public class ApiExceptionHandler {
    private static final Logger log = LoggerFactory.getLogger(ApiExceptionHandler.class);

    @ExceptionHandler(BusinessException.class)
    public ResponseEntity<?> business(BusinessException e) {
        return ResponseEntity.status(e.status).body(map("code", e.code, "message", e.getMessage(), "traceId",
            UUID.randomUUID().toString()));
    }

    @ExceptionHandler(DuplicateKeyException.class)
    public ResponseEntity<?> duplicate(Exception e) {
        return business(new BusinessException(409, "编号已存在"));
    }

    @ExceptionHandler(Exception.class)
    public ResponseEntity<?> other(Exception e) {
        String trace = UUID.randomUUID().toString();
        log.error("Request failed trace={}", trace, e);
        return ResponseEntity.status(500).body(map("code", 50000, "message", "服务器处理失败", "traceId", trace));
    }
}
