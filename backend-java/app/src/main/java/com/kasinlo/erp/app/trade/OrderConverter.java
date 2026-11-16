package com.kasinlo.erp.app.trade;

import com.kasinlo.erp.domain.inventory.StockPosting;
import com.kasinlo.erp.domain.trade.TradeOrder;
import org.mapstruct.Mapper;
import org.mapstruct.MappingConstants;
import org.mapstruct.ReportingPolicy;

import java.util.List;

@Mapper(componentModel = MappingConstants.ComponentModel.SPRING, unmappedTargetPolicy = ReportingPolicy.ERROR)
public interface OrderConverter {
    StockPosting.Item toStockPostingItem(TradeOrder.Item item);

    List<StockPosting.Item> toStockPostingItems(List<TradeOrder.Item> items);
}
