package com.kasinlo.erp.module.inv;

import com.kasinlo.erp.persistence.ErpRepository;
import com.kasinlo.erp.module.system.IdentityService;
import com.kasinlo.erp.module.system.IdentityService.Actor;
import org.springframework.stereotype.Service;
import java.math.*;
import java.time.*;
import java.util.*;

import static com.kasinlo.erp.common.Values.*;

@Service
public class ReportingService {
    final ErpRepository db;
    final IdentityService auth;
    final MasterDataService master;
    final ContractService contracts;

    public ReportingService(ErpRepository db, IdentityService auth, MasterDataService master, ContractService contracts) {
        this.db = db;
        this.auth = auth;
        this.master = master;
        this.contracts = contracts;
    }

    BigDecimal sum(List<Map<String, Object>> list, String key) {
        return list.stream().map(r -> d(r, key)).reduce(BigDecimal.ZERO, BigDecimal::add);
    }

    String start(Map<String, String> q) {
        if (!q.getOrDefault("startDate", "").isBlank()) return q.get("startDate");
        LocalDate t = LocalDate.parse(today());
        return switch (q.getOrDefault("range", "month")) {
            case "today" -> t.toString();
            case "week" -> t.minusDays(t.getDayOfWeek().getValue() - 1).toString();
            case "quarter" -> t.withMonth(((t.getMonthValue() - 1) / 3) * 3 + 1).withDayOfMonth(1).toString();
            case "year" -> t.withDayOfYear(1).toString();
            default -> t.withDayOfMonth(1).toString();
        };
    }

    List<Map<String, Object>> orders(Map<String, String> q, Actor a) {
        String from = start(q), to = q.getOrDefault("endDate", today());
        return auth.scoped("inv_orders", a).stream().filter(o -> n(o, "status") == 2 && s(o, "orderDate").compareTo(from) >=
            0 && s(o, "orderDate").compareTo(to) <= 0).toList();
    }

