package com.kasinlo.erp.module.inv;

import com.kasinlo.erp.persistence.ErpRepository;
import com.kasinlo.erp.module.system.IdentityService;
import com.kasinlo.erp.module.system.IdentityService.Actor;
import com.kasinlo.erp.common.BusinessException;
import org.springframework.stereotype.Service;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.util.*;

import static com.kasinlo.erp.common.Values.*;
import static com.kasinlo.erp.common.BusinessException.*;

@Service
public class ContractService {
    final ErpRepository db;
    final IdentityService auth;
    final TradeService trade;

    public ContractService(ErpRepository db, IdentityService auth, TradeService trade) {
        this.db = db;
        this.auth = auth;
        this.trade = trade;
    }

    public boolean due(Map<String, Object> p, Map<String, Object> c) {
        return switch (s(p, "triggerNode")) {
            case "sign" -> true;
            case "accept" -> c.get("acceptedAt") != null;
            case "warranty_end" -> c.get("warrantyEndAt") != null && s(c, "warrantyEndAt").compareTo(today()) <= 0;
            default -> p.get("dueDate") != null && s(p, "dueDate").compareTo(today()) <= 0;
        };
    }

    public List<Map<String, Object>> plans(Map<String, Object> c) {
        var list = db.where("inv_contract_plans", "contractId", n(c, "id"));
        list.sort(Comparator.comparingLong(p -> n(p, "seq")));
        for (var p : list) {
            BigDecimal outstanding = money(d(p, "amount").subtract(d(p, "settledAmount")));
            p.put("outstanding", outstanding);
            String st = outstanding.signum() <= 0 ? "settled" : d(p, "settledAmount").signum() > 0 ? "partial" : due(p,
                c) ? "due" : "pending";
            p.put("status", st);
            p.put("statusLabel", switch (st) {
                case "settled" -> "已结清";
                case "partial" -> "部分收付";
                case "due" -> "待收付";
                default -> "未到期";
            });
            p.put("kindLabel", switch ((int) n(p, "kind")) {
                case 1 -> "预付款";
                case 2 -> "进度款";
                case 3 -> "验收款";
                case 4 -> "质保金";
                default -> "其他";
            });
        }
        return list;
    }

    public Map<String, Object> detail(long id, Actor a) {
        var c = db.find("inv_contracts", id);
        auth.visible(a, c);
        return enrich(c);
    }

    public Map<String, Object> enrich(Map<String, Object> c) {
        var ps = plans(c);
        c.put("plans", ps);
        c.put("planCount", ps.size());
        BigDecimal total = ps.stream().map(p -> d(p, "amount")).reduce(BigDecimal.ZERO, BigDecimal::add), settled =
            ps.stream().map(p -> d(p, "settledAmount")).reduce(BigDecimal.ZERO, BigDecimal::add);
        c.put("planAmount", total);
        c.put("settledAmount", settled);
        c.put("outstanding", total.subtract(settled));
        c.put("retentionOutstanding", ps.stream().filter(p -> n(p, "kind") == 4).map(p -> d(p, "outstanding")).reduce(BigDecimal.ZERO,
            BigDecimal::add));
        c.put("operator", trade.operator(n(c, "createdBy")));
        c.put("attachmentCount", db.where("inv_attachments", "bizId", n(c, "id")).stream().filter(r -> s(r,
            "bizType").equals("contract")).count());
        String stage = n(c, "status") == 0 ? "unaudited" : n(c, "status") == 3 ? "terminated" : n(c, "status") == 2 ?
            "finished" : c.get("acceptedAt") == null ? "pending" : c.get("warrantyEndAt") == null ? "accepted" : s(c,
            "warrantyEndAt").compareTo(today()) <= 0 ? "warranty_due" : "warranty";
        c.put("stage", stage);
        c.put("stageLabel", switch (stage) {
            case "unaudited" -> "待审核";
            case "terminated" -> "已终止";
            case "finished" -> "已完成";
            case "pending" -> "待验收";
            case "accepted" -> "已验收";
            case "warranty_due" -> "质保到期";
            default -> "质保中";
        });
        return c;
    }

