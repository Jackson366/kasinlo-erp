#!/usr/bin/env bash
#
# 框架层不得依赖具体模块。
#
# 为什么值得一条独立门禁：「业务模块能否整块删掉」取决于框架层认不认识它。
# 只要有一个框架包 import 了 user/role/invflow，那个模块就再也删不掉了 ——
# 而且是编译期就报错的那种「删不掉」，比「不敢删」更硬。
#
# 唯一允许 import 具体模块的位置是 internal/assembly：它就是为了
# 「把模块塞进登记表」而存在的组装层。除此之外一律视为越界。
#
# 比 UPGRADE.md 里最初设想的 grep 更强的地方：那条 grep 只能盯业务模块名，
# 这条把「框架包引用 internal/module/<任意子包>」整体禁掉 ——
# 新增模块不必回来维护白名单，也顺带覆盖了系统模块。
set -uo pipefail

cd "$(dirname "$0")/.."

FRAMEWORK_PATHS=(
  internal/pkg
  internal/authz
  internal/middleware
  internal/oplog
  internal/database
  internal/seed
  internal/router
  internal/bootstrap
  internal/model
  internal/config
  internal/logger
  internal/module
  cmd
)

violations=$(grep -rn '"nova-admin/internal/module/' \
  "${FRAMEWORK_PATHS[@]}" 2>/dev/null \
  | grep -v '^internal/assembly/' \
  | grep -v '_test\.go:' || true)

if [ -n "$violations" ]; then
  echo "❌ 框架层出现了对具体模块的依赖（只有 internal/assembly 允许）："
  echo
  echo "$violations"
  echo
  echo "改法二选一："
  echo "  1. 需要清单/路由时，用 internal/module 的契约类型当参数传进来；"
  echo "  2. 需要装配时，在 internal/assembly 里加一行。"
  exit 1
fi

echo "✅ 框架层对具体模块零依赖（internal/assembly 是唯一的组装层）"
