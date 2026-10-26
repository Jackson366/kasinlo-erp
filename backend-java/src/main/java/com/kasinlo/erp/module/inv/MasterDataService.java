package com.kasinlo.erp.module.inv;

import com.kasinlo.erp.persistence.ErpRepository;
import com.kasinlo.erp.common.BusinessException;
import org.springframework.stereotype.Service;
import java.math.BigDecimal;
import java.util.*;

import static com.kasinlo.erp.common.Values.*;
import static com.kasinlo.erp.common.BusinessException.*;

@Service
public class MasterDataService {
    private final ErpRepository db;

    public MasterDataService(ErpRepository db) {
        this.db = db;
    }

    public Map<String, Object> product(long id) {
        var p = db.find("inv_products", id);
        var specs = db.where("inv_product_specs", "productId", id);
        BigDecimal total = BigDecimal.ZERO;
        boolean warn = false;
        int units = 0;
        for (var sp : specs) {
            var us = db.where("inv_product_units", "specId", n(sp, "id"));
            sp.put("units", us);
            sp.put("warn", d(sp, "stock").compareTo(d(sp, "warnStock")) < 0);
            units += us.size();
            total = total.add(d(sp, "stock"));
            warn |= bool(sp.get("warn"));
        }
        p.put("specs", specs);
        p.put("specCount", specs.size());
        p.put("unitCount", units);
        p.put("totalStock", total);
        p.put("warn", warn);
        try {
            p.put("categoryName", db.find("inv_categories", n(p, "categoryId")).get("name"));
        } catch (Exception e) {
            p.put("categoryName", "");
        }
        return p;
    }

    public List<Map<String, Object>> products() {
        return db.all("inv_products").stream().map(p -> product(n(p, "id"))).toList();
    }

    public List<Map<String, Object>> categories() {
        var list = db.all("inv_categories");
        for (var c : list) {
            c.put("productCount", db.where("inv_products", "categoryId", n(c, "id")).size());
            c.put("childCount", db.where("inv_categories", "parentId", n(c, "id")).size());
        }
        return list;
    }

