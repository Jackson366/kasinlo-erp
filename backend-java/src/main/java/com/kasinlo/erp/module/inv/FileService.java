package com.kasinlo.erp.module.inv;

import com.kasinlo.erp.persistence.ErpRepository;
import com.kasinlo.erp.module.system.*;
import com.kasinlo.erp.module.system.IdentityService.Actor;
import com.kasinlo.erp.common.BusinessException;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;
import org.springframework.web.multipart.MultipartFile;
import org.springframework.http.ResponseEntity;
import org.apache.poi.xssf.usermodel.XSSFWorkbook;
import org.apache.poi.ss.usermodel.*;
import java.nio.file.*;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.io.*;
import java.util.*;
import static com.kasinlo.erp.common.Values.*;
import static com.kasinlo.erp.common.BusinessException.*;

@Service
public class FileService {
    final ErpRepository db;final IdentityService auth;final SystemService system;final Path root;
    public FileService(ErpRepository db,IdentityService auth,SystemService system,@Value("${erp.attachment-dir}")String path){this.db=db;this.auth=auth;this.system=system;this.root=Path.of(path).toAbsolutePath().normalize();}
    public void owner(String biz,long id,Actor a,boolean write){require(Set.of("purchase","sales","contract").contains(biz),"附件业务类型非法");var o=db.find(biz.equals("contract")?"inv_contracts":"inv_orders",id);auth.visible(a,o);if(!biz.equals("contract"))require(biz.equals(s(o,"bizType")),"单据类型不匹配");if(write)auth.require(a,"inv:"+biz+":update");else auth.require(a,"inv:"+biz+":list");}
    public Object attachment(String h,long id,Map<String,String>q,MultipartFile file,Actor a)throws Exception{
        String biz=q.getOrDefault("bizType","");long bid=id(q.get("bizId"));
        if(h.equals("AttachmentDelete")||h.equals("AttachmentFile")){var row=db.find("inv_attachments",id);owner(s(row,"bizType"),n(row,"bizId"),a,h.equals("AttachmentDelete"));if(h.equals("AttachmentDelete")){db.delete("inv_attachments",id);return null;}Path p=root.resolve(s(row,"storedPath")).normalize();require(p.startsWith(root),"文件路径非法");return download(s(row,"fileName"),Files.readAllBytes(p),s(row,"mime"));}
        if(h.equals("AttachmentCount")){Map<String,Object> result=new LinkedHashMap<>();String[] ids=q.getOrDefault("bizIds",q.getOrDefault("ids",Long.toString(bid))).split(",");for(String raw:ids){long value=id(raw);if(value<=0)continue;owner(biz,value,a,false);result.put(raw,db.where("inv_attachments","bizId",value).stream().filter(r->s(r,"bizType").equals(biz)).count());}return result;}
        owner(biz,bid,a,h.equals("AttachmentUpload"));if(h.equals("AttachmentList"))return db.where("inv_attachments","bizId",bid).stream().filter(r->s(r,"bizType").equals(biz)).map(this::safeAttachment).toList();
        require(file!=null&&!file.isEmpty(),"请选择附件文件");require(file.getSize()<=20L*1024*1024,"附件不能超过20MB");String original=Objects.requireNonNullElse(file.getOriginalFilename(),"file").replace('\\','/');original=original.substring(original.lastIndexOf('/')+1);String suffix=original.contains(".")?original.substring(original.lastIndexOf('.')).toLowerCase(Locale.ROOT):"";require(Set.of(".pdf",".png",".jpg",".jpeg",".gif",".webp",".xlsx",".xls",".docx",".doc",".txt",".zip").contains(suffix),"不支持此附件类型");String stored=biz+"/"+bid+"/"+UUID.randomUUID()+suffix;Path p=root.resolve(stored).normalize();Files.createDirectories(p.getParent());Files.write(p,file.getBytes(),StandardOpenOption.CREATE_NEW);long aid=db.insert("inv_attachments",map("bizType",biz,"bizId",bid,"fileName",original,"storedPath",stored,"size",file.getSize(),"mime",Objects.requireNonNullElse(file.getContentType(),"application/octet-stream"),"uploadedBy",a.id(),"uploader",s(a.user(),"nickname")));return safeAttachment(db.find("inv_attachments",aid));
    }
    Map<String,Object> safeAttachment(Map<String,Object>r){var out=new LinkedHashMap<>(r);out.remove("storedPath");out.put("url","/api/v1/inv/attachments/"+n(r,"id")+"/file");return out;}
    public ResponseEntity<byte[]> download(String name,byte[] bytes,String type){return ResponseEntity.ok().header("Content-Type",type.isBlank()?"application/octet-stream":type).header("Content-Disposition","attachment; filename=\"download\"; filename*=UTF-8''"+URLEncoder.encode(name,StandardCharsets.UTF_8).replace("+","%20")).body(bytes);}
    public ResponseEntity<byte[]> excel(String title,List<Map<String,Object>>rows)throws Exception{
        try(var workbook=new XSSFWorkbook();var output=new ByteArrayOutputStream()){var sheet=workbook.createSheet("数据");List<String> keys=rows.isEmpty()?List.of("提示"):rows.get(0).keySet().stream().filter(k->!Set.of("password","tokenEpoch","deletedAt","storedPath").contains(k)&&!(rows.get(0).get(k) instanceof Collection<?>)&&!(rows.get(0).get(k) instanceof Map<?,?>)).toList();var head=sheet.createRow(0);var style=workbook.createCellStyle();var font=workbook.createFont();font.setBold(true);style.setFont(font);for(int i=0;i<keys.size();i++){head.createCell(i,CellType.STRING).setCellValue(keys.get(i));head.getCell(i).setCellStyle(style);sheet.setColumnWidth(i,Math.min(60,Math.max(12,keys.get(i).length()+4))*256);}for(int r=0;r<rows.size();r++){var row=sheet.createRow(r+1);for(int c=0;c<keys.size();c++)row.createCell(c,CellType.STRING).setCellValue(str(rows.get(r).get(keys.get(c))));}sheet.createFreezePane(0,1);workbook.write(output);return download(title+"_"+today()+".xlsx",output.toByteArray(),"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet");}
    }
    public ResponseEntity<byte[]> importTemplate()throws Exception {var header=new LinkedHashMap<String,Object>();for(String col:List.of("用户名","昵称","密码","邮箱","手机号","部门","角色","状态","备注"))header.put(col,"");var response=excel("用户导入模板",List.of(header));return response;}
    public Object importUsers(MultipartFile file,boolean dry,Actor a)throws Exception{require(file!=null&&!file.isEmpty(),"请选择导入文件");List<String> headers=List.of("用户名","昵称","密码","邮箱","手机号","部门","角色","状态","备注");var results=new ArrayList<Map<String,Object>>();int valid=0,created=0;Set<String> seen=new HashSet<>();try(var workbook=WorkbookFactory.create(file.getInputStream())){var sheet=workbook.getSheetAt(0);var fmt=new DataFormatter();require(sheet.getLastRowNum()<=1000,"单次最多导入1000行");for(int i=0;i<headers.size();i++)require(sheet.getRow(0)!=null&&headers.get(i).equals(fmt.formatCellValue(sheet.getRow(0).getCell(i))),"导入表头或列顺序不一致");for(int r=1;r<=sheet.getLastRowNum();r++){var row=sheet.getRow(r);if(row==null)continue;String[]v=new String[9];boolean blank=true;for(int c=0;c<9;c++){v[c]=fmt.formatCellValue(row.getCell(c)).trim();blank&=v[c].isBlank();}if(blank)continue;var errors=new ArrayList<String>();if(!v[0].matches("[A-Za-z0-9_.-]{3,64}"))errors.add("用户名格式非法");if(v[1].isBlank())errors.add("昵称不能为空");if(db.where("sys_users","username",v[0]).size()>0||seen.contains(v[0]))errors.add("用户名重复");if(!v[2].isBlank()&&v[2].length()<8)errors.add("密码至少8位");long dept=0;for(var d:db.all("sys_depts"))if(s(d,"name").equals(v[5]))dept=n(d,"id");if(!v[5].isBlank()&&dept==0)errors.add("部门不存在");List<Long>roles=new ArrayList<>();for(String code:v[6].split("[,，、;；]")){if(code.isBlank())continue;var role=db.all("sys_roles").stream().filter(x->s(x,"name").equals(code)||s(x,"code").equals(code)).findFirst();if(role.isEmpty())errors.add("角色不存在："+code);else roles.add(n(role.get(),"id"));}try{system.grantable(a,roles,true);}catch(BusinessException e){errors.add(e.getMessage());}if(errors.isEmpty()){seen.add(v[0]);valid++;if(!dry){system.handle("user","Create",0,map("username",v[0],"nickname",v[1],"password",v[2].isBlank()?"Aa123456":v[2],"email",v[3],"phone",v[4],"deptId",dept,"roleIds",roles,"status",v[7].equals("禁用")||v[7].equals("0")?0:1,"remark",v[8]),Map.of(),a);created++;}}results.add(map("row",r+1,"username",v[0],"nickname",v[1],"errors",errors));}}return map("total",results.size(),"valid",valid,"invalid",results.size()-valid,"created",created,"rows",results);}
}
