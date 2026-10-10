#!/usr/bin/env bash
# 重新生成后端的 OpenAPI（Swagger）文档。
#
# 何时需要执行：
#   - 新增 / 删除 / 修改了任意 handler 的 swag 注解（@Summary / @Param / @Success ...）
#   - 修改了 cmd/server/main.go 顶部的通用注解（title / BasePath / 安全定义等）
#   - 调整了 dto 类型，希望文档里的字段跟着变
#
# 生成物（backend/api/docs/）需要一并提交：这样没有装 swag 的环境也能直接跑起文档。
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
backend_dir="${repo_root}/backend"

if ! command -v swag >/dev/null 2>&1; then
  cat >&2 <<'EOF'
未找到 swag 命令。安装方式：

  go install github.com/swaggo/swag/cmd/swag@v1.16.4

（安装后在 $GOPATH/bin 下，确保该目录在 PATH 中）
EOF
  exit 1
fi

cd "${backend_dir}"

# --parseInternal 必须加：本项目的业务代码全部位于 internal/ 下，
#   不加的话 swag 会跳过它们，生成的文档只剩通用信息、一个接口都没有。
echo "[swag] 生成 OpenAPI 文档 -> backend/api/docs"
swag init \
  -g cmd/server/main.go \
  -o api/docs \
  --parseInternal \
  --instanceName swagger \
  -ot go,json,yaml \
  --quiet

echo "[swag] 完成："
ls -1 api/docs
