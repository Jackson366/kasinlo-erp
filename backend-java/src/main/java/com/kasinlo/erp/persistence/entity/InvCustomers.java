package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for inv_customers. */
@TableName("inv_customers")
public class InvCustomers extends SoftDeleteEntity {
    @TableField("\"code\"")
    private String code;

    @TableField("\"name\"")
    private String name;

    @TableField("\"contact\"")
    private String contact;

    @TableField("\"phone\"")
    private String phone;

    @TableField("\"address\"")
    private String address;

    @TableField("\"tax_no\"")
    private String taxNo;

    @TableField("\"status\"")
    private Long status;

    @TableField("\"remark\"")
    private String remark;

    public String getCode() { return code; }
    public void setCode(String code) { this.code = code; }

    public String getName() { return name; }
    public void setName(String name) { this.name = name; }

    public String getContact() { return contact; }
    public void setContact(String contact) { this.contact = contact; }

    public String getPhone() { return phone; }
    public void setPhone(String phone) { this.phone = phone; }

    public String getAddress() { return address; }
    public void setAddress(String address) { this.address = address; }

    public String getTaxNo() { return taxNo; }
    public void setTaxNo(String taxNo) { this.taxNo = taxNo; }

    public Long getStatus() { return status; }
    public void setStatus(Long status) { this.status = status; }

    public String getRemark() { return remark; }
    public void setRemark(String remark) { this.remark = remark; }

}
