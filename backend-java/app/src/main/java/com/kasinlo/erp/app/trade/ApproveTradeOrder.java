package com.kasinlo.erp.app.trade;

import com.alibaba.cola.exception.BizException;
import com.kasinlo.erp.domain.inventory.StockGateway;
import com.kasinlo.erp.domain.inventory.StockPosting;
import com.kasinlo.erp.domain.inventory.StockPosting.Direction;
import com.kasinlo.erp.domain.settlement.ApprovalSettlement;
import com.kasinlo.erp.domain.settlement.SettlementGateway;
import com.kasinlo.erp.domain.trade.TradeOrder;
import com.kasinlo.erp.domain.trade.TradeOrderGateway;
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
    private final Clock clock;

    public ApproveTradeOrder(TradeOrderGateway trade, StockGateway stock, SettlementGateway settlement, Clock clock) {
        this.trade = trade;
        this.stock = stock;
        this.settlement = settlement;
        this.clock = clock;
    }

    @Transactional
    public void execute(long orderId, Actor actor, boolean settleNow) {
        TradeOrder order = trade.lock(orderId);
        if (order == null) {
            throw new BizException("40400", "单据不存在");
        }
        order.checkApprovable();
        Direction direction = switch (order.bizType()) {
            case "purchase" -> Direction.IN;
            case "sales" -> Direction.OUT;
            default -> throw new BizException("40000", "单据业务类型非法");
        };
        List<TradeOrder.Item> lines = trade.lines(order.id());
        if (lines.isEmpty()) {
            throw new BizException("40000", "单据明细为空，无法过账");
        }
        OffsetDateTime now = OffsetDateTime.now(clock);
        StockPosting posting = new StockPosting(order.id(), order.bizType(), direction, order.orderDate(),
                order.salesman(), actor.id(), lines.stream()
                        .map(line -> new StockPosting.Item(line.productId(), line.productName(), line.specId(),
                                line.spec(), line.unitId(), line.unit(), line.rate(), line.qty(), line.qtyBase()))
                        .toList());
        List<StockPosting.Change> changes = posting.changes();
        long stockOrderId = stock.save(posting, now);
        for (StockPosting.Change change : changes) {
            applyStockChange(change);
        }
        if (!trade.approve(order.id(), actor.id(), now)) {
            throw new BizException("40900", "单据已被审核，请勿重复操作");
        }
        ApprovalSettlement.fullPayment(settleNow, order.totalAmount(), order.settledAmount()).ifPresent(amount -> {
            if (!settlement.payOnApproval(order.id(), order.bizType(), order.partyId(), amount, actor.id(), now)) {
                throw ApprovalSettlement.overpaid();
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
            throw new BizException("40900", "「" + change.label() + "」商品规格已不存在，无法更新库存");
        }
        throw new BizException("40900", "「" + change.label() + "」库存不足（现有 "
                + available.stripTrailingZeros().toPlainString() + "）");
    }

    public record Actor(long id, String name) {
        public Actor {
            if (id <= 0 || name == null) {
                throw new IllegalArgumentException("审核需要有效的操作人");
            }
        }
    }
}
