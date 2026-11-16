package com.kasinlo.erp.adapter;

import com.fasterxml.jackson.annotation.JsonInclude;

@JsonInclude(JsonInclude.Include.NON_NULL)
public record ApiResponse(int code, String message, Object data) {}
