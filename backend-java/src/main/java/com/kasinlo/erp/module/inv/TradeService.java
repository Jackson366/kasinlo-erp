package com.kasinlo.erp.module.inv;

import com.kasinlo.erp.persistence.ErpRepository;
import com.kasinlo.erp.module.system.IdentityService;
import com.kasinlo.erp.module.system.IdentityService.Actor;
import com.kasinlo.erp.common.BusinessException;
import org.springframework.stereotype.Service;
import java.math.BigDecimal;
import java.util.*;

import static com.kasinlo.erp.common.Values.*;
import static com.kasinlo.erp.common.BusinessException.*;

@Service
public class TradeService {
    final ErpRepository db;
    final IdentityService auth;

    public TradeService(ErpRepository db, IdentityService auth) {
        this.db = db;
        this.auth = auth;
    }

    public List<Map<String, Object>> buildItems(List<Map<String, Object>> requested, String biz) {
        require(!requested.isEmpty(), "至少填写一条明细");
        var out = new ArrayList<Map<String, Object>>();
        for (var r : requested) {
            var p = db.find("inv_products", n(r, "productId"));
            var sp = db.find("inv_product_specs", n(r, "specId"));
            var u = db.find("inv_product_units", n(r, "unitId"));
            require(n(p, "status") == 1 && n(sp, "status") == 1 && n(u, "status") == 1, "商品、规格或单位已停用");
            require(n(sp, "productId") == n(p, "id") && n(u, "specId") == n(sp, "id"), "商品规格单位不匹配");
            require(d(r, "qty").signum() > 0 && d(r, "price").signum() >= 0, "数量或价格非法");
            BigDecimal price = d(r, "price");
            if (price.signum() == 0) price = d(u, biz.equals("purchase") ? "purchasePrice" : "salePrice");
            out.add(map("productId", p.get("id"), "productCode", p.get("code"), "productName", p.get("name"),
                "specId", sp.get("id"), "spec", sp.get("spec"), "unitId", u.get("id"), "unit", u.get("unit"),
                "rate", u.get("rate"), "qty", money(d(r, "qty")), "qtyBase", qty(d(r, "qty").multiply(d(u,
                "rate"))), "price", price, "amount", money(d(r, "qty").multiply(price))));
        }
        return out;
    }

    public void applyStock(String direction, List<Map<String, Object>> items) {
        var sorted = new ArrayList<>(items);
        sorted.sort(Comparator.comparingLong(i -> n(i, "specId")));
        for (var it : sorted) {
            long sid = n(it, "specId");
            db.lock("inv_product_specs", sid);
            BigDecimal delta = d(it, "qtyBase");
            if ("out".equals(direction)) delta = delta.negate();
            int count = db.adjustStock(sid, delta);
            conflict(count == 1, "库存不足：" + s(it, "productName") + " / " + s(it, "spec"));
        }
    }

    public void bizPerm(Actor a, String biz, String action) {
        auth.require(a, "inv:" + biz + ":" + action);
    }

    public String operator(long id) {
        try {
            var u = db.find("sys_users", id);
            return s(u, "nickname").isBlank() ? s(u, "username") : s(u, "nickname");
        } catch (Exception e) {
            return "";
        }
    }

    public Map<String, Object> order(long id, Actor a) {
        var o = db.find("inv_orders", id);
        auth.visible(a, o);
        bizPerm(a, s(o, "bizType"), "list");
        return enrichOrder(o);
    }

    public Map<String, Object> enrichOrder(Map<String, Object> o) {
        var items = db.where("inv_order_items", "orderId", n(o, "id"));
        for (var it : items) try {
            it.put("productCode", db.find("inv_products", n(it, "productId")).get("code"));
        } catch (Exception e) {
            it.put("productCode", "");
        }
        o.put("items", items);
        o.put("itemCount", items.size());
        o.put("settleStatus", settle(d(o, "totalAmount"), d(o, "settledAmount")));
        o.put("operator", operator(n(o, "createdBy")));
        return o;
    }

    public List<Map<String, Object>> orders(Actor a, String biz) {
        bizPerm(a, biz, "list");
        return auth.scoped("inv_orders", a).stream().filter(o -> biz.equals(s(o, "bizType"))).map(this::enrichOrder)
            .toList();
    }

