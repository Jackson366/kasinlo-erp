package com.kasinlo.erp.module.system;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.kasinlo.erp.persistence.ErpRepository;
import com.kasinlo.erp.common.BusinessException;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.stereotype.Service;
import jakarta.servlet.http.HttpServletRequest;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import java.nio.charset.StandardCharsets;
import java.security.*;
import java.time.Instant;
import java.util.*;
import static com.kasinlo.erp.common.Values.*;

@Service
public class IdentityService {
    public record Actor(long id,Map<String,Object> user,List<Map<String,Object>> roles,Set<String> permissions,boolean superuser,int scope) {}
    private final ErpRepository db;
    private final ObjectMapper json;
    private final byte[] secret;
    public final BCryptPasswordEncoder passwords=new BCryptPasswordEncoder();
    public IdentityService(ErpRepository db,ObjectMapper json,@Value("${erp.jwt-secret:}")String key) {
        this.db=db;this.json=json;
        if(key.isBlank()){secret=new byte[64];new SecureRandom().nextBytes(secret);}else{BusinessException.require(key.length()>=32,"JWT 密钥至少 32 个字符");secret=key.getBytes(StandardCharsets.UTF_8);}
    }
    public Actor actor(long uid) {
        var user=db.find("sys_users",uid);
        if(n(user,"status")!=1)throw new BusinessException(401,"账号已停用");
        var roles=new ArrayList<Map<String,Object>>();Set<String> perms=new LinkedHashSet<>();boolean su=false;int scope=4;
        for(var link:db.where("sys_user_roles","userId",uid)){
            Map<String,Object> role;try{role=db.find("sys_roles",n(link,"roleId"));}catch(BusinessException e){continue;}
            if(n(role,"status")!=1)continue;roles.add(role);scope=Math.min(scope,(int)Math.max(1,n(role,"dataScope")));su|="super_admin".equals(s(role,"code"));
            for(var rm:db.where("sys_role_menus","roleId",n(role,"id")))try{var menu=db.find("sys_menus",n(rm,"menuId"));if(n(menu,"status")==1&&!s(menu,"permission").isBlank())perms.add(s(menu,"permission"));}catch(BusinessException ignored){}
        }
        return new Actor(uid,user,roles,perms,su,scope);
    }
    public Actor authenticate(HttpServletRequest req){String h=req.getHeader("Authorization");if(h==null||!h.startsWith("Bearer "))throw new BusinessException(401,"请先登录");return fromToken(h.substring(7),"access");}
    private byte[] hmac(String text) throws Exception {Mac mac=Mac.getInstance("HmacSHA256");mac.init(new SecretKeySpec(secret,"HmacSHA256"));return mac.doFinal(text.getBytes(StandardCharsets.UTF_8));}
    private String b64(byte[] value){return Base64.getUrlEncoder().withoutPadding().encodeToString(value);}
    private String token(Actor actor,String type,long seconds) {
        try{String head=b64("{\"alg\":\"HS256\",\"typ\":\"JWT\"}".getBytes(StandardCharsets.UTF_8));String body=b64(json.writeValueAsBytes(map("sub",Long.toString(actor.id()),"epoch",n(actor.user(),"tokenEpoch"),"type",type,"iss","kasinlo-erp","iat",Instant.now().getEpochSecond(),"exp",Instant.now().getEpochSecond()+seconds)));return head+"."+body+"."+b64(hmac(head+"."+body));}catch(Exception e){throw new IllegalStateException(e);}
    }
    @SuppressWarnings("unchecked") private Actor fromToken(String token,String type) {
        try{String[] parts=token.split("\\.");if(parts.length!=3||!MessageDigest.isEqual(hmac(parts[0]+"."+parts[1]),Base64.getUrlDecoder().decode(parts[2])))throw new Exception();var head=json.readValue(Base64.getUrlDecoder().decode(parts[0]),Map.class);var claim=json.readValue(Base64.getUrlDecoder().decode(parts[1]),Map.class);if(!"HS256".equals(head.get("alg"))||!"kasinlo-erp".equals(claim.get("iss"))||!type.equals(claim.get("type"))||n(claim,"exp")<=Instant.now().getEpochSecond()||n(claim,"sub")<=0)throw new Exception();Actor a=actor(n(claim,"sub"));if(n(claim,"epoch")!=n(a.user(),"tokenEpoch"))throw new Exception();return a;}catch(Exception e){throw new BusinessException(401,"登录已失效，请重新登录");}
    }
    public Object login(Map<String,Object> body,HttpServletRequest req){
        var users=db.where("sys_users","username",s(body,"username"));
        boolean valid=!users.isEmpty()&&passwords.matches(s(body,"password"),s(users.get(0),"password"))&&n(users.get(0),"status")==1;
        db.insert("sys_login_logs",map("userId",users.isEmpty()?0:n(users.get(0),"id"),"username",s(body,"username"),"ip",req.getRemoteAddr(),"userAgent",str(req.getHeader("User-Agent")),"success",valid,"message",valid?"登录成功":"账号或密码错误"));
        if(!valid)throw new BusinessException(401,40101,"账号或密码错误");
        var a=actor(n(users.get(0),"id"));db.update("sys_users",a.id(),map("lastLoginAt",now(),"lastLoginIp",req.getRemoteAddr()));return tokens(a);
    }
    public Object refresh(Map<String,Object> b){return tokens(fromToken(s(b,"refreshToken"),"refresh"));}
    public Object tokens(Actor a){return map("accessToken",token(a,"access",7200),"refreshToken",token(a,"refresh",604800),"expiresIn",7200,"tokenType","Bearer","user",profile(a));}
    public void revoke(long uid){var u=db.lock("sys_users",uid);db.update("sys_users",uid,map("tokenEpoch",n(u,"tokenEpoch")+1));}
    public Map<String,Object> profile(Actor a){var out=new LinkedHashMap<>(a.user());out.remove("password");out.remove("tokenEpoch");out.remove("deletedAt");out.put("isSuper",a.superuser());out.put("roles",a.roles().stream().map(r->s(r,"name")).toList());out.put("roleCodes",a.roles().stream().map(r->s(r,"code")).toList());out.put("permissions",a.permissions());out.put("deptName",deptName(n(out,"deptId")));return out;}
    public String deptName(long id){try{return s(db.find("sys_depts",id),"name");}catch(BusinessException e){return "";}}
    public void require(Actor a,String... perms){if(a.superuser())return;for(String p:perms)if(a.permissions().contains(p))return;throw new BusinessException(403,"无操作权限");}
    public Set<Long> visibleUsers(Actor a){
        Set<Long> ids=new HashSet<>();if(a.superuser()||a.scope()==1){db.all("sys_users").forEach(u->ids.add(n(u,"id")));ids.add(0L);return ids;}
        if(a.scope()==4){ids.add(a.id());return ids;}Set<Long> depts=new HashSet<>();depts.add(n(a.user(),"deptId"));
        if(a.scope()==3){boolean changed;do{changed=false;for(var d:db.all("sys_depts"))if(depts.contains(n(d,"parentId")))changed|=depts.add(n(d,"id"));}while(changed);}
        for(var u:db.all("sys_users"))if(depts.contains(n(u,"deptId")))ids.add(n(u,"id"));return ids;
    }
    public void visible(Actor a,Map<String,Object> record){if(!a.superuser()&&a.scope()!=1&&!visibleUsers(a).contains(n(record,"createdBy")))throw new BusinessException(404,"记录不存在");}
    public List<Map<String,Object>> scoped(String table,Actor a){var rows=db.all(table);if(a.superuser()||a.scope()==1)return rows;var ids=visibleUsers(a);return rows.stream().filter(r->ids.contains(n(r,"createdBy"))).toList();}
    public List<Map<String,Object>> routeTree(Actor a){
        var menus=db.all("sys_menus").stream().filter(m->n(m,"status")==1&&!"button".equals(s(m,"type"))).toList();
        Set<Long> allowed=new HashSet<>();for(var m:menus)if(a.superuser()||a.permissions().contains(s(m,"permission")))allowed.add(n(m,"id"));
        boolean added;do{added=false;for(var m:menus)if(allowed.contains(n(m,"id")))added|=allowed.add(n(m,"parentId"));}while(added);
        var output=new ArrayList<Map<String,Object>>();for(var m:menus)if(allowed.contains(n(m,"id")))output.add(map("id",m.get("id"),"parentId",m.get("parentId"),"name",m.get("name"),"path",m.get("path"),"component",m.get("component"),"redirect",m.get("redirect"),"sort",m.get("sort"),"meta",map("title",m.get("title"),"icon",m.get("icon"),"hidden",m.get("hidden"),"keepAlive",m.get("keepAlive"),"isExternal",m.get("isExternal"),"permission",m.get("permission"))));return tree(output,0);
    }
    public static List<Map<String,Object>> tree(List<Map<String,Object>> input,long parent){var out=new ArrayList<Map<String,Object>>();for(var row:input)if(n(row,"parentId")==parent){var r=new LinkedHashMap<>(row);r.put("children",tree(input,n(row,"id")));out.add(r);}out.sort(Comparator.comparingLong(r->n(r,"sort")));return out;}
}