    public Object report(String h, Map<String, String> q, Actor a) {
        var orders = orders(q, a);
        var sales = orders.stream().filter(o -> s(o, "bizType").equals("sales")).toList();
        var purchases = orders.stream().filter(o -> s(o, "bizType").equals("purchase")).toList();
        if (h.equals("Overview")) {
            var ar = contracts.finance("sales", a);
            var ap = contracts.finance("purchase", a);
            return map("customerCount", db.all("inv_customers").size(), "activeCustomer", db.all("inv_customers")
                .stream().filter(c -> n(c, "status") == 1).count(), "monthNewCustomer", db.all("inv_customers")
                .stream().filter(c -> s(c, "createdAt").compareTo(today().substring(0, 7) + "-01") >= 0).count(),
                "receivableTotal", sum(ar, "outstanding"), "payableTotal", sum(ap, "outstanding"), "monthSales",
                sum(sales, "totalAmount"), "monthPurchase", sum(purchases, "totalAmount"), "pendingOrders",
                auth.scoped("inv_orders", a).stream().filter(o -> n(o, "status") == 1).count(), "productCount",
                db.all("inv_products").size(), "warnProductCount", master.inventory().stream().filter(i -> bool(i.get("warn")))
                .count());
        }
        if (h.equals("ReportSummary")) {
            BigDecimal amount = sum(sales, "totalAmount"), cost = BigDecimal.ZERO;
            for (var o : sales) for (var it : db.where("inv_order_items", "orderId", n(o, "id"))) {
                var bases = db.where("inv_product_units", "specId", n(it, "specId")).stream().filter(u -> bool(u.get("isBase")))
                    .toList();
                if (!bases.isEmpty()) cost = cost.add(d(bases.get(0), "purchasePrice").multiply(d(it,
                    "qtyBase")));
            }
            BigDecimal profit = money(amount.subtract(cost));
            return map("range", q.getOrDefault("range", "month"), "salesAmount", amount, "costAmount",
                money(cost), "grossProfit", profit, "grossRate", amount.signum() == 0 ? 0 : profit.multiply(BigDecimal.valueOf(100))
                .divide(amount, 2, RoundingMode.HALF_UP), "salesOrders", sales.size(), "purchaseOrders",
                purchases.size(), "purchases", sum(purchases, "totalAmount"));
        }
        if (h.equals("ReportTrend")) {
            var days = new TreeMap<String, Map<String, Object>>();
            LocalDate f = LocalDate.parse(start(q)), to = LocalDate.parse(q.getOrDefault("endDate", today()));
            for (int i = 0; !f.isAfter(to) && i < 366; i++, f = f.plusDays(1)) days.put(f.toString(),
                map("date", f.toString(), "salesAmount", BigDecimal.ZERO, "purchaseAmount", BigDecimal.ZERO));
            for (var o : orders) {
                String dt = s(o, "orderDate");
                var r = days.computeIfAbsent(dt, k -> map("date", k, "salesAmount", BigDecimal.ZERO, "purchaseAmount",
                    BigDecimal.ZERO));
                String key = s(o, "bizType").equals("sales") ? "salesAmount" : "purchaseAmount";
                r.put(key, d(r, key).add(d(o, "totalAmount")));
            }
            return new ArrayList<>(days.values());
        }
        if (h.equals("ReportInventory") || h.equals("ReportStockRanking")) {
            var inv = master.inventory().stream().filter(i -> n(i, "status") == 1).toList();
            var cat = new LinkedHashMap<String, Map<String, Object>>();
            var ranked = new ArrayList<Map<String, Object>>();
            BigDecimal total = BigDecimal.ZERO;
            for (var i : inv) {
                BigDecimal value = money(d(i, "stock").multiply(d(i, "purchasePrice")));
                total = total.add(value);
                var r = new LinkedHashMap<>(i);
                r.put("value", value);
                ranked.add(r);
                var c = cat.computeIfAbsent(s(i, "categoryName"), k -> map("category", k, "value", BigDecimal.ZERO,
                    "ratio", 0));
                c.put("value", d(c, "value").add(value));
            }
            ranked.sort((x, y) -> d(y, "value").compareTo(d(x, "value")));
            for (int i = 0; i < ranked.size(); i++) {
                ranked.get(i).put("rank", i + 1);
                ranked.get(i).put("ratio", ratio(d(ranked.get(i), "value"), total));
            }
            for (var c : cat.values()) c.put("ratio", ratio(d(c, "value"), total));
            if (h.equals("ReportStockRanking")) return ranked;
            return map("totalValue", total, "specCount", inv.size(), "warnCount", inv.stream().filter(i -> bool(i.get("warn")))
                .count(), "categoryValue", new ArrayList<>(cat.values()), "warnList", inv.stream().filter(i -> bool(i.get("warn")))
                .limit(20).toList());
        }
        if (h.equals("ReportContract")) {
            var cs = auth.scoped("inv_contracts", a).stream().filter(c -> q.getOrDefault("bizType", "").isBlank() ||
                s(c, "bizType").equals(q.get("bizType"))).map(contracts::enrich).toList();
            for (var c : cs) c.put("contractId", c.get("id"));
            var out = map("rows", cs, "count", cs.size(), "truncated", false);
            for (String k : List.of("amount", "planAmount", "settledAmount", "outstanding", "retentionAmount",
                "retentionOutstanding")) out.put(k, sum(cs, k));
            return out;
        }
        if (h.equals("ReportContractLedger")) {
            var out = new ArrayList<Map<String, Object>>();
            for (var c : auth.scoped("inv_contracts", a)) for (var p : contracts.plans(c)) for (var rec : db.where("inv_settlements",
                "planId", n(p, "id"))) out.add(map("settleId", rec.get("id"), "settledAt", rec.get("settledAt"),
                "contractNo", c.get("contractNo"), "name", c.get("name"), "bizType", c.get("bizType"),
                "partyName", c.get("partyName"), "planSeq", p.get("seq"), "planName", p.get("name"), "amount",
                rec.get("amount"), "method", rec.get("method"), "remark", rec.get("remark")));
            return out;
        }
        String biz = q.getOrDefault("bizType", "sales");
        var selected = orders.stream().filter(o -> s(o, "bizType").equals(biz)).toList();
        var detail = new ArrayList<Map<String, Object>>();
        for (var o : selected) for (var it : db.where("inv_order_items", "orderId", n(o, "id"))) {
            var r = new LinkedHashMap<>(it);
            r.putAll(map("orderId", o.get("id"), "orderNo", o.get("orderNo"), "orderDate", o.get("orderDate"),
                "partyName", o.get("partyName"), "partyId", o.get("partyId"), "salesman", o.get("salesman")));
            detail.add(r);
        }
        if (h.equals("ReportDetail")) return map("rows", detail, "total", detail.size(), "amount", sum(detail,
            "amount"), "qty", sum(detail, "qty"), "truncated", false);
        var groups = new LinkedHashMap<String, Map<String, Object>>();
        String dim = q.getOrDefault("dimension", "customer");
        if (h.equals("ReportCategory")) dim = "category";
        for (var it : detail) {
            String name;
            long gid;
            if (dim.equals("product")) {
                name = s(it, "productName");
                gid = n(it, "productId");
            } else if (dim.equals("category")) {
                try {
                    var p = master.product(n(it, "productId"));
                    name = s(p, "categoryName");
                    gid = n(p, "categoryId");
                } catch (Exception e) {
                    name = "未分类";
                    gid = 0;
                }
            } else {
                name = s(it, "partyName");
                gid = n(it, "partyId");
            }
            String key = Long.toString(gid);
            final String groupName = name;
            var g = groups.computeIfAbsent(key, k -> map("id", Long.parseLong(k), "name", groupName, "category",
                groupName, "amount", BigDecimal.ZERO, "quantity", BigDecimal.ZERO, "orderCount", 0, "orderIds",
                new HashSet<Long>()));
            g.put("amount", d(g, "amount").add(d(it, "amount")));
            g.put("quantity", d(g, "quantity").add(d(it, "qtyBase")));
            ((Set<Long>) g.get("orderIds")).add(n(it, "orderId"));
        }
        var result = new ArrayList<>(groups.values());
        result.sort((x, y) -> d(y, "amount").compareTo(d(x, "amount")));
        BigDecimal total = sum(result, "amount");
        for (int i = 0; i < result.size(); i++) {
            var r = result.get(i);
            r.put("rank", i + 1);
            r.put("ratio", ratio(d(r, "amount"), total));
            r.put("orderCount",((Set<?>) r.remove("orderIds")).size());
        }
        return result;
    }

    BigDecimal ratio(BigDecimal amount, BigDecimal total) {
        return total.signum() == 0 ? BigDecimal.ZERO : amount.multiply(BigDecimal.valueOf(100)).divide(total,
            2, RoundingMode.HALF_UP);
    }
}
