#!/usr/bin/env python3
"""重新生成 scripts/fixtures/users-import.xlsx（用户导入接口的测试样例）。

为什么把样例固化成文件而不是运行期动态生成：
   接口自测脚本（scripts/api-smoke.mjs）要求「零依赖、随时可跑」，
   而现造一个合法的 xlsx 需要 zip + XML 拼装（或引入第三方库）。
   固化成静态样例后，冒烟脚本只需读取字节流上传即可。

这份样例刻意全部使用 dryRun 校验（不写库），因此可以反复执行：
  - 第 2 行：合法行 → 校验通过
  - 第 3 行：与第 2 行用户名重复 → 文件内重复
  - 第 4 行：邮箱格式非法
  - 第 5 行：状态取值非法
  - 第 6 行：角色列用不同分隔符把同一个角色写多遍 → 应当去重后通过
    （不去重会触发 sys_user_roles 复合主键冲突，让整批导入回滚）
表头必须与后端 internal/module/user/excel.go 的列定义保持一致。

用法：python scripts/fixtures/build-users-import.py
"""
import os

import openpyxl

HEADERS = ["用户名", "昵称", "密码", "邮箱", "手机号", "部门", "角色", "状态", "备注"]
ROWS = [
    ["smoke_import_ok", "冒烟合法行", "", "smoke_import@example.com", "", "", "", "启用", "冒烟样例"],
    ["smoke_import_ok", "文件内重名", "", "", "", "", "", "", ""],
    ["smoke_import_bad_email", "邮箱非法", "", "not-an-email", "", "", "", "", ""],
    ["smoke_import_bad_status", "状态非法", "", "", "", "", "", "待定", ""],
    ["smoke_import_dup_role", "角色重复书写", "", "", "", "", "只读访客、只读访客, 只读访客", "启用", ""],
]

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "users-import.xlsx")

wb = openpyxl.Workbook()
ws = wb.active
ws.title = "用户导入"
ws.append(HEADERS)
for row in ROWS:
    ws.append(row)
wb.save(OUT)
print("已生成", OUT, "数据行数:", len(ROWS))