    public long save(long id, Map<String, Object> b, Actor a) {
        require(!s(b, "name").isBlank(), "合同名称不能为空");
        String biz = s(b, "bizType");
        require(Set.of("purchase", "sales").contains(biz), "合同类型非法");
        var party = db.find(biz.equals("sales") ? "inv_customers" : "inv_suppliers", n(b, "partyId"));
        require(n(party, "status") == 1, "往来单位已停用");
        require(date(b.get("signDate")) != null, "签订日期不能为空");
        require(d(b, "amount").signum() >= 0, "合同金额不能为负数");
        if (!s(b, "startDate").isBlank() && !s(b, "endDate").isBlank()) require(s(b, "startDate").compareTo(s(b,
            "endDate")) <= 0, "履约结束日期不能早于开始日期");
        var data = com.kasinlo.erp.module.system.SystemService.pick(b, "contractNo", "name", "bizType",
            "partyId", "signDate", "startDate", "endDate", "amount", "salesman", "remark", "warrantyMonths",
            "retentionRate", "retentionAmount");
        data.put("partyName", party.get("name"));
        if (d(b, "retentionAmount").signum() == 0) data.put("retentionAmount", money(d(b, "amount").multiply(d(b,
            "retentionRate")).movePointLeft(2)));
        conflict(d(data, "retentionAmount").compareTo(d(b, "amount")) <= 0, "质保金不能超过合同金额");
        Map<String, Object> old = null;
        if (id > 0) {
            old = db.lock("inv_contracts", id);
            auth.visible(a, old);
            require(s(old, "bizType").equals(biz), "合同类型不可更改");
            conflict(n(old, "status") != 2 && n(old, "status") != 3, "已完成或终止合同不可修改");
            data.remove("contractNo");
            data.remove("status");
        } else {
            String no = s(b, "contractNo");
            if (no.isBlank()) no = db.number("inv_contracts", "contractNo", "HT");
            db.unique("inv_contracts", "contractNo", no, 0);
            data.put("contractNo", no);
            data.put("status", 0);
            data.put("createdBy", a.id());
        }
        List<Map<String, Object>> ps = rows(b.get("plans"));
        require(ps.size() <= 20, "付款计划最多 20 期");
        Set<Long> seq = new HashSet<>(), kept = new HashSet<>();
        BigDecimal total = BigDecimal.ZERO;
        for (var p : ps) {
            require(n(p, "seq") > 0 && n(p, "seq") <= 99 && seq.add(n(p, "seq")), "期次号非法或重复");
            require(!s(p, "name").isBlank(), "期次名称不能为空");
            BigDecimal amount = d(p, "amount");
            if (amount.signum() == 0 && d(p, "rate").signum() > 0) amount = money(d(b, "amount").multiply(d(p,
                "rate")).movePointLeft(2));
            p.put("amount", amount);
            require(amount.signum() >= 0, "期次金额不能为负数");
            total = total.add(amount);
            if (n(p, "id") > 0) {
                require(id > 0, "新增合同不能引用既有期次");
                var oldp = db.find("inv_contract_plans", n(p, "id"));
                require(n(oldp, "contractId") == id, "期次不属于当前合同");
                if (d(oldp, "settledAmount").signum() > 0) conflict(amount.compareTo(d(oldp, "amount")) == 0 && d(p,
                    "rate").compareTo(d(oldp, "rate")) == 0, "已收付款期次不能修改金额和比例");
                kept.add(n(p, "id"));
            }
        }
        conflict(total.compareTo(d(b, "amount")) <= 0, "计划金额合计超过合同金额");
        if (old != null) for (var p : db.where("inv_contract_plans", "contractId", id)) if (!kept.contains(n(p,
            "id"))) {
            conflict(d(p, "settledAmount").signum() == 0, "已收付款期次不能删除");
            db.delete("inv_contract_plans", n(p, "id"));
        }
        if (id == 0) id = db.insert("inv_contracts", data);
        else db.update("inv_contracts", id, data);
        for (var p : ps) {
            var pd = new LinkedHashMap<>(p);
            pd.put("contractId", id);
            pd.put("bizType", biz);
            pd.put("partyId", b.get("partyId"));
            pd.put("partyName", party.get("name"));
            pd.put("createdBy", a.id());
            pd.putIfAbsent("kind", 5);
            if (n(p, "id") == 0) db.insert("inv_contract_plans", pd);
            else db.update("inv_contract_plans", n(p, "id"), pd);
        }
        return id;
    }

    public void settlePlan(Map<String, Object> b, Actor a) {
        long pid = n(b, "planId");
        var peek = db.find("inv_contract_plans", pid);
        var c = db.lock("inv_contracts", n(peek, "contractId"));
        auth.visible(a, c);
        var p = db.lock("inv_contract_plans", pid);
        conflict(n(c, "status") == 1, "只有已审核执行中合同可收付款");
        BigDecimal amount = money(d(b, "amount"));
        require(amount.signum() > 0, "收付金额必须大于零");
        conflict(amount.add(d(p, "settledAmount")).compareTo(d(p, "amount")) <= 0, "收付金额超过期次未结余额");
        conflict(due(p, c) || s(p, "triggerNode").isBlank(), "期次触发条件尚未满足");
        var rec = new LinkedHashMap<>(b);
        rec.put("source", "plan");
        rec.put("amount", amount);
        rec.put("bizType", c.get("bizType"));
        rec.put("partyId", c.get("partyId"));
        rec.put("createdBy", a.id());
        if (s(rec, "settledAt").isBlank()) rec.put("settledAt", today());
        if (s(rec, "method").isBlank()) rec.put("method", "银行转账");
        db.insert("inv_settlements", rec);
        db.update("inv_contract_plans", pid, map("settledAmount", d(p, "settledAmount").add(amount)));
    }