    public long createOrder(Map<String, Object> b, Actor a, long id) {
        String biz = s(b, "bizType");
        require(Set.of("purchase", "sales").contains(biz), "单据类型非法");
        bizPerm(a, biz, id == 0 ? "create" : "update");
        var party = db.find(biz.equals("purchase") ? "inv_suppliers" : "inv_customers", n(b, "partyId"));
        require(n(party, "status") == 1, "往来单位已停用");
        require(date(b.get("orderDate")) != null, "请填写单据日期");
        var items = buildItems(rows(b.get("items")), biz);
        var data = com.kasinlo.erp.module.system.SystemService.pick(b, "bizType", "partyId", "orderNo",
            "orderDate", "dueDate", "remark", "salesman");
        data.put("partyName", party.get("name"));
        data.put("totalAmount", items.stream().map(it -> d(it, "amount")).reduce(BigDecimal.ZERO, BigDecimal::add));
        if (id == 0) {
            String no = s(b, "orderNo");
            if (no.isBlank()) no = db.number("inv_orders", "orderNo",(biz.equals("purchase") ? "CG" : "XS") + today()
                .replace("-", ""));
            db.unique("inv_orders", "orderNo", no, 0);
            data.put("orderNo", no);
            data.put("status", 1);
            data.put("createdBy", a.id());
            id = db.insert("inv_orders", data);
        } else {
            var o = db.lock("inv_orders", id);
            auth.visible(a, o);
            conflict(n(o, "status") == 1, "仅待处理单据可修改");
            require(biz.equals(s(o, "bizType")), "单据类型不可修改");
            data.remove("orderNo");
            db.update("inv_orders", id, data);
            db.deleteWhere("inv_order_items", "orderId", id);
        }
        for (var it : items) {
            it.put("orderId", id);
            db.insert("inv_order_items", it);
        }
        return id;
    }

    public long postStock(Map<String, Object> header, List<Map<String, Object>> items, Actor a) {
        String dir = s(header, "direction");
        require(Set.of("in", "out").contains(dir), "出入库方向非法");
        var data = new LinkedHashMap<>(header);
        String no = s(header, "orderNo");
        if (no.isBlank()) no = db.number("inv_stock_orders", "orderNo", "CK" + today().replace("-", ""));
        db.unique("inv_stock_orders", "orderNo", no, 0);
        data.put("orderNo", no);
        data.put("status", 1);
        data.put("createdBy", a.id());
        long id = db.insert("inv_stock_orders", data);
        for (var it : items) {
            var line = new LinkedHashMap<>(it);
            line.remove("id");
            line.put("stockOrderId", id);
            db.insert("inv_stock_items", line);
        }
        applyStock(dir, items);
        return id;
    }

    public void audit(long id, Map<String, Object> b, Actor a) {
        var o = db.lock("inv_orders", id);
        auth.visible(a, o);
        bizPerm(a, s(o, "bizType"), "finish");
        long status = n(b, "status");
        require(status >= 1 && status <= 3, "状态非法");
        conflict(n(o, "status") != 2, "已审核单据不可再次流转");
        if (status == n(o, "status")) return;
        if (status != 2) {
            conflict(n(o, "status") == 1 && status == 3 || n(o, "status") == 3 && status == 1, "状态流转非法");
            db.update("inv_orders", id, map("status", status));
            return;
        }
        conflict(n(o, "status") == 1, "仅待处理单据可以审核");
        var items = db.where("inv_order_items", "orderId", id);
        postStock(map("direction", s(o, "bizType").equals("purchase") ? "in" : "out", "sourceType", o.get("bizType"),
            "sourceId", id, "orderDate", o.get("orderDate"), "salesman", o.get("salesman")), items, a);
        db.update("inv_orders", id, map("status", 2));
        if (bool(b.get("settleNow")) && d(o, "totalAmount").signum() > 0) settleOrder(map("orderId", id,
            "amount", o.get("totalAmount"), "remark", "审核时直接结清"), a);
    }

    public void reverse(long id, Actor a) {
        var o = db.lock("inv_orders", id);
        auth.visible(a, o);
        bizPerm(a, s(o, "bizType"), "finish");
        conflict(n(o, "status") == 2, "仅已审核单据可以反审核");
        conflict(d(o, "settledAmount").signum() == 0, "已有收付款，不能反审核");
        for (var so : db.where("inv_stock_orders", "sourceId", id)) if (s(so, "sourceType").equals(s(o,
            "bizType")) && n(so, "status") == 1) {
            var locked = db.lock("inv_stock_orders", n(so, "id"));
            applyStock(s(locked, "direction").equals("in") ? "out" : "in", db.where("inv_stock_items",
                "stockOrderId", n(so, "id")));
            db.update("inv_stock_orders", n(so, "id"), map("status", 2));
        }
        db.update("inv_orders", id, map("status", 1));
    }

