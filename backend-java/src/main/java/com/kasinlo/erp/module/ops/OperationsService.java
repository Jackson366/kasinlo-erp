package com.kasinlo.erp.module.ops;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.kasinlo.erp.persistence.ErpRepository;
import com.kasinlo.erp.module.inv.FileService;
import com.kasinlo.erp.common.BusinessException;
import org.springframework.stereotype.Service;
import java.nio.file.*;
import java.util.*;
import java.security.MessageDigest;
import java.util.stream.Collectors;

import static com.kasinlo.erp.common.Values.*;
import static com.kasinlo.erp.common.BusinessException.*;

@Service
public class OperationsService {
    final ErpRepository db;
    final ObjectMapper json;
    final FileService files;
    final Path backups = Path.of("data/backups").toAbsolutePath();

    public OperationsService(ErpRepository db, ObjectMapper json, FileService files) {
        this.db = db;
        this.json = json;
        this.files = files;
    }

    Path backupPath(String name) {
        require(name.matches("[A-Za-z0-9_.-]+\\.json"), "备份文件名非法");
        return backups.resolve(name);
    }

    Map<String, Object> info(Path p) throws Exception {
        return map("name", p.getFileName().toString(), "path", p.getFileName().toString(), "size", Files.size(p),
            "time", Files.getLastModifiedTime(p).toInstant().toString(), "sha256", HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256")
            .digest(Files.readAllBytes(p))), "database", "kasinlo-erp", "serverVersion", "Java", "tables",
            db.schema().size(), "hasMeta", true, "verified", true);
    }

    public Object handle(String h, Map<String, Object> b, Map<String, String> q) throws Exception {
        Files.createDirectories(backups);
        if (h.equals("Backups")) {
            try (var paths = Files.list(backups)) {
                var out = new ArrayList<Map<String, Object>>();
                for (var p : paths.filter(x -> x.toString().endsWith(".json")).toList()) out.add(info(p));
                return out;
            }
        }
        if (h.equals("RunBackup") || h.equals("UpgradeBackup")) {
            Map<String, Object> data = new LinkedHashMap<>();
            for (String t : db.schema().keySet()) data.put(t, db.all(t));
            String name = "kasinlo-" + System.currentTimeMillis() + ".json";
            byte[] bytes = json.writeValueAsBytes(map("format", "kasinlo-java-backup-v1", "createdAt",
                now(), "tables", data));
            json.readTree(bytes);
            Files.write(backupPath(name), bytes, StandardOpenOption.CREATE_NEW);
            return info(backupPath(name));
        }
        if (h.equals("DownloadBackup")) {
            String name = q.getOrDefault("file", "");
            return files.download(name, Files.readAllBytes(backupPath(name)), "application/json");
        }
        if (h.equals("DeleteBackup")) {
            Files.delete(backupPath(s(b, "file")));
            return null;
        }
        if (h.equals("RestoreInPlace") || h.equals("Restore")) {
            if (h.equals("Restore")) throw new BusinessException(422, "Java 版恢复演练请使用独立数据库启动后执行原地恢复");
            require("确认覆盖当前数据库".equals(s(b, "confirm")), "请输入确认覆盖当前数据库");
            Map<String, Object> data = json.readValue(Files.readAllBytes(backupPath(s(b, "file"))), new TypeReference<>() {
            });
            require("kasinlo-java-backup-v1".equals(s(data, "format")), "备份格式不兼容");
            require(data.get("tables") instanceof Map<?, ?>, "备份内容不完整");
            Map<?, ?> tables =(Map<?, ?>) data.get("tables");
            var requiredTables = new HashSet<>(db.schema().keySet());
            requiredTables.remove("inv_order_audits"); // V1/V2 backups contain no audit events.
            require(tables.keySet().containsAll(requiredTables), "备份缺少必需数据表");
            for (String t : db.schema().keySet()) {
                db.purge(t);
                for (var row : rows(tables.get(t))) db.restoreRow(t, row);
            }
            for (String t : db.schema().keySet()) if (db.fields(t).containsKey("id")) {
                long next = db.all(t).stream().mapToLong(r -> n(r, "id")).max().orElse(0) + 1;
                db.resetIdentity(t, next);
            }
            return map("restored", true, "tables", db.schema().size());
        }
        if (h.equals("Check")) {
            var stats = new ArrayList<Map<String, Object>>();
            for (String t : db.schema().keySet()) stats.add(map("name", t, "rows", db.all(t).size(), "softDeleted",
                0, "sizeBytes", 0));
            return map("database", "kasinlo-erp", "serverVersion", "Java 17+ / Spring Boot", "sizeBytes",
                0, "tables", db.schema().size(), "schemaVersionDb", 3, "schemaVersionBinary", 3, "schemaVersionKnown",
                true, "opLogRows", db.all("sys_operation_logs").size(), "opLogOldest", "", "loginLogRows",
                db.all("sys_login_logs").size(), "idempotencyRows", db.requestCount(), "idempotencyExpired",
                0, "latestBackup", "", "latestBackupAt", "", "checkedAt", now(), "keyTables", stats);
        }
        if (h.equals("BizScopes")) return scopes();
        if (h.equals("BusinessCleanup")) {
            var scope = scopes().stream().filter(s -> s(s, "key").equals(s(b, "scope"))).findFirst().orElseThrow(() ->
                new BusinessException(400, "清理范围非法"));
            boolean apply = bool(b.get("apply")) || !s(b, "confirm").isBlank();
            if (apply) require(s(scope, "confirm").equals(s(b, "confirm")), "确认口令不匹配");
            List<String> tables = new ArrayList<>(List.of("inv_order_audits", "inv_order_items", "inv_stock_items", "inv_settlements",
                "inv_orders", "inv_stock_orders"));
            if (!s(b, "scope").equals("inv_documents")) {
                tables.addAll(List.of("inv_contract_plans", "inv_contracts", "inv_attachments", "inv_product_units",
                    "inv_product_specs", "inv_products", "inv_categories", "inv_customers", "inv_suppliers",
                    "inv_print_templates"));
            }
            var stats = new ArrayList<Map<String, Object>>();
            long total = 0;
            for (String t : tables) {
                long count = db.all(t).size();
                total += count;
                stats.add(map("name", t, "rows", count));
                if (apply) db.purge(t);
            }
            if (apply && bool(b.get("resetStock"))) db.resetStock();
            return map("scope", b.get("scope"), "title", scope.get("title"), "dryRun", !apply, "stockReset",
                bool(b.get("resetStock")), "totalRows", total, "tables", stats);
        }
        if (h.equals("CleanupPreview") || h.equals("CleanupApply")) {
            boolean apply = h.equals("CleanupApply");
            if (apply) require(s(b, "confirm").contains("确认"), "请输入确认口令");
            var steps = new ArrayList<Map<String, Object>>();
            for (String t : List.of("sys_operation_logs", "sys_login_logs")) {
                long days = n(b, t.equals("sys_operation_logs") ? "opLogDays" : "loginLogDays");
                if (days <= 0) days = 90;
                String cutoff = java.time.OffsetDateTime.now().minusDays(days).toString();
                long count = db.countBefore(t, java.time.OffsetDateTime.parse(cutoff));
                if (apply) db.deleteBefore(t, java.time.OffsetDateTime.parse(cutoff));
                steps.add(map("name", t, "table", t, "cutoff", cutoff, "rows", count, "batches", count > 0 ? 1 : 0,
                    "note", ""));
            }
            return map("dryRun", !apply, "steps", steps, "vacuumed", List.of(), "blocked", "");
        }
        if (h.equals("UpgradeRecords")) return db.all("sys_upgrade_records");
        if (h.equals("UpgradeStatus")) return map("currentVersion", "2.0.0-java", "release", "2.0.0-java",
            "schemaVersion", 2, "plans", List.of(), "actions", List.of(), "requiresRestart", false, "packageFormat",
            "java-jar");
        throw new BusinessException(422, "Java 版采用 Maven/JAR 发布；Go 二进制升级包不适用，请通过部署流程更新 JAR");
    }

    List<Map<String, Object>> scopes() {
        return List.of(map("key", "inv_documents", "title", "进销存单据", "desc", "清理采购销售和出入库单据", "keep", "商品、往来单位与系统配置",
            "confirm", "确认清理单据", "resetStockable", true, "totalRows", 0, "tables", List.of()), map("key",
            "inv_all", "title", "进销存全部数据", "desc", "清理全部业务档案与单据", "keep", "系统用户、权限、部门", "confirm", "确认清空进销存",
            "resetStockable", false, "totalRows", 0, "tables", List.of()), map("key", "database", "title",
            "数据库重置", "desc", "清理业务数据", "keep", "系统框架", "confirm", "确认重置数据库", "resetStockable", false,
            "totalRows", 0, "tables", List.of()));
    }
}
