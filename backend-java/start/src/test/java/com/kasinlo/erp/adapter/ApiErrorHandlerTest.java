package com.kasinlo.erp.adapter;

import com.kasinlo.erp.common.error.CommonErrorType;
import com.kasinlo.erp.common.error.ErpException;
import com.kasinlo.erp.domain.trade.TradeErrorType;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;

import static org.assertj.core.api.Assertions.assertThat;

class ApiErrorHandlerTest {
    private final ApiErrorHandler handler = new ApiErrorHandler();

    @Test
    void preservesHttpStatusAndApplicationCode() {
        var databaseFailure = handler.businessError(new ErpException(CommonErrorType.DATABASE_FAILURE));
        assertThat(databaseFailure.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
        assertThat(databaseFailure.getBody()).isEqualTo(new Result<Void>(50100, "数据库操作失败", null));

        var orderNotFound = handler.businessError(new ErpException(TradeErrorType.ORDER_NOT_FOUND));
        assertThat(orderNotFound.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
        assertThat(orderNotFound.getBody()).isEqualTo(new Result<Void>(40400, "单据不存在", null));
    }
}
