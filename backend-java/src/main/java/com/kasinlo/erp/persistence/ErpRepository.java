package com.kasinlo.erp.persistence;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.kasinlo.erp.common.BusinessException;
import org.springframework.core.io.ClassPathResource;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.support.GeneratedKeyHolder;
import org.springframework.stereotype.Repository;
import java.sql.*;
import java.time.*;
import java.util.*;
import java.util.stream.*;
import static com.kasinlo.erp.common.Values.*;

/** SQL identifiers come exclusively from the committed schema registry. */
@Repository
public class ErpRepository {
    public final JdbcTemplate jdbc;
    public final Map<String,Map<String,Map<String,Object>>> schema;
    public ErpRepository(JdbcTemplate jdbc, ObjectMapper json) throws Exception {
        this.jdbc=jdbc;
        schema=json.readValue(new ClassPathResource("schema-metadata.json").getInputStream(),new TypeReference<>(){});
    }
    public JdbcTemplate jdbc() { return jdbc; }
    public Map<String,Map<String,Map<String,Object>>> schema() { return schema; }
    public Map<String,Map<String,Object>> fields(String table) {
        var f=schema.get(table); if(f==null) throw new IllegalArgumentException("Unknown table"); return f;
    }
    public String col(String table,String field) {
        var f=fields(table).get(field); if(f==null) throw new IllegalArgumentException("Unknown field: "+table+"."+field);return s(f,"column");
    }
    private Map<String,Object> read(String table,ResultSet rs) throws SQLException {
        var out=new LinkedHashMap<String,Object>();
        for(var e:fields(table).entrySet()) {
            Object v=rs.getObject(s(e.getValue(),"column"));
            if(v instanceof Clob c) v=c.getSubString(1,(int)c.length());
            if(v instanceof java.sql.Date d)v=d.toLocalDate().toString();
            if(v instanceof Timestamp t)v=t.toInstant().atOffset(ZoneOffset.ofHours(8)).toString();
            if(v instanceof OffsetDateTime t)v=t.toString();
            if(v==null && "string".equals(s(e.getValue(),"type")))v="";
            out.put(e.getKey(),v);
        }
        return out;
    }
    public List<Map<String,Object>> all(String table) {
        String cond=fields(table).containsKey("deletedAt")?" WHERE deleted_at IS NULL":"";
        return jdbc.query("SELECT * FROM "+table+cond+(fields(table).containsKey("id")?" ORDER BY id":""),(rs,n)->read(table,rs));
    }
    public List<Map<String,Object>> where(String table,String field,Object value) {
        return jdbc.query("SELECT * FROM "+table+" WHERE "+col(table,field)+"=?"+(fields(table).containsKey("deletedAt")?" AND deleted_at IS NULL":"")+(fields(table).containsKey("id")?" ORDER BY id":""),(rs,n)->read(table,rs),value);
    }
    public Map<String,Object> find(String table,long id) { return find(table,id,false); }
    public Map<String,Object> lock(String table,long id) { return find(table,id,true); }
    private Map<String,Object> find(String table,long id,boolean lock) {
        var found=jdbc.query("SELECT * FROM "+table+" WHERE id=?"+(fields(table).containsKey("deletedAt")?" AND deleted_at IS NULL":"")+(lock?" FOR UPDATE":""),(rs,n)->read(table,rs),id);
        if(found.isEmpty())throw new BusinessException(404,"记录不存在"); return found.get(0);
    }
    public Object convert(Map<String,Object> meta,Object v) {
        if(v==null)return null;
        return switch(s(meta,"type")) {
            case "long" -> id(v);
            case "decimal" -> dec(v);
            case "bool" -> bool(v);
            case "time" -> {
                String t=str(v); if(t.isBlank())yield null;
                if("date".equals(s(meta,"sqlType")))yield java.sql.Date.valueOf(date(t));
                yield t.length()==10?OffsetDateTime.parse(t+"T00:00:00+08:00"):OffsetDateTime.parse(t);
            }
            default -> { String text=str(v); long max=n(meta,"max");BusinessException.require(max==0||text.codePointCount(0,text.length())<=max,"字段长度超限");yield text; }
        };
    }
    public long insert(String table,Map<String,?> input) {
        var values=new LinkedHashMap<String,Object>();
        for(var e:input.entrySet())if(fields(table).containsKey(e.getKey())&&!Set.of("id","deletedAt").contains(e.getKey()))values.put(e.getKey(),convert(fields(table).get(e.getKey()),e.getValue()));
        if(fields(table).containsKey("createdAt")) values.put("createdAt",OffsetDateTime.now());
        if(fields(table).containsKey("updatedAt")) values.put("updatedAt",OffsetDateTime.now());
        if(fields(table).containsKey("status"))values.putIfAbsent("status",1);
        var keys=new ArrayList<>(values.keySet());
        String sql="INSERT INTO "+table+" ("+keys.stream().map(k->col(table,k)).collect(Collectors.joining(","))+") VALUES ("+String.join(",",Collections.nCopies(keys.size(),"?"))+")";
        if(!fields(table).containsKey("id")){jdbc.update(sql,values.values().toArray());return 0;}
        var holder=new GeneratedKeyHolder();
        jdbc.update(c->{var p=c.prepareStatement(sql,new String[]{"id"});int i=1;for(Object v:values.values())p.setObject(i++,v);return p;},holder);
        return Objects.requireNonNull(holder.getKey()).longValue();
    }
    public void update(String table,long id,Map<String,?> input) {
        find(table,id);var values=new LinkedHashMap<String,Object>();
        for(var e:input.entrySet())if(fields(table).containsKey(e.getKey())&&!Set.of("id","createdAt","deletedAt").contains(e.getKey()))values.put(e.getKey(),convert(fields(table).get(e.getKey()),e.getValue()));
        if(fields(table).containsKey("updatedAt"))values.put("updatedAt",OffsetDateTime.now());
        if(values.isEmpty())return;
        var args=new ArrayList<>(values.values());args.add(id);
        jdbc.update("UPDATE "+table+" SET "+values.keySet().stream().map(k->col(table,k)+"=?").collect(Collectors.joining(","))+" WHERE id=?",args.toArray());
    }
    public void delete(String table,long id) {
        find(table,id);
        if(fields(table).containsKey("deletedAt"))jdbc.update("UPDATE "+table+" SET deleted_at=CURRENT_TIMESTAMP WHERE id=?",id);
        else jdbc.update("DELETE FROM "+table+" WHERE id=?",id);
    }
    public void deleteWhere(String table,String field,Object value) {
        String condition=col(table,field)+"=?";
        if(fields(table).containsKey("deletedAt"))jdbc.update("UPDATE "+table+" SET deleted_at=CURRENT_TIMESTAMP WHERE "+condition,value);
        else jdbc.update("DELETE FROM "+table+" WHERE "+condition,value);
    }
    public void unique(String table,String field,String value,long except) {
        boolean exists=where(table,field,value).stream().anyMatch(r->n(r,"id")!=except);
        BusinessException.conflict(!exists,"编号或名称已存在");
    }
    public String number(String table,String field,String prefix) {
        long max=0;for(var r:all(table)){String v=s(r,field);if(v.startsWith(prefix))try{max=Math.max(max,Long.parseLong(v.substring(prefix.length())));}catch(NumberFormatException ignored){}}
        return prefix+String.format("%04d",max+1);
    }
}
