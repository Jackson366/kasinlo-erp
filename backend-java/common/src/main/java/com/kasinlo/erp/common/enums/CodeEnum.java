package com.kasinlo.erp.common.enums;

/** Code is an explicit, unique value within the enum and is independent of its declaration order. */
public interface CodeEnum<T> {
    T getCode();

    String getDescription();
}
