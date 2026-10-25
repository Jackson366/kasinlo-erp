package com.kasinlo.erp.persistence;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.springframework.core.io.ClassPathResource;
import java.util.*;
import java.util.stream.Collectors;

/** Identifiers must match typed mappings and committed metadata. */
public final class MaintenanceSql {
    private static final Map<String, Map<String, Map<String, Object>>> SCHEMA;
    static {
        try (var stream = new ClassPathResource("schema-metadata.json").getInputStream()) {
            SCHEMA = new ObjectMapper().readValue(stream, new TypeReference<>() {});
        } catch (Exception error) { throw new ExceptionInInitializerError(error); }
    }
    private static String table(Map<String, Object> params) {
        String name = (String) params.get("table");
        EntityMappings.require(name);
        return name;
    }
    private static String column(String table, String field) {
        var meta = SCHEMA.get(table).get(field);
        if (meta == null) throw new IllegalArgumentException("Unknown field");
        return "\"" + meta.get("column") + "\"";
    }
    public static String purge(Map<String, Object> params) { return "DELETE FROM " + table(params); }
    public static String selectPhysical(Map<String, Object> params) {
        String table = table(params);
        return "SELECT " + SCHEMA.get(table).keySet().stream()
                .map(field -> column(table, field) + " AS \"" + field + "\"")
                .collect(Collectors.joining(",")) + " FROM " + table;
    }
    public static String restore(Map<String, Object> params) {
        String table = table(params);
        Map<?, ?> values = (Map<?, ?>) params.get("values");
        List<String> keys = values.keySet().stream().map(Object::toString).toList();
        if (keys.isEmpty()) throw new IllegalArgumentException("Empty snapshot row");
        return "INSERT INTO " + table + " (" + keys.stream().map(k -> column(table, k))
                .collect(Collectors.joining(",")) + ") VALUES ("
                + keys.stream().map(k -> "#{values." + k + "}").collect(Collectors.joining(",")) + ")";
    }
    public static String resetH2(Map<String, Object> params) {
        String table = table(params);
        if (!SCHEMA.get(table).containsKey("id")) throw new IllegalArgumentException("No identity column");
        long next = ((Number) params.get("next")).longValue();
        if (next <= 0) throw new IllegalArgumentException("Invalid sequence start");
        return "ALTER TABLE " + table + " ALTER COLUMN id RESTART WITH " + next;
    }
}
