package com.kasinlo.erp.web;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.kasinlo.erp.persistence.ErpRepository;
import com.kasinlo.erp.module.system.*;
import com.kasinlo.erp.module.system.IdentityService.Actor;
import com.kasinlo.erp.module.inv.*;
import com.kasinlo.erp.module.ops.OperationsService;
import com.kasinlo.erp.common.BusinessException;
import org.springframework.core.io.ClassPathResource;
import org.springframework.http.ResponseEntity;
import org.springframework.stereotype.Controller;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.multipart.MultipartHttpServletRequest;
import org.springframework.web.multipart.MultipartFile;
import org.springframework.transaction.annotation.Transactional;
import jakarta.servlet.http.HttpServletRequest;
import java.util.*;
import java.util.regex.*;
import java.security.MessageDigest;
import java.nio.charset.StandardCharsets;
import java.math.BigDecimal;

import static com.kasinlo.erp.common.Values.*;
import static com.kasinlo.erp.common.BusinessException.*;

@RestController
public class ApiController {
    record Route(String method, String path, String module, String handler, List<String> permissions) {
    }

    final org.springframework.boot.availability.ApplicationAvailability availability;
    final ObjectMapper json;
    final ErpRepository db;
    final IdentityService auth;
    final SystemService system;
    final MasterDataService master;
    final TradeService trade;
    final ContractService contracts;
    final ReportingService reports;
    final FileService files;
    final OperationsService ops;
    final List<Route> routes;

    public ApiController(ObjectMapper json, ErpRepository db, IdentityService auth, SystemService system,
        MasterDataService master, TradeService trade, ContractService contracts, ReportingService reports,
        FileService files, OperationsService ops, org.springframework.boot.availability.ApplicationAvailability availability) throws Exception {
        this.availability = availability;
        this.json = json;
        this.db = db;
        this.auth = auth;
        this.system = system;
        this.master = master;
        this.trade = trade;
        this.contracts = contracts;
        this.reports = reports;
        this.files = files;
        this.ops = ops;
        routes = json.readValue(new ClassPathResource("routes.json").getInputStream(), new TypeReference<>() {
        });
    }

    @GetMapping("/health")
    public Object health() {
        if (availability.getReadinessState() != org.springframework.boot.availability.ReadinessState.ACCEPTING_TRAFFIC) return ResponseEntity.status(503)
            .body(map("status", "starting"));
        db.ping();
        return map("status", "ok", "runtime", "java", "version", "2.0.0-java");
    }

    @GetMapping("/api/v1/migration/coverage")
    public Object coverage(HttpServletRequest req) {
        auth.require(auth.authenticate(req), "ops:list");
        return ok(map("runtime", "java", "routes", routes.size(), "contracts", routes, "knownDifferences",
            List.of("Go binary upgrade packages are replaced by JAR deployment", "Restore-to-new-database uses an independent Java process")));
    }

    @GetMapping("/swagger/doc.json")
    public ResponseEntity<byte[]> swagger() throws Exception {
        return ResponseEntity.ok().header("Content-Type", "application/json").body(new ClassPathResource("legacy-swagger.json")
            .getContentAsByteArray());
    }

