package com.kasinlo.erp.common.enums;

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;

public final class EnumUtils {
    private static final ClassValue<Map<Object, Enum<?>>> BY_CODE = new ClassValue<>() {
        @Override
        protected Map<Object, Enum<?>> computeValue(Class<?> enumType) {
            Map<Object, Enum<?>> values = new LinkedHashMap<>();
            for (Object constant : enumType.getEnumConstants()) {
                Object code = Objects.requireNonNull(((CodeEnum<?>) constant).getCode(), "Enum code");
                if (values.putIfAbsent(code, (Enum<?>) constant) != null) {
                    throw new IllegalStateException("Duplicate code in " + enumType.getSimpleName() + ": " + code);
                }
            }
            return Collections.unmodifiableMap(values);
        }
    };

    private EnumUtils() {}

    public static <T, E extends Enum<E> & CodeEnum<T>> Optional<E> getByCode(Class<E> enumType, T code) {
        if (code == null) {
            return Optional.empty();
        }
        return Optional.ofNullable(enumType.cast(BY_CODE.get(enumType).get(code)));
    }

    public static <T, E extends Enum<E> & CodeEnum<T>> Optional<String> getDescription(Class<E> enumType, T code) {
        return getByCode(enumType, code).map(CodeEnum::getDescription);
    }

    public static <T, E extends Enum<E> & CodeEnum<T>> List<E> getAll(Class<E> enumType) {
        return BY_CODE.get(enumType).values().stream()
                .map(enumType::cast)
                .toList();
    }

    public static <T, E extends Enum<E> & CodeEnum<T>> boolean containsCode(Class<E> enumType, T code) {
        return getByCode(enumType, code).isPresent();
    }
}
