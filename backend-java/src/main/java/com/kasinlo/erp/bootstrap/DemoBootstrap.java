package com.kasinlo.erp.bootstrap;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.kasinlo.erp.persistence.ErpRepository;
import com.kasinlo.erp.module.system.IdentityService;
import com.kasinlo.erp.module.inv.*;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.core.io.ClassPathResource;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Transactional;
import java.util.*;

import static com.kasinlo.erp.common.Values.*;

@Component
public class DemoBootstrap implements ApplicationRunner {
    final ErpRepository db;
    final IdentityService auth;
    final ObjectMapper json;
    final MasterDataService master;
    final TradeService trade;
    final ContractService contracts;
    final boolean demo;

    public DemoBootstrap(ErpRepository db, IdentityService auth, ObjectMapper json, MasterDataService master,
        TradeService trade, ContractService contracts,
    @Value("${erp.demo-data:true}")
    boolean demo) {
        this.db = db;
        this.auth = auth;
        this.json = json;
        this.master = master;
        this.trade = trade;
        this.contracts = contracts;
        this.demo = demo;
    }

    @Override
    @Transactional
    public void run(ApplicationArguments args) throws Exception {
        if (!db.all("sys_users").isEmpty()) return;
        long company = db.insert("sys_depts", map("name", "总公司", "parentId", 0, "status", 1));
        long sales = db.insert("sys_depts", map("name", "销售部", "parentId", company, "status", 1));
        long tech = db.insert("sys_depts", map("name", "采购部", "parentId", company, "status", 1));
        List<Map<String, Object>> menuDefs = json.readValue(new ClassPathResource("menus.json").getInputStream(),
            new TypeReference<>() {
        });
        Map<Long, Long> ids = new HashMap<>();
        for (var m : menuDefs) {
            long key = n(m, "id");
            var copy = new LinkedHashMap<>(m);
            copy.put("parentId", 0);
            ids.put(key, db.insert("sys_menus", copy));
        }
        for (var m : menuDefs) if (n(m, "parentId") > 0) db.update("sys_menus", ids.get(n(m, "id")), map("parentId",
            ids.get(n(m, "parentId"))));
        long adminRole = db.insert("sys_roles", map("name", "超级管理员", "code", "super_admin", "status",
            1, "dataScope", 1, "isBuiltin", true));
        long viewer = db.insert("sys_roles", map("name", "只读访客", "code", "viewer", "status", 1, "dataScope",
            1, "isBuiltin", true));
        long manager = db.insert("sys_roles", map("name", "业务管理员", "code", "manager", "status", 1, "dataScope",
            1, "isBuiltin", true));
        for (var m : db.all("sys_menus")) {
            long mid = n(m, "id");
            db.insert("sys_role_menus", map("roleId", adminRole, "menuId", mid));
            db.insert("sys_role_menus", map("roleId", manager, "menuId", mid));
            String perm = s(m, "permission");
            if (!"button".equals(s(m, "type")) || perm.endsWith(":list") || perm.endsWith(":view")) db.insert("sys_role_menus",
                map("roleId", viewer, "menuId", mid));
        }
        long admin = db.insert("sys_users", map("username", "admin", "nickname", "管理员", "password", auth.passwords.encode("admin123456"),
            "deptId", company, "status", 1));
        db.insert("sys_user_roles", map("userId", admin, "roleId", adminRole));
        if (demo) {
            long user = db.insert("sys_users", map("username", "demo", "nickname", "只读演示", "password",
                auth.passwords.encode("demo123456"), "deptId", sales, "status", 1));
            db.insert("sys_user_roles", map("userId", user, "roleId", viewer));
        }
        for (String type : List.of("purchase", "sales", "stock_in", "stock_out", "inventory", "report")) db.insert("inv_print_templates",
            map("code", type + "_default", "name", switch (type) {
            case "purchase" -> "采购单默认模板";
            case "sales" -> "销售单默认模板";
            case "stock_in" -> "入库单默认模板";
            case "stock_out" -> "出库单默认模板";
            case "inventory" -> "库存清单默认模板";
            default -> "报表默认模板";
        }, "docType", type, "paper", "A4", "content", "{}", "status", 1));
        if (!demo) return;
        var actor = auth.actor(admin);
        long cat = id(((Map<?, ?>) master.handle("CategoryCreate", 0, map("name", "工业用品", "parentId",
            0), Map.of())).get("id"));
        long customer = id(((Map<?, ?>) master.handle("CustomerCreate", 0, map("name", "华东机械有限公司", "contact",
            "王经理", "phone", "13800000001"), Map.of())).get("id"));
        long supplier = id(((Map<?, ?>) master.handle("SupplierCreate", 0, map("name", "恒远工业供应商", "contact",
            "李经理", "phone", "13800000002"), Map.of())).get("id"));
        String[] names = {
            "不锈钢螺栓", "工业轴承", "耐压管材", "密封垫片", "电气开关"
        };
        for (int i = 0; i < names.length; i++) master.handle("ProductCreate", 0, map("name", names[i],
            "categoryId", cat, "refPurchasePrice", 10 + i * 5, "refSalePrice", 15 + i * 6, "specs", List.of(map("spec",
            i == 0 ? "M12×40" : "标准型", "warnStock", 20, "units", List.of(map("unit", "个", "isBase", true,
            "rate", 1, "purchasePrice", 10 + i * 5, "salePrice", 15 + i * 6), map("unit", "箱", "isBase",
            false, "rate", 10, "purchasePrice", 100 + i * 50, "salePrice", 150 + i * 60))))), Map.of());
        var products = master.products();
        for (var p : products) {
            var sp = rows(p.get("specs")).get(0);
            var u = rows(sp.get("units")).get(0);
            long oid = trade.createOrder(map("bizType", "purchase", "partyId", supplier, "orderDate",
                today(), "salesman", "李经理", "remark", "Java 评审示例：采购审核入库", "items", List.of(map("productId",
                p.get("id"), "specId", sp.get("id"), "unitId", u.get("id"), "qty", 100, "price", u.get("purchasePrice")))),
                actor, 0);
            trade.audit(oid, map("status", 2), actor);
        }
        var p = products.get(0);
        var sp = rows(p.get("specs")).get(0);
        var u = rows(sp.get("units")).get(0);
        long sale = trade.createOrder(map("bizType", "sales", "partyId", customer, "orderDate", today(),
            "dueDate", today(), "salesman", "王经理", "items", List.of(map("productId", p.get("id"), "specId",
            sp.get("id"), "unitId", u.get("id"), "qty", 10, "price", 15))), actor, 0);
        trade.audit(sale, map("status", 2), actor);
        trade.settleOrder(map("orderId", sale, "amount", 50, "method", "银行转账"), actor);
        long contract = contracts.save(0, map("name", "年度工业用品供货合同", "bizType", "sales", "partyId", customer,
            "signDate", today(), "amount", 10000, "warrantyMonths", 12, "retentionRate", 10, "plans",
            List.of(map("seq", 1, "name", "预付款", "kind", 1, "rate", 30, "triggerNode", "sign"), map("seq",
            2, "name", "验收款", "kind", 3, "rate", 60, "triggerNode", "accept"), map("seq", 3, "name", "质保金",
            "kind", 4, "rate", 10, "triggerNode", "warranty_end"))), actor);
        contracts.handle("ContractAudit", contract, map(), actor);
    }
}
