package com.kasinlo.erp.module.system;

import com.kasinlo.erp.persistence.ErpRepository;
import com.kasinlo.erp.common.BusinessException;
import org.springframework.stereotype.Service;
import java.util.*;

import static com.kasinlo.erp.common.Values.*;
import static com.kasinlo.erp.common.BusinessException.*;

import com.kasinlo.erp.module.system.IdentityService.Actor;

@Service
public class SystemService {
    private final ErpRepository db;
    private final IdentityService auth;

    public SystemService(ErpRepository db, IdentityService auth) {
        this.db = db;
        this.auth = auth;
    }

    public List<Map<String, Object>> users(Actor a) {
        var out = new ArrayList<Map<String, Object>>();
        Set<Long> visible = auth.visibleUsers(a);
        for (var u : db.all("sys_users")) {
            if (!visible.contains(n(u, "id"))) continue;
            var r = clean(u);
            var links = db.where("sys_user_roles", "userId", n(u, "id"));
            r.put("roleIds", links.stream().map(l -> n(l, "roleId")).toList());
            var roles = links.stream().map(l -> db.find("sys_roles", n(l, "roleId"))).toList();
            r.put("roleNames", roles.stream().map(l -> s(l, "name")).toList());
            r.put("deptName", auth.deptName(n(u, "deptId")));
            out.add(r);
        }
        return out;
    }

    public Map<String, Object> clean(Map<String, Object> r) {
        var out = new LinkedHashMap<>(r);
        out.remove("password");
        out.remove("tokenEpoch");
        out.remove("deletedAt");
        return out;
    }

    public List<Map<String, Object>> roles() {
        var out = db.all("sys_roles");
        for (var r : out) {
            r.put("menuIds", db.where("sys_role_menus", "roleId", n(r, "id")).stream().map(l -> n(l, "menuId"))
                .toList());
            r.put("userCount", db.where("sys_user_roles", "roleId", n(r, "id")).size());
        }
        return out;
    }

    public void grantable(Actor a, List<?> ids, boolean role) {
        for (Object v : ids) {
            var r = db.find(role ? "sys_roles" : "sys_menus", id(v));
            if (a.superuser()) continue;
            if (role) {
                if ("super_admin".equals(s(r, "code")) || n(r, "dataScope") < a.scope()) throw new BusinessException(403,
                    "不可授予超出自身范围的角色");
                for (var l : db.where("sys_role_menus", "roleId", id(v))) grantable(a, List.of(n(l, "menuId")),
                    false);
            } else if (!s(r, "permission").isBlank() && !a.permissions().contains(s(r, "permission"))) throw new BusinessException(403,
                "不可授予自身没有的权限");
        }
    }

