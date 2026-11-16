package com.kasinlo.erp.start;

import com.kasinlo.erp.common.enums.CodeEnum;
import com.kasinlo.erp.common.enums.EnablementStatus;
import com.kasinlo.erp.common.enums.EnumUtils;
import com.kasinlo.erp.common.error.ErpException;
import com.kasinlo.erp.domain.inventory.InventoryErrorType;
import com.kasinlo.erp.domain.inventory.StockMovementType;
import com.kasinlo.erp.domain.trade.TradeOrderStatus;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class ErpCommonTest {
    @Test
    void looksUpStableCodesAndDescriptionsAndListsEnumsInDeclarationOrder() {
        assertThat(EnumUtils.getByCode(EnablementStatus.class, 1)).contains(EnablementStatus.ENABLED);
        assertThat(EnumUtils.getByCode(TradeOrderStatus.class, 2)).contains(TradeOrderStatus.APPROVED);
        assertThat(EnumUtils.getByCode(TradeOrderStatus.class, null)).isEmpty();
        assertThat(EnumUtils.getByCode(TradeOrderStatus.class, 99)).isEmpty();
        assertThat(EnumUtils.getDescription(StockMovementType.class, "out")).contains("出库");
        assertThat(EnumUtils.getDescription(StockMovementType.class, "OUT")).isEmpty();
        assertThat(EnumUtils.containsCode(StockMovementType.class, "in")).isTrue();
        assertThat(EnumUtils.containsCode(StockMovementType.class, "unknown")).isFalse();
        assertThat(EnumUtils.getAll(StockMovementType.class)).containsExactly(StockMovementType.IN, StockMovementType.OUT);
        assertThatThrownBy(() -> EnumUtils.getAll(DuplicateCode.class)).isInstanceOf(IllegalStateException.class);
    }

    @Test
    void errorsKeepTheirLegacyCodeAndCarryTheSpecificReasonAndMessage() {
        var shortage = new ErpException(InventoryErrorType.STOCK_INSUFFICIENT, "螺钉 M8", "3");
        var duplicate = new IllegalStateException("unique constraint");
        var numberConflict = new ErpException(InventoryErrorType.STOCK_ORDER_NUMBER_CONFLICT, duplicate);

        assertThat(shortage.getErrCode()).isEqualTo("40900");
        assertThat(shortage.getMessage()).isEqualTo("「螺钉 M8」库存不足（现有 3）");
        assertThat(shortage.error()).isEqualTo(InventoryErrorType.STOCK_INSUFFICIENT);
        assertThat(numberConflict.getErrCode()).isEqualTo("40900");
        assertThat(numberConflict.error()).isEqualTo(InventoryErrorType.STOCK_ORDER_NUMBER_CONFLICT);
        assertThat(numberConflict.getCause()).isSameAs(duplicate);
    }

    private enum DuplicateCode implements CodeEnum<Integer> {
        FIRST, SECOND;

        @Override
        public Integer getCode() {
            return 1;
        }

        @Override
        public String getDescription() {
            return name();
        }
    }
}
