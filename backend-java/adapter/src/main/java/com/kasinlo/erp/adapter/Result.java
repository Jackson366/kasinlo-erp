package com.kasinlo.erp.adapter;

import com.fasterxml.jackson.annotation.JsonInclude;

@JsonInclude(JsonInclude.Include.NON_NULL)
public record Result<T>(int code, String message, T data) {}
