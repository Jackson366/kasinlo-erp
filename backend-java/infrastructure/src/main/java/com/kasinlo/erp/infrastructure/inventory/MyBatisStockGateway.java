package com.kasinlo.erp.infrastructure.inventory;

import com.kasinlo.erp.common.error.ErpException;
import com.kasinlo.erp.domain.inventory.InventoryErrorType;
import com.kasinlo.erp.domain.inventory.StockGateway;
import com.kasinlo.erp.domain.inventory.StockOrderStatus;
import com.kasinlo.erp.domain.inventory.StockPosting;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.stereotype.Repository;

import java.math.BigDecimal;
import java.time.OffsetDateTime;
import java.time.format.DateTimeFormatter;
import java.util.Locale;

@Repository
public class MyBatisStockGateway implements StockGateway {
    private final StockPostingMapper mapper;

    public MyBatisStockGateway(StockPostingMapper mapper) {
        this.mapper = mapper;
    }

    @Override
    public long save(StockPosting posting, OffsetDateTime at) {
        StockOrderData row = new StockOrderData();
        String prefix = "CK" + at.toLocalDate().format(DateTimeFormatter.BASIC_ISO_DATE);
        row.orderNo = prefix + String.format(Locale.ROOT, "%04d", mapper.countNumbers(prefix + "%") + 1);
        row.direction = posting.direction().getCode();
        row.sourceType = posting.sourceType();
        row.sourceId = posting.sourceId();
        row.orderDate = posting.orderDate();
        row.status = StockOrderStatus.POSTED.getCode();
        row.remark = "";
        row.salesman = posting.salesman();
        row.createdBy = posting.userId();
        row.createdAt = at;
        row.updatedAt = at;
        try {
            mapper.insert(row);
        } catch (DuplicateKeyException e) {
            throw new ErpException(InventoryErrorType.STOCK_ORDER_NUMBER_CONFLICT, e);
        }
        for (StockPosting.Item item : posting.items()) {
            mapper.insertLine(row.id, item, at);
        }
        return row.id;
    }

    @Override
    public boolean changeStock(long specId, BigDecimal delta) {
        return mapper.changeStock(specId, delta);
    }

    @Override
    public BigDecimal stock(long specId) {
        return mapper.stock(specId);
    }
}
