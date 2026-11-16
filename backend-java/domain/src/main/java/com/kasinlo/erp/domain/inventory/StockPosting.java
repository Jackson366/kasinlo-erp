package com.kasinlo.erp.domain.inventory;

import com.alibaba.cola.exception.BizException;

import java.math.BigDecimal;
import java.math.RoundingMode;
import java.time.LocalDate;
import java.util.List;
import java.util.Map;
import java.util.TreeMap;

public record StockPosting(long sourceId, String sourceType, Direction direction, LocalDate orderDate,
                           String salesman, long userId, List<Item> items) {
    public StockPosting {
        items = List.copyOf(items);
    }

    public enum Direction {
        IN("in"), OUT("out");

        private final String code;

        Direction(String code) {
            this.code = code;
        }

        public String code() {
            return code;
        }
    }

    public List<Change> changes() {
        // A stable spec order keeps overlapping postings from taking spec locks in opposite order.
        Map<Long, Change> bySpec = new TreeMap<>();
        for (Item item : items) {
            if (item.specId() <= 0) {
                throw new BizException("40000", "出入库明细缺少规格，无法过账");
            }
            BigDecimal delta = direction == Direction.OUT ? item.qtyBase().negate() : item.qtyBase();
            bySpec.merge(item.specId(), new Change(item.specId(), item.productName() + " " + item.spec(), delta),
                    (a, b) -> new Change(a.specId(), a.label(), a.delta().add(b.delta())));
        }
        return bySpec.values().stream()
                .map(change -> new Change(change.specId(), change.label(), change.delta().setScale(4, RoundingMode.HALF_UP)))
                .filter(change -> change.delta().signum() != 0)
                .toList();
    }

    public record Item(long productId, String productName, long specId, String spec, long unitId,
                       String unit, BigDecimal rate, BigDecimal qty, BigDecimal qtyBase) {}

    public record Change(long specId, String label, BigDecimal delta) {}
}
