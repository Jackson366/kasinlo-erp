package com.kasinlo.erp.persistence;

import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.baomidou.mybatisplus.core.conditions.update.UpdateWrapper;
import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.kasinlo.erp.common.BusinessException;
import com.kasinlo.erp.persistence.entity.IdentifiedEntity;
import com.kasinlo.erp.persistence.entity.RequestKey;
import com.kasinlo.erp.persistence.mapper.*;
import org.mybatis.spring.SqlSessionTemplate;
import org.springframework.core.io.ClassPathResource;
import org.springframework.stereotype.Repository;
import javax.sql.DataSource;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;
import java.util.*;
import static com.kasinlo.erp.common.Values.*;

/** Ordinary CRUD uses typed BaseMapper; specialized SQL lives in mapper interfaces. */
@Repository
public class ErpRepository {
    private final SqlSessionTemplate sessions;
    private final ObjectMapper json;
    private final DataSource dataSource;
    private final RequestKeyMapper requestKeys;
    private final StockMutationMapper stock;
    private final MaintenanceMapper maintenance;
    private final Map<String, Map<String, Map<String, Object>>> schema;

    public ErpRepository(SqlSessionTemplate sessions, ObjectMapper json, DataSource dataSource,
                         RequestKeyMapper requestKeys, StockMutationMapper stock,
                         MaintenanceMapper maintenance) throws Exception {
        this.sessions = sessions;
        this.json = json;
        this.dataSource = dataSource;
        this.requestKeys = requestKeys;
        this.stock = stock;
        this.maintenance = maintenance;
        schema = json.readValue(new ClassPathResource("schema-metadata.json").getInputStream(), new TypeReference<>() {});
    }

    public Map<String, Map<String, Map<String, Object>>> schema() { return schema; }
    public Map<String, Map<String, Object>> fields(String table) {
        EntityMappings.require(table);
        return Objects.requireNonNull(schema.get(table), "Missing schema metadata");
    }
    public String col(String table, String field) {
        var metadata = fields(table).get(field);
        if (metadata == null) throw new IllegalArgumentException("Unknown field");
        return "\"" + s(metadata, "column") + "\"";
    }
    @SuppressWarnings({"unchecked", "rawtypes"})
    private BaseMapper<Object> mapper(String table) {
        return (BaseMapper) sessions.getMapper(EntityMappings.require(table).mapperType());
    }
    private Map<String, Object> record(String table, Object entity) {
        Map<String, Object> raw = json.convertValue(entity, new TypeReference<>() {});
        Map<String, Object> result = new LinkedHashMap<>();
        fields(table).forEach((field, meta) -> {
            Object value = raw.get(field);
            if (value == null && "string".equals(s(meta, "type"))) value = "";
            result.put(field, value);
        });
        return result;
    }
    private QueryWrapper<Object> query(String table) {
        fields(table);
        QueryWrapper<Object> wrapper = new QueryWrapper<>();
        if (fields(table).containsKey("id")) wrapper.orderByAsc("id");
        return wrapper;
    }
    public List<Map<String, Object>> all(String table) {
        return mapper(table).selectList(query(table)).stream().map(e -> record(table, e)).toList();
    }
    public List<Map<String, Object>> where(String table, String field, Object value) {
        return mapper(table).selectList(query(table).eq(col(table, field), value))
                .stream().map(e -> record(table, e)).toList();
    }
    public Map<String, Object> find(String table, long id) { return find(table, id, false); }
    public Map<String, Object> lock(String table, long id) { return find(table, id, true); }
    private Map<String, Object> find(String table, long id, boolean lock) {
        var wrapper = query(table).eq(col(table, "id"), id);
        if (lock) wrapper.last("FOR UPDATE");
        Object entity = mapper(table).selectOne(wrapper);
        if (entity == null) throw new BusinessException(404, "记录不存在");
        return record(table, entity);
    }
    public Object convert(Map<String, Object> meta, Object value) {
        if (value == null) return null;
        return switch (s(meta, "type")) {
            case "long" -> id(value);
            case "decimal" -> dec(value);
            case "bool" -> bool(value);
            case "time" -> {
                String text = str(value);
                if (text.isBlank()) yield null;
                if ("date".equals(s(meta, "sqlType"))) yield LocalDate.parse(date(text));
                yield OffsetDateTime.parse(text.length() == 10 ? text + "T00:00:00+08:00" : text);
            }
            default -> {
                String text = str(value);
                long max = n(meta, "max");
                BusinessException.require(max == 0 || text.codePointCount(0, text.length()) <= max, "字段长度超限");
                yield text;
            }
        };
    }
    private Map<String, Object> values(String table, Map<String, ?> input, Set<String> excluded) {
        Map<String, Object> result = new LinkedHashMap<>();
        input.forEach((key, value) -> {
            if (fields(table).containsKey(key) && !excluded.contains(key)) {
                result.put(key, convert(fields(table).get(key), value));
            }
        });
        return result;
    }
    public long insert(String table, Map<String, ?> input) {
        var values = values(table, input, Set.of("id", "deletedAt"));
        if (fields(table).containsKey("createdAt")) values.put("createdAt", OffsetDateTime.now());
        if (fields(table).containsKey("updatedAt")) values.put("updatedAt", OffsetDateTime.now());
        if (fields(table).containsKey("status")) values.putIfAbsent("status", 1L);
        Object entity = json.convertValue(values, EntityMappings.require(table).entityType());
        mapper(table).insert(entity);
        return entity instanceof IdentifiedEntity identified ? identified.getId() : 0;
    }
    public void update(String table, long id, Map<String, ?> input) {
        find(table, id);
        var values = values(table, input, Set.of("id", "createdAt", "deletedAt"));
        if (fields(table).containsKey("updatedAt")) values.put("updatedAt", OffsetDateTime.now());
        if (values.isEmpty()) return;
        var wrapper = new UpdateWrapper<Object>().eq(col(table, "id"), id);
        // Explicit .set preserves PATCH null, empty string, false and zero values.
        values.forEach((field, value) -> wrapper.set(col(table, field), value));
        mapper(table).update(null, wrapper);
    }
    public void delete(String table, long id) {
        find(table, id);
        mapper(table).delete(new QueryWrapper<>().eq(col(table, "id"), id));
    }
    public void deleteWhere(String table, String field, Object value) {
        mapper(table).delete(new QueryWrapper<>().eq(col(table, field), value));
    }
    public void unique(String table, String field, String value, long except) {
        BusinessException.conflict(where(table, field, value).stream().noneMatch(r -> n(r, "id") != except), "编号或名称已存在");
    }
    public String number(String table, String field, String prefix) {
        long max = 0;
        for (var row : all(table)) {
            String value = s(row, field);
            if (value.startsWith(prefix)) {
                try { max = Math.max(max, Long.parseLong(value.substring(prefix.length()))); }
                catch (NumberFormatException ignored) { /* Manual non-sequential code. */ }
            }
        }
        return prefix + String.format("%04d", max + 1);
    }

