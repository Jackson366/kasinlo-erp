package com.kasinlo.erp;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@org.mybatis.spring.annotation.MapperScan("com.kasinlo.erp.persistence.mapper")
@SpringBootApplication
public class ErpApplication {
    public static void main(String[] args) { SpringApplication.run(ErpApplication.class, args); }
}
