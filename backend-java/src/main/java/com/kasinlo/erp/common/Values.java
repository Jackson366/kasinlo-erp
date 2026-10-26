package com.kasinlo.erp.common;

import java.math.*;
import java.time.*;
import java.util.*;

public final class Values {
    private Values() {
    }

    public static Map<String, Object> map(Object...pairs) {
        Map<String, Object> out = new LinkedHashMap<>();
        for (int i = 0; i < pairs.length; i += 2) out.put(pairs[i].toString(), pairs[i + 1]);
        return out;
    }

    public static String str(Object value) {
        return value == null ? "" : value.toString();
    }

    public static String s(Map<String, ?> m, String k) {
        return str(m.get(k));
    }

    public static long id(Object v) {
        try {
            return new BigDecimal(str(v)).longValueExact();
        } catch (Exception e) {
            return 0;
        }
    }

    public static long n(Map<String, ?> m, String k) {
        return id(m.get(k));
    }

    public static BigDecimal dec(Object v) {
        try {
            return new BigDecimal(str(v));
        } catch (Exception e) {
            return BigDecimal.ZERO;
        }
    }

    public static BigDecimal d(Map<String, ?> m, String k) {
        return dec(m.get(k));
    }

    public static BigDecimal money(BigDecimal v) {
        return v.setScale(2, RoundingMode.HALF_UP);
    }

    public static BigDecimal qty(BigDecimal v) {
        return v.setScale(4, RoundingMode.HALF_UP);
    }

    public static boolean bool(Object v) {
        return Boolean.TRUE.equals(v) || "true".equals(str(v)) || "1".equals(str(v));
    }

    @SuppressWarnings("unchecked")
    public static List<Map<String, Object>> rows(Object v) {
        return v instanceof List < ? > ?(List<Map<String, Object>>) v : new ArrayList<>();
    }

    public static String date(Object value) {
        String v = str(value);
        return v.isBlank() ? null : v.substring(0, Math.min(10, v.length()));
    }

    public static String today() {
        return LocalDate.now(ZoneId.of("Asia/Shanghai")).toString();
    }

    public static String now() {
        return OffsetDateTime.now(ZoneId.of("Asia/Shanghai")).toString();
    }

    public static String settle(BigDecimal total, BigDecimal settled) {
        return settled.signum() <= 0 ? "unpaid" : settled.compareTo(total) >= 0 ? "paid" : "partial";
    }
}