    public int ping() { return maintenance.ping(); }
    public int adjustStock(long id, BigDecimal delta) { return stock.adjust(id, delta); }
    public void resetStock() { mapper("inv_product_specs").update(null, new UpdateWrapper<>().set("stock", BigDecimal.ZERO)); }
    public void clearOperations() { mapper("sys_operation_logs").delete(new QueryWrapper<>()); }
    public void clearSuccessfulLogins() { mapper("sys_login_logs").delete(new QueryWrapper<>().eq("success", true)); }
    public long countBefore(String table, OffsetDateTime cutoff) {
        return mapper(table).selectCount(new QueryWrapper<>().lt(col(table, "createdAt"), cutoff));
    }
    public int deleteBefore(String table, OffsetDateTime cutoff) {
        return mapper(table).delete(new QueryWrapper<>().lt(col(table, "createdAt"), cutoff));
    }
    public Map<String, Object> lockRequest(String key) {
        RequestKey entry = requestKeys.selectOne(new QueryWrapper<RequestKey>().eq("request_key", key).last("FOR UPDATE"));
        return entry == null ? null : map("fingerprint", entry.getFingerprint(), "response", entry.getResponse());
    }
    public void createRequest(String key, long uid, String fingerprint) {
        RequestKey entry = new RequestKey();
        entry.setRequestKey(key); entry.setUserId(uid); entry.setFingerprint(fingerprint);
        entry.setCreatedAt(OffsetDateTime.now());
        requestKeys.insert(entry);
    }
    public void finishRequest(String key, String response) {
        requestKeys.update(null, new UpdateWrapper<RequestKey>().eq("request_key", key).set("response", response));
    }
    public long requestCount() { return requestKeys.selectCount(new QueryWrapper<>()); }

    /** Physical operations are reserved for confirmed backup/restore and cleanup. */
    public void purge(String table) { fields(table); maintenance.purge(table); }
    public List<Map<String, Object>> physicalRows(String table) { fields(table); return maintenance.physicalRows(table); }
    public void restoreRow(String table, Map<String, Object> row) {
        var values = values(table, row, Set.of());
        if (!values.isEmpty()) maintenance.restore(table, values);
    }
    public void resetIdentity(String table, long next) throws Exception {
        BusinessException.require(next > 0, "序列起点非法");
        fields(table);
        try (var connection = dataSource.getConnection()) {
            if (connection.getMetaData().getDatabaseProductName().contains("H2")) maintenance.resetH2(table, next);
            else maintenance.resetPostgres(table, next);
        }
    }
}