    public Object handle(String module, String handler, long id, Map<String, Object> b, Map<String, String> q,
        Actor a) {
        if (module.equals("profile")) {
            switch (handler) {
                case "Info":
                    return auth.profile(a);
                case "Routes":
                    return auth.routeTree(a);
                case "Update":
                    db.update("sys_users", a.id(), pick(b, "nickname", "email", "phone", "avatar"));
                    return null;
                case "ChangePassword":
                    require(auth.passwords.matches(s(b, "oldPassword"), s(a.user(), "password")), "原密码错误");
                    require(s(b, "newPassword").length() >= 8, "新密码至少 8 位");
                    db.update("sys_users", a.id(), map("password", auth.passwords.encode(s(b, "newPassword"))));
                    auth.revoke(a.id());
                    return null;
            }
        }
        if (module.equals("log")) {
            if (handler.equals("Modules")) return List.of("system", "inv", "ops");
            String t = handler.contains("Operation") ? "sys_operation_logs" : "sys_login_logs";
            if (handler.startsWith("Clear")) {
                if (t.equals("sys_login_logs")) db.clearSuccessfulLogins();
                else db.clearOperations();
                return map("deleted", 0);
            }
            return db.all(t);
        }
        String table = switch (module) {
            case "user" -> "sys_users";
            case "role" -> "sys_roles";
            case "menu" -> "sys_menus";
            case "dept" -> "sys_depts";
            default -> throw new BusinessException(404, "模块不存在");
        };
        if (handler.equals("List") || handler.equals("Options")) return module.equals("user") ? users(a) : module.equals("role") ?
            roles() : db.all(table);
        if (handler.equals("Detail")) {
            final long detailId = id;
            if (module.equals("user")) return users(a).stream().filter(r -> n(r, "id") == detailId).findFirst()
                .orElseThrow(() -> new BusinessException(404, "用户不存在"));
            if (module.equals("role")) return roles().stream().filter(r -> n(r, "id") == detailId).findFirst().orElseThrow();
            return clean(db.find(table, id));
        }
        if (id > 0) {
            var old = db.lock(table, id);
            if (module.equals("user")) {
                if (!auth.visibleUsers(a).contains(id)) throw new BusinessException(404, "用户不存在");
                boolean target = auth.actor(id).superuser();
                if (target && !a.superuser()) throw new BusinessException(403, "不可修改超级管理员");
                if (id == 1 &&(handler.equals("Delete") || b.containsKey("status") && n(b, "status") == 0)) throw new BusinessException(409,
                    "内置管理员不可禁用或删除");
            }
            if (module.equals("role") && bool(old.get("isBuiltin")) &&(handler.equals("Delete") || b.containsKey("status") &&
                n(b, "status") == 0)) throw new BusinessException(409, "内置角色受保护");
        }
        if (handler.equals("Delete")) {
            if (module.equals("dept")) {
                conflict(db.where("sys_users", "deptId", id).isEmpty() && db.where(table, "parentId",
                    id).isEmpty(), "部门仍被引用");
            }
            if (module.equals("menu")) conflict(db.where(table, "parentId", id).isEmpty(), "请先删除子菜单");
            db.delete(table, id);
            if (module.equals("user")) db.deleteWhere("sys_user_roles", "userId", id);
            if (module.equals("role")) {
                db.deleteWhere("sys_user_roles", "roleId", id);
                db.deleteWhere("sys_role_menus", "roleId", id);
            }
            return null;
        }
        if (handler.equals("ResetPassword")) {
            require(s(b, "password").length() >= 8, "密码至少 8 位");
            db.update(table, id, map("password", auth.passwords.encode(s(b, "password"))));
            auth.revoke(id);
            return null;
        }
        if (handler.equals("UpdateStatus")) {
            require(n(b, "status") == 0 || n(b, "status") == 1, "状态非法");
            db.update(table, id, map("status", b.get("status")));
            auth.revoke(id);
            return null;
        }
        if (module.equals("user")) {
            List<?> roleIds = b.get("roleIds") instanceof List<?> l ? l : List.of();
            grantable(a, roleIds, true);
            var data = pick(b, "username", "nickname", "email", "phone", "avatar", "deptId", "status",
                "remark");
            if (handler.equals("Create")) {
                require(s(b, "username").length() >= 3 && !s(b, "nickname").isBlank(), "请填写用户名和昵称");
                require(s(b, "password").length() >= 8, "密码至少 8 位");
                db.unique(table, "username", s(b, "username"), 0);
                data.put("password", auth.passwords.encode(s(b, "password")));
                id = db.insert(table, data);
            } else db.update(table, id, data);
            if (b.containsKey("roleIds")) {
                db.deleteWhere("sys_user_roles", "userId", id);
                for (Object rid : roleIds) db.insert("sys_user_roles", map("userId", id, "roleId", rid));
            }
            if (n(b, "status") == 0 && b.containsKey("status")) auth.revoke(id);
            return map("id", id);
        }
        if (module.equals("role")) {
            List<?> mids = b.get("menuIds") instanceof List<?> l ? l : List.of();
            grantable(a, mids, false);
            if (b.containsKey("dataScope") && !a.superuser() && n(b, "dataScope") < a.scope()) throw new BusinessException(403,
                "数据范围超出自身");
            if (handler.equals("AssignMenus")) {
                db.deleteWhere("sys_role_menus", "roleId", id);
                for (Object mid : mids) db.insert("sys_role_menus", map("roleId", id, "menuId", mid));
                return null;
            }
            var data = pick(b, "name", "code", "sort", "status", "dataScope", "remark");
            require(!s(b, "name").isBlank(), "角色名称不能为空");
            if (handler.equals("Create")) {
                require(!s(b, "code").isBlank(), "角色标识不能为空");
                if ("super_admin".equals(s(b, "code"))) throw new BusinessException(403, "超级管理员角色不可创建");
                db.unique(table, "code", s(b, "code"), 0);
                data.putIfAbsent("dataScope", 1);
                id = db.insert(table, data);
            } else {
                data.remove("code");
                db.update(table, id, data);
            }
            if (b.containsKey("menuIds")) {
                db.deleteWhere("sys_role_menus", "roleId", id);
                for (Object mid : mids) db.insert("sys_role_menus", map("roleId", id, "menuId", mid));
            }
            return map("id", id);
        }
        require(!s(b, "name").isBlank(), "名称不能为空");
        long parent = n(b, "parentId");
        if (parent > 0) {
            db.find(table, parent);
            Set<Long> seen = new HashSet<>();
            long cursor = parent;
            while (cursor > 0) {
                conflict(cursor != id && seen.add(cursor), "不可选择自身或子级作为上级");
                cursor = n(db.find(table, cursor), "parentId");
            }
        }
        var data = new LinkedHashMap<>(b);
        if (module.equals("menu")) {
            db.unique(table, "name", s(b, "name"), id);
            if (!a.superuser() && !s(b, "permission").isBlank()) auth.require(a, s(b, "permission"));
        }
        if (handler.equals("Create")) id = db.insert(table, data);
        else db.update(table, id, data);
        return map("id", id);
    }

    public static Map<String, Object> pick(Map<String, Object> b, String...keys) {
        var out = new LinkedHashMap<String, Object>();
        for (String k : keys) if (b.containsKey(k)) out.put(k, b.get(k));
        return out;
    }
}