    public void settleOrder(Map<String, Object> b, Actor a) {
        var o = db.lock("inv_orders", n(b, "orderId"));
        auth.visible(a, o);
        conflict(n(o, "status") == 2, "仅已审核单据可收付款");
        BigDecimal amount = money(d(b, "amount"));
        require(amount.signum() > 0, "金额必须大于零");
        conflict(amount.add(d(o, "settledAmount")).compareTo(d(o, "totalAmount")) <= 0, "结算金额超出未结余额");
        var rec = new LinkedHashMap<>(b);
        rec.put("source", "order");
        rec.put("bizType", o.get("bizType"));
        rec.put("partyId", o.get("partyId"));
        rec.put("createdBy", a.id());
        rec.put("amount", amount);
        if (s(rec, "settledAt").isBlank()) rec.put("settledAt", today());
        if (s(rec, "method").isBlank()) rec.put("method", "银行转账");
        db.insert("inv_settlements", rec);
        db.update("inv_orders", n(o, "id"), map("settledAmount", d(o, "settledAmount").add(amount)));
    }

    public Map<String, Object> stock(long id, Actor a) {
        var o = db.find("inv_stock_orders", id);
        auth.visible(a, o);
        var items = db.where("inv_stock_items", "stockOrderId", id);
        for (var it : items) {
            try {
                it.put("productCode", db.find("inv_products", n(it, "productId")).get("code"));
                var base = db.where("inv_product_units", "specId", n(it, "specId")).stream().filter(u -> bool(u.get("isBase")))
                    .findFirst();
                it.put("unitBase", base.map(u -> s(u, "unit")).orElse(""));
            } catch (Exception e) {
                it.put("productCode", "");
                it.put("unitBase", "");
            }
        }
        o.put("items", items);
        o.put("itemCount", items.size());
        o.put("totalQty", items.stream().map(it -> d(it, "qtyBase")).reduce(BigDecimal.ZERO, BigDecimal::add));
        o.put("operator", operator(n(o, "createdBy")));
        String source = "";
        if (n(o, "sourceId") > 0) try {
            source = s(db.find("inv_orders", n(o, "sourceId")), "orderNo");
        } catch (Exception ignored) {
        }
        o.put("sourceOrderNo", source);
        return o;
    }

    public Object handle(String h, long id, Map<String, Object> b, Map<String, String> q, Actor a) {
        switch (h) {
            case "OrderList":
                return orders(a, q.getOrDefault("bizType", "sales"));
            case "OrderDetail":
                return order(id, a);
            case "OrderCreate":
            case "OrderUpdate":
                return map("id", createOrder(b, a, id));
            case "OrderChangeStatus":
                audit(id, b, a);
                return null;
            case "OrderReverse":
                reverse(id, a);
                return null;
            case "OrderDelete":
                var o = db.lock("inv_orders", id);
                auth.visible(a, o);
                bizPerm(a, s(o, "bizType"), "delete");
                conflict(n(o, "status") != 2, "已审核单据不可删除");
                db.deleteWhere("inv_order_items", "orderId", id);
                db.delete("inv_orders", id);
                return null;
            case "StockOrderList":
                return auth.scoped("inv_stock_orders", a).stream().map(r -> stock(n(r, "id"), a)).toList();
            case "StockOrderDetail":
                return stock(id, a);
            case "StockOrderCreate":
                var header = new LinkedHashMap<>(b);
                header.put("sourceType", "manual");
                header.put("sourceId", 0);
                return map("id", postStock(header, buildItems(rows(b.get("items")), "purchase"), a));
            case "StockOrderVoid":
                var so = db.lock("inv_stock_orders", id);
                auth.visible(a, so);
                conflict(n(so, "status") == 1, "出入库单已作废");
                conflict(s(so, "sourceType").equals("manual"), "关联采购销售单请使用反审核");
                applyStock(s(so, "direction").equals("in") ? "out" : "in", db.where("inv_stock_items",
                    "stockOrderId", id));
                db.update("inv_stock_orders", id, map("status", 2));
                return null;
            case "Settle":
                settleOrder(b, a);
                return null;
            case "SettleBatch":
                BigDecimal total = BigDecimal.ZERO;
                int count = 0;
                Set<Long> seen = new HashSet<>();
                require(b.get("orderIds") instanceof List<?>, "请选择单据");
                for (Object v :(List<?>) b.get("orderIds")) {
                    long oid = id(v);
                    if (!seen.add(oid)) continue;
                    var order = db.lock("inv_orders", oid);
                    BigDecimal amount = d(order, "totalAmount").subtract(d(order, "settledAmount"));
                    if (b.get("amounts") instanceof Map<?, ?> am && am.containsKey(Long.toString(oid))) amount = dec(am.get(Long.toString(oid)));
                    if (amount.signum() <= 0) continue;
                    var request = new LinkedHashMap<>(b);
                    request.put("orderId", oid);
                    request.put("amount", amount);
                    settleOrder(request, a);
                    total = total.add(amount);
                    count++;
                }
                return map("count", count, "amount", total);
            default:
                throw new BusinessException(501, "未实现业务操作：" + h);
        }
    }
}