    public Object handle(String h, long id, Map<String, Object> b, Map<String, String> q) {
        if (h.startsWith("Product")) {
            if (h.equals("ProductList")) return products();
            if (h.equals("ProductPick")) return map("items", products().stream().filter(r -> n(r, "status") == 1)
                .toList(), "truncated", false);
            if (h.equals("ProductDetail")) return product(id);
            if (h.equals("ProductDuplicates")) return products().stream().filter(r -> s(r, "name").contains(q.getOrDefault("name",
                "")) && n(r, "id") != id(q.get("excludeId"))).map(r -> {
                r.put("specSummary", rows(r.get("specs")).stream().map(sp -> s(sp, "spec")).reduce((x,
                    y) -> x + "、" + y).orElse(""));
                return r;
            }).toList();
            if (h.equals("ProductDelete")) {
                conflict(db.where("inv_order_items", "productId", id).isEmpty() && db.where("inv_stock_items",
                    "productId", id).isEmpty(), "商品已有业务引用，不可删除");
                for (var sp : db.where("inv_product_specs", "productId", id)) {
                    conflict(d(sp, "stock").signum() == 0, "商品仍有库存");
                    db.deleteWhere("inv_product_units", "specId", n(sp, "id"));
                    db.delete("inv_product_specs", n(sp, "id"));
                }
                db.delete("inv_products", id);
                return null;
            }
            require(!s(b, "name").isBlank(), "商品名称不能为空");
            var data = new LinkedHashMap<>(b);
            String code = s(b, "code");
            if (code.isBlank()) code = id > 0 ? s(db.find("inv_products", id), "code") : db.number("inv_products",
                "code", "SP");
            db.unique("inv_products", "code", code, id);
            data.put("code", code);
            if (n(b, "categoryId") > 0) db.find("inv_categories", n(b, "categoryId"));
            if (id == 0) id = db.insert("inv_products", data);
            else db.update("inv_products", id, data);
            List<Map<String, Object>> specs = rows(b.get("specs"));
            if (specs.isEmpty()) specs = List.of(map("spec", "默认", "units", List.of(map("unit", "个", "rate",
                1, "isBase", true))));
            Set<Long> kept = new HashSet<>();
            Set<String> names = new HashSet<>();
            for (var sp : specs) {
                String name = s(sp, "spec");
                if (name.isBlank()) name = "默认";
                require(names.add(name), "规格名称重复");
                long sid = n(sp, "id");
                if (sid > 0) require(n(db.find("inv_product_specs", sid), "productId") == id, "规格不属于当前商品");
                var sd = map("productId", id, "spec", name, "warnStock", d(sp, "warnStock"), "status",
                    sp.getOrDefault("status", 1));
                if (sid == 0) {
                    var old = db.where("inv_product_specs", "productId", id).stream().filter(r -> s(r,
                        "spec").equals(s(sp, "spec"))).findFirst();
                    if (old.isPresent()) sid = n(old.get(), "id");
                }
                if (sid == 0) sid = db.insert("inv_product_specs", sd);
                else db.update("inv_product_specs", sid, sd);
                kept.add(sid);
                List<Map<String, Object>> us = rows(sp.get("units"));
                if (us.isEmpty()) us = List.of(map("unit", "个", "rate", 1, "isBase", true));
                Set<String> unames = new HashSet<>();
                Set<Long> ukeep = new HashSet<>();
                int bases = 0;
                for (int i = 0; i < us.size(); i++) {
                    var u = us.get(i);
                    String unit = s(u, "unit");
                    if (unit.isBlank()) unit = "个";
                    require(unames.add(unit), "单位名称重复");
                    boolean base = bool(u.get("isBase")) ||(us.size() == 1);
                    if (base) bases++;
                    BigDecimal rate = base ? BigDecimal.ONE : d(u, "rate");
                    if (rate.signum() == 0) rate = BigDecimal.ONE;
                    require(rate.signum() > 0 && d(u, "purchasePrice").signum() >= 0 && d(u, "salePrice").signum() >=
                        0, "换算率或价格非法");
                    long uid = 0;
                    for (var old : db.where("inv_product_units", "specId", sid)) if (s(old, "unit").equals(unit)) uid =
                        n(old, "id");
                    var ud = map("specId", sid, "unit", unit, "rate", rate, "isBase", base, "purchasePrice",
                        d(u, "purchasePrice"), "salePrice", d(u, "salePrice"), "sort", i, "status", u.getOrDefault("status",
                        1));
                    if (uid == 0) uid = db.insert("inv_product_units", ud);
                    else db.update("inv_product_units", uid, ud);
                    ukeep.add(uid);
                }
                require(bases == 1, "每个规格必须有且只有一个基本单位");
                for (var old : db.where("inv_product_units", "specId", sid)) if (!ukeep.contains(n(old,
                    "id"))) {
                    boolean used = !db.where("inv_order_items", "unitId", n(old, "id")).isEmpty() || !db.where("inv_stock_items",
                        "unitId", n(old, "id")).isEmpty();
                    if (used) db.update("inv_product_units", n(old, "id"), map("status", 0));
                    else db.delete("inv_product_units", n(old, "id"));
                }
            }
            for (var old : db.where("inv_product_specs", "productId", id)) if (!kept.contains(n(old, "id"))) {
                if (d(old, "stock").signum() != 0 || !db.where("inv_order_items", "specId", n(old, "id")).isEmpty() ||
                    !db.where("inv_stock_items", "specId", n(old, "id")).isEmpty()) db.update("inv_product_specs",
                    n(old, "id"), map("status", 0));
                else {
                    db.deleteWhere("inv_product_units", "specId", n(old, "id"));
                    db.delete("inv_product_specs", n(old, "id"));
                }
            }
            return map("id", id);
        }
        String table = h.startsWith("Category") ? "inv_categories" : h.startsWith("Customer") ? "inv_customers" : h.startsWith("Supplier") ?
            "inv_suppliers" : "inv_print_templates";
        if (h.endsWith("List")) {
            if (h.startsWith("Category")) return categories();
            var list = db.all(table);
            if (h.startsWith("Customer") || h.startsWith("Supplier")) for (var r : list) {
                String biz = h.startsWith("Customer") ? "sales" : "purchase";
                var orders = db.where("inv_orders", "partyId", n(r, "id")).stream().filter(o -> biz.equals(s(o,
                    "bizType")) && n(o, "status") == 2).toList();
                r.put("orderCount", orders.size());
                r.put("balance", orders.stream().map(o -> d(o, "totalAmount").subtract(d(o, "settledAmount"))).reduce(BigDecimal.ZERO,
                    BigDecimal::add));
            }
            return list;
        }
        if (h.equals("CategoryOptions")) return categories().stream().filter(r -> n(r, "status") == 1).toList();
        if (h.endsWith("Delete")) {
            if (h.startsWith("Category")) conflict(db.where("inv_products", "categoryId", id).isEmpty() && db.where(table,
                "parentId", id).isEmpty(), "分类仍被引用");
            if (h.startsWith("Customer") || h.startsWith("Supplier")) {
                String biz = h.startsWith("Customer") ? "sales" : "purchase";
                conflict(db.where("inv_orders", "partyId", id).stream().noneMatch(o -> biz.equals(s(o,
                    "bizType"))) && db.where("inv_contracts", "partyId", id).stream().noneMatch(o -> biz.equals(s(o,
                    "bizType"))), "往来单位已有业务引用");
            }
            db.delete(table, id);
            return null;
        }
        require(!s(b, "name").isBlank(), "名称不能为空");
        var data = new LinkedHashMap<>(b);
        if (h.startsWith("Category")) {
            long p = n(b, "parentId");
            Set<Long> seen = new HashSet<>();
            while (p > 0) {
                conflict(p != id && seen.add(p), "分类上级不能选择自身或子级");
                p = n(db.find(table, p), "parentId");
            }
            for (var c : db.where(table, "parentId", n(b, "parentId"))) conflict(n(c, "id") == id || !s(c,
                "name").equals(s(b, "name")), "同级分类名称重复");
        } else {
            String code = s(b, "code");
            if (code.isBlank()) code = id > 0 ? s(db.find(table, id), "code") : db.number(table, "code",
                h.startsWith("Customer") ? "KH" : h.startsWith("Supplier") ? "GYS" : "DY");
            db.unique(table, "code", code, id);
            data.put("code", code);
        }
        if (h.startsWith("PrintTemplate")) {
            String content = s(b, "content");
            require(content.isBlank() || content.startsWith("{"), "打印模板配置必须为 JSON 对象");
            data.putIfAbsent("paper", "A4");
        }
        if (id == 0) id = db.insert(table, data);
        else db.update(table, id, data);
        return map("id", id);
    }

    public List<Map<String, Object>> inventory() {
        var result = new ArrayList<Map<String, Object>>();
        for (var p : products()) for (var sp : rows(p.get("specs"))) {
            var base = rows(sp.get("units")).stream().filter(u -> bool(u.get("isBase"))).findFirst().orElse(map());
            result.add(map("id", sp.get("id"), "productId", p.get("id"), "code", p.get("code"), "name",
                p.get("name"), "productCode", p.get("code"), "productName", p.get("name"), "categoryId",
                p.get("categoryId"), "categoryName", p.get("categoryName"), "specId", sp.get("id"), "spec",
                sp.get("spec"), "unit", base.getOrDefault("unit", "个"), "stock", sp.get("stock"), "warnStock",
                sp.get("warnStock"), "warn", sp.get("warn"), "purchasePrice", base.getOrDefault("purchasePrice",
                0), "status", sp.get("status")));
        }
        return result;
    }
}
