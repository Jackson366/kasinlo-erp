package com.kasinlo.erp.adapter;

import com.kasinlo.erp.common.error.CommonErrorType;
import com.kasinlo.erp.common.error.ErpError;
import com.kasinlo.erp.common.error.ErpException;
import jakarta.validation.ConstraintViolationException;
import jakarta.validation.ElementKind;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.dao.DataAccessException;
import org.springframework.http.CacheControl;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.MethodArgumentNotValidException;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.method.annotation.HandlerMethodValidationException;

import java.util.stream.StreamSupport;

@RestControllerAdvice
class ApiErrorHandler {
    private static final Logger LOG = LoggerFactory.getLogger(ApiErrorHandler.class);

    @ExceptionHandler(ErpException.class)
    ResponseEntity<Result<Void>> businessError(ErpException exception) {
        return errorResponse(exception.error(), exception.getMessage());
    }

    @ExceptionHandler(MethodArgumentNotValidException.class)
    ResponseEntity<Result<Void>> invalidRequestBody() {
        return errorResponse(CommonErrorType.INVALID_ARGUMENT);
    }

    @ExceptionHandler(HandlerMethodValidationException.class)
    ResponseEntity<Result<Void>> invalidControllerMethod(HandlerMethodValidationException exception) {
        if (exception.isForReturnValue()) {
            LOG.error("接口返回值校验失败", exception);
            return errorResponse(CommonErrorType.INTERNAL_ERROR);
        }
        return errorResponse(CommonErrorType.INVALID_ARGUMENT);
    }

    @ExceptionHandler(ConstraintViolationException.class)
    ResponseEntity<Result<Void>> invalidApplicationMethod(ConstraintViolationException exception) {
        boolean invalidReturnValue = exception.getConstraintViolations().stream()
                .flatMap(violation -> StreamSupport.stream(violation.getPropertyPath().spliterator(), false))
                .anyMatch(node -> node.getKind() == ElementKind.RETURN_VALUE);
        if (invalidReturnValue) {
            LOG.error("应用服务返回值校验失败", exception);
            return errorResponse(CommonErrorType.INTERNAL_ERROR);
        }
        return errorResponse(CommonErrorType.INVALID_ARGUMENT);
    }

    @ExceptionHandler(DataAccessException.class)
    ResponseEntity<Result<Void>> databaseError(DataAccessException error) {
        LOG.error("库存查询数据库操作失败", error);
        return errorResponse(CommonErrorType.DATABASE_FAILURE);
    }

    private ResponseEntity<Result<Void>> errorResponse(ErpError error) {
        return errorResponse(error, error.message());
    }

    private ResponseEntity<Result<Void>> errorResponse(ErpError error, String message) {
        int code = error.code();
        return ResponseEntity.status(httpStatusFor(code)).cacheControl(CacheControl.noStore())
                .body(new Result<>(code, message, null));
    }

    private static HttpStatus httpStatusFor(int code) {
        return switch (code) {
            case 40000 -> HttpStatus.BAD_REQUEST;
            case 40100, 40102, 40105 -> HttpStatus.UNAUTHORIZED;
            case 40300 -> HttpStatus.FORBIDDEN;
            case 40400 -> HttpStatus.NOT_FOUND;
            case 40900 -> HttpStatus.CONFLICT;
            default -> HttpStatus.INTERNAL_SERVER_ERROR;
        };
    }
}
