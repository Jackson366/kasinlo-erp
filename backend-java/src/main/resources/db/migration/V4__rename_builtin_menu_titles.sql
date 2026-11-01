-- 内置菜单改名：与 Go 后端 seed.menuRenames 保持一致。
-- 只改「标题仍是旧内置值」的行：管理员在菜单管理里自定义过的标题保持不动；
-- 条件里带旧标题，重复执行无副作用。
UPDATE sys_menus SET title = '首页', updated_at = NOW()
WHERE name = 'InvOverview' AND title = '数据概览' AND deleted_at IS NULL;

UPDATE sys_menus SET title = '系统', updated_at = NOW()
WHERE name = 'System' AND title = '系统管理' AND deleted_at IS NULL;