    @RequestMapping("/api/v1/**")
    @Transactional
    public Object api(HttpServletRequest req) throws Exception {
        String uri = req.getRequestURI();
        Route route = null;
        long rid = 0;
        for (var r : routes) {
            if (!r.method().equals(req.getMethod())) continue;
            Matcher matcher = Pattern.compile("^" + r.path().replace(":id", "([0-9]+)") + "$").matcher(uri);
            if (matcher.matches()) {
                route = r;
                if (matcher.groupCount() > 0) rid = Long.parseLong(matcher.group(1));
                break;
            }
        }
        if (route == null) throw new BusinessException(404, "接口不存在");
        Map<String, String> q = new LinkedHashMap<>();
        req.getParameterMap().forEach((k, v) -> q.put(k, v.length > 0 ? v[0] : ""));
        Map<String, Object> b = new LinkedHashMap<>();
        MultipartFile file = null;
        if (req instanceof MultipartHttpServletRequest multi) {
            file = multi.getFile("file");
            b.putAll(q);
        } else if (req.getContentType() != null && req.getContentType().contains("application/json") && req.getContentLengthLong() !=
            0) {
            byte[] bytes = req.getInputStream().readAllBytes();
            if (bytes.length > 0) b = json.readValue(bytes, new TypeReference<>() {
            });
        }
        String h = route.handler(), module = route.module();
        Actor actor = null;
        if (module.equals("auth") && h.equals("Login")) return ok(auth.login(b, req));
        if (module.equals("auth") && h.equals("Refresh")) return ok(auth.refresh(b));
        actor = auth.authenticate(req);
        if (!route.permissions().isEmpty()) auth.require(actor, route.permissions().toArray(String[]::new));
        String idem = req.getHeader("Idempotency-Key");
        String requestKey = null;
        if (idem != null && !idem.isBlank() && !req.getMethod().equals("GET") && file == null) {
            require(idem.length() <= 128, "幂等键过长");
            requestKey = actor.id() + ":" + idem;
            String fingerprint = HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest((req.getMethod() +
                uri + json.writeValueAsString(b)).getBytes(StandardCharsets.UTF_8)));
            var existing = db.lockRequest(requestKey);
            if (existing != null) {
                conflict(str(existing.get("fingerprint")).equals(fingerprint), "幂等键已被不同请求使用");
                String response = str(existing.get("response"));
                conflict(!response.isBlank(), "请求正在处理中");
                return json.readValue(response, Object.class);
            }
            db.createRequest(requestKey, actor.id(), fingerprint);
        }
        Object data;
        if (module.equals("auth")) {
            auth.revoke(actor.id());
            data = null;
        } else if (module.equals("ops")) data = ops.handle(h, b, q);
        else if (h.contains("Export") || h.equals("ImportTemplate")) {
            data = export(module, h, q, actor);
        } else if (module.equals("user") && h.equals("Import")) data = files.importUsers(file, "1".equals(q.get("dryRun")) ||
            "true".equals(q.get("dryRun")), actor);
        else if (!module.equals("inv")) data = system.handle(module, h, rid, b, q, actor);
        else if (h.startsWith("Attachment")) data = files.attachment(h, rid, q, file, actor);
        else if (h.equals("Inventory")) data = master.inventory();
        else if (h.equals("Overview") || h.startsWith("Report")) data = reports.report(h, q, actor);
        else if (h.equals("Receivables") || h.equals("Payables")) {
            var list = filter(contracts.finance(h.equals("Receivables") ? "sales" : "purchase", actor),
                q);
            var summary = map("total", sum(list, "totalAmount"), "settled", sum(list, "settledAmount"),
                "outstanding", sum(list, "outstanding"), "overdue", sum(list.stream().filter(r -> bool(r.get("overdue")))
                .toList(), "outstanding"), "orderTotal", sum(list.stream().filter(r -> s(r, "source").equals("order"))
                .toList(), "totalAmount"), "planTotal", sum(list.stream().filter(r -> s(r, "source").equals("plan"))
                .toList(), "totalAmount"));
            var page = page(list, q);
            page.put("summary", summary);
            data = page;
        } else if (h.startsWith("Contract")) data = contracts.handle(h, rid, b, actor);
        else if (h.startsWith("Order") || h.startsWith("StockOrder") || h.startsWith("Settle")) data = trade.handle(h,
            rid, b, q, actor);
        else data = master.handle(h, rid, b, q);
        if (data instanceof ResponseEntity<?>) return data;
        if (data instanceof List<?> listValue &&(listValue.isEmpty() || listValue.get(0) instanceof Map<?,
            ?>)) {
            List<Map<String, Object>> list = rows(data);
            list = filter(list, q);
            boolean tree = module.equals("menu") || module.equals("dept");
            if (tree && h.equals("List")) data = IdentityService.tree(list, 0);
            else if (h.endsWith("List") && !h.equals("AttachmentList") || h.equals("Inventory") || module.equals("log") &&
                !h.equals("Modules")) data = page(list, q);
            else data = list;
        }
        var response = ok(data);
        if (requestKey != null) db.finishRequest(requestKey, json.writeValueAsString(response));
        if (!req.getMethod().equals("GET") && !module.equals("profile")) db.insert("sys_operation_logs",
            map("userId", actor.id(), "username", s(actor.user(), "username"), "module", module, "action",
            h, "method", req.getMethod(), "path", uri, "ip", req.getRemoteAddr(), "request", redact(b),
            "status", 200, "userAgent", str(req.getHeader("User-Agent"))));
        return response;
    }

    String redact(Map<String, Object> b) throws Exception {
        var copy = new LinkedHashMap<>(b);
        for (String k : List.of("password", "oldPassword", "newPassword", "refreshToken")) if (copy.containsKey(k)) copy.put(k,
            "***");
        return json.writeValueAsString(copy);
    }

    Map<String, Object> ok(Object data) {
        return map("code", 0, "message", "success", "data", data, "traceId", UUID.randomUUID().toString());
    }

    BigDecimal sum(List<Map<String, Object>> list, String key) {
        return list.stream().map(r -> d(r, key)).reduce(BigDecimal.ZERO, BigDecimal::add);
    }

    public static List<Map<String, Object>> filter(List<Map<String, Object>> input, Map<String, String> q) {
        var list = new ArrayList<Map<String, Object>>();
        for (var row : input) {
            boolean match = true;
            String keyword = q.getOrDefault("keyword", "").trim().toLowerCase(Locale.ROOT);
            if (!keyword.isBlank()) match = row.entrySet().stream().filter(e -> e.getValue() instanceof String && !Set.of("password",
                "tokenEpoch").contains(e.getKey())).anyMatch(e -> str(e.getValue()).toLowerCase(Locale.ROOT).contains(keyword));
            for (String k : List.of("status", "bizType", "direction", "sourceType", "categoryId", "partyId",
                "deptId", "docType", "stage", "source")) if (q.containsKey(k) && !q.get(k).isBlank() && row.containsKey(k)) match &=
                q.get(k).equals(str(row.get(k)));
            String settle = q.getOrDefault("settle", "");
            if (!settle.isBlank() && row.containsKey("settleStatus")) match &= settle.equals(s(row, "settleStatus"));
            if (bool(q.get("warn"))) match &= bool(row.get("warn"));
            for (String key : List.of("startDate", "endDate")) if (q.containsKey(key) && !q.get(key).isBlank()) {
                String date = s(row, row.containsKey("orderDate") ? "orderDate" : "signDate");
                if (!date.isBlank()) match &= key.equals("startDate") ? date.compareTo(q.get(key)) >= 0 : date.compareTo(q.get(key)) <=
                    0;
            }
            if (q.containsKey("ids") && !q.get("ids").isBlank()) {
                Set<String> ids = Set.of(q.get("ids").split(","));
                match &= ids.contains(str(row.get("id")));
            }
            if (match) list.add(row);
        }
        return list;
    }

    public static Map<String, Object> page(List<Map<String, Object>> input, Map<String, String> q) {
        long page = Math.max(1, id(q.getOrDefault("page", "1")));
        int size =(int) Math.max(1, Math.min(200, id(q.getOrDefault("pageSize", "20"))));
        require(page <= 1000000, "页码超出范围");
        long offset =(page - 1) * size;
        int from =(int) Math.min(offset, input.size()), to = Math.min(from + size, input.size());
        return map("list", input.subList(from, to), "total", input.size(), "page", page, "pageSize", size);
    }

    Object export(String module, String h, Map<String, String> q, Actor a) throws Exception {
        if (h.equals("ImportTemplate")) return files.importTemplate();
        Object data;
        if (module.equals("inv")) {
            if (h.equals("ReportExport")) {
                String type = q.getOrDefault("type", "detail");
                String handler = switch (type) {
                    case "contract" -> "ReportContract";
                    case "contract-ledger" -> "ReportContractLedger";
                    case "inventory" -> "ReportStockRanking";
                    case "ranking" -> "ReportRanking";
                    default -> "ReportDetail";
                };
                data = reports.report(handler, q, a);
                if (data instanceof Map<?, ?> m) data = m.get("rows");
            } else {
                String entity = q.getOrDefault("entity", "products");
                data = switch (entity) {
                    case "products" -> {
                        auth.require(a, "inv:product:list");
                        yield master.products();
                    }
                    case "customers" -> {
                        auth.require(a, "inv:customer:list");
                        yield master.handle("CustomerList", 0, map(), q);
                    }
                    case "suppliers" -> {
                        auth.require(a, "inv:supplier:list");
                        yield master.handle("SupplierList", 0, map(), q);
                    }
                    case "orders" -> trade.orders(a, q.getOrDefault("bizType", "sales"));
                    case "stock-orders" -> {
                        auth.require(a, "inv:stock:list");
                        yield trade.handle("StockOrderList", 0, map(), q, a);
                    }
                    case "inventory" -> {
                        auth.require(a, "inv:stock:list");
                        yield master.inventory();
                    }
                    default -> throw new BusinessException(400, "导出实体非法");
                };
            }
        } else if (module.equals("user")) data = system.users(a);
        else if (module.equals("role")) data = system.roles();
        else if (module.equals("menu")) data = db.all("sys_menus");
        else if (module.equals("dept")) data = db.all("sys_depts");
        else data = db.all(h.contains("Operation") ? "sys_operation_logs" : "sys_login_logs");
        return files.excel("导出数据", filter(rows(data), q));
    }
}