    public Object handle(String h, long id, Map<String, Object> b, Actor a) {
        switch (h) {
            case "ContractList":
                return auth.scoped("inv_contracts", a).stream().map(this::enrich).toList();
            case "ContractDetail":
                return detail(id, a);
            case "ContractCreate":
            case "ContractUpdate":
                return map("id", save(id, b, a));
            case "ContractPlanSettle":
                settlePlan(b, a);
                return null;
            case "ContractPlanSettles":
                var p = db.find("inv_contract_plans", id);
                auth.visible(a, db.find("inv_contracts", n(p, "contractId")));
                return db.where("inv_settlements", "planId", id);
            case "ContractLogs":
                detail(id, a);
                return db.all("sys_operation_logs").stream().filter(l -> s(l, "path").contains("/contracts/" + id))
                    .toList();
            default:
                break;
        }
        var c = db.lock("inv_contracts", id);
        auth.visible(a, c);
        switch (h) {
            case "ContractAudit":
                conflict(n(c, "status") == 0, "合同已审核");
                db.update("inv_contracts", id, map("status", 1, "auditedAt", now(), "auditor", trade.operator(a.id())));
                break;
            case "ContractUnaudit":
                conflict(n(c, "status") == 1, "仅执行中合同可以反审核");
                conflict(plans(c).stream().allMatch(p -> d(p, "settledAmount").signum() == 0) && c.get("acceptedAt") ==
                    null, "已有验收或收付款，不能反审核");
                db.update("inv_contracts", id, map("status", 0, "auditedAt", null, "auditor", ""));
                break;
            case "ContractAccept":
                conflict(n(c, "status") == 1, "仅已审核执行中合同可验收");
                require(date(b.get("acceptedAt")) != null, "验收日期不能为空");
                require(Set.of("合格", "不合格", "整改").contains(s(b, "acceptResult")), "验收结果非法");
                long months = n(b, "warrantyMonths");
                if (!b.containsKey("warrantyMonths")) months = n(c, "warrantyMonths");
                require(months >= 0 && months <= 120, "质保期非法");
                String end = months > 0 ? LocalDate.parse(date(b.get("acceptedAt"))).plusMonths(months).toString() : null;
                db.update("inv_contracts", id, map("acceptedAt", b.get("acceptedAt"), "acceptResult",
                    b.get("acceptResult"), "warrantyMonths", months, "warrantyEndAt", end));
                break;
            case "ContractDelete":
                conflict(n(c, "status") == 0, "仅待审核合同可删除");
                conflict(plans(c).stream().allMatch(p -> d(p, "settledAmount").signum() == 0), "合同已有收付款");
                db.deleteWhere("inv_contract_plans", "contractId", id);
                db.delete("inv_contracts", id);
                break;
            default:
                throw new BusinessException(501, "未实现合同操作：" + h);
        }
        return null;
    }

    public List<Map<String, Object>> finance(String biz, Actor a) {
        var out = new ArrayList<Map<String, Object>>();
        for (var o : auth.scoped("inv_orders", a)) if (biz.equals(s(o, "bizType")) && n(o, "status") == 2) out.add(map("source",
            "order", "orderId", o.get("id"), "orderNo", o.get("orderNo"), "contractId", 0, "contractNo",
            "", "planId", 0, "planSeq", 0, "planName", "", "partyId", o.get("partyId"), "partyName", o.get("partyName"),
            "orderDate", o.get("orderDate"), "dueDate", o.get("dueDate"), "totalAmount", o.get("totalAmount"),
            "settledAmount", o.get("settledAmount")));
        for (var c : auth.scoped("inv_contracts", a)) if (biz.equals(s(c, "bizType")) && n(c, "status") == 1) for (var p : plans(c)) out.add(map("source",
            "plan", "orderId", 0, "orderNo", c.get("contractNo"), "contractId", c.get("id"), "contractNo",
            c.get("contractNo"), "planId", p.get("id"), "planSeq", p.get("seq"), "planName", p.get("name"),
            "partyId", c.get("partyId"), "partyName", c.get("partyName"), "orderDate", c.get("signDate"),
            "dueDate", p.get("dueDate"), "totalAmount", p.get("amount"), "settledAmount", p.get("settledAmount")));
        for (var r : out) {
            r.put("outstanding", money(d(r, "totalAmount").subtract(d(r, "settledAmount"))));
            r.put("settleStatus", settle(d(r, "totalAmount"), d(r, "settledAmount")));
            r.put("overdue", r.get("dueDate") != null && s(r, "dueDate").compareTo(today()) < 0 && d(r,
                "outstanding").signum() > 0);
        }
        return out;
    }
}
