package com.kasinlo.erp.app.trade;

import com.kasinlo.erp.common.error.ErpException;
import com.kasinlo.erp.domain.inventory.InventoryErrorType;
import com.kasinlo.erp.domain.inventory.StockGateway;
import com.kasinlo.erp.domain.inventory.StockMovementType;
import com.kasinlo.erp.domain.inventory.StockPosting;
import com.kasinlo.erp.domain.settlement.ApprovalSettlement;
import com.kasinlo.erp.domain.settlement.SettlementErrorType;
import com.kasinlo.erp.domain.settlement.SettlementGateway;
import com.kasinlo.erp.domain.trade.TradeErrorType;
import com.kasinlo.erp.domain.trade.TradeOrder;
import com.kasinlo.erp.domain.trade.TradeOrderGateway;
import com.kasinlo.erp.domain.trade.TradeOrderType;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.math.BigDecimal;
import java.time.Clock;
import java.time.OffsetDateTime;
import java.util.List;

@Service
public class ApproveTradeOrder {
    private final TradeOrderGateway trade;
    private final StockGateway stock;
    private final SettlementGateway settlement;
    private final OrderConverter orderConverter;
    private final Clock clock;

    public ApproveTradeOrder(TradeOrderGateway trade, StockGateway stock, SettlementGateway settlement,
                             OrderConverter orderConverter, Clock clock) {
        this.trade = trade;
        this.stock = stock;
        this.settlement = settlement;
        this.orderConverter = orderConverter;
        this.clock = clock;
    }

    @Transactional
    public void execute(long orderId, Actor actor, boolean settleNow) {
        TradeOrder order = trade.lock(orderId);
        if (order == null) {
            throw new ErpException(TradeErrorType.ORDER_NOT_FOUND);
        }
        order.checkApprovable();
        TradeOrderType type = order.type();
        StockMovementType direction = switch (type) {
            case PURCHASE -> StockMovementType.IN;
            case SALES -> StockMovementType.OUT;
        };
        List<TradeOrder.Item> lines = trade.lines(order.id());
        if (lines.isEmpty()) {
            throw new ErpException(TradeErrorType.ORDER_LINES_EMPTY);
        }
        OffsetDateTime now = OffsetDateTime.now(clock);
        StockPosting posting = new StockPosting(order.id(), type.getCode(), direction, order.orderDate(),
                order.salesman(), actor.id(), orderConverter.toStockPostingItems(lines));
        List<StockPosting.Change> changes = posting.changes();
        long stockOrderId = stock.save(posting, now);
        for (StockPosting.Change change : changes) {
            applyStockChange(change);
        }
        if (!trade.approve(order.id(), actor.id(), now)) {
            throw new ErpException(TradeErrorType.ORDER_APPROVAL_CONFLICT);
        }
        ApprovalSettlement.fullPayment(settleNow, order.totalAmount(), order.settledAmount()).ifPresent(amount -> {
            if (!settlement.payOnApproval(order.id(), type.getCode(), order.partyId(), amount, actor.id(), now)) {
                throw new ErpException(SettlementErrorType.SETTLEMENT_OVERPAID);
            }
        });
        trade.recordApproval(order.id(), order.orderNo(), actor.id(), actor.name(), stockOrderId, now);
    }

    private void applyStockChange(StockPosting.Change change) {
        if (stock.changeStock(change.specId(), change.delta())) {
            return;
        }
        BigDecimal available = stock.stock(change.specId());
        if (available == null) {
            throw new ErpException(InventoryErrorType.STOCK_SPEC_REMOVED, change.label());
        }
        throw new ErpException(InventoryErrorType.STOCK_INSUFFICIENT,
                change.label(), available.stripTrailingZeros().toPlainString());
    }

    public record Actor(long id, String name) {
        public Actor {
            if (id <= 0 || name == null) {
                throw new IllegalArgumentException(TradeErrorType.APPROVAL_ACTOR_INVALID.message());
            }
        }
    }
}
