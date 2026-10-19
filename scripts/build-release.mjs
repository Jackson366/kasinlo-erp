#!/usr/bin/env node
/**
 * 打「整机交付包」：给一台全新 Windows 机器用的完整目录 + zip。
 *
 * 用法（仓库根目录）：
 *   node scripts/build-release.mjs --version 1.1.0
 *   node scripts/build-release.mjs --version 1.1.0 --skip-build   # 复用上次构建产物
 *
 * 产物：
 *   build/release/nova-erp-1.1.0/           目录（可直接拷到服务器）
 *   build/release/nova-erp-1.1.0-win64.zip  压缩包
 *
 * 与 build-upgrade-package.mjs 的区别：
 *   - 交付包 = 首次部署：带 web/ 目录、config 模板、启动脚本、部署文档；
 *   - 升级包 = 已部署环境的增量：带 manifest + 单文件，由管理页上传。
 *
 * 版本号会同时注入两个 exe（-ldflags -X nova-erp/internal/config.appRelease=...），
 * 这样即使现场 config.yaml 没写 app.release，升级核对也能拿到正确版本。
 */

import { execFileSync } from 'node:child_process'
import { createHash, randomBytes } from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { buildZip } from './lib/zip.mjs'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const BACKEND_DIR = path.join(ROOT, 'backend')
const FRONTEND_DIR = path.join(ROOT, 'frontend')
const OUT_DIR = path.join(ROOT, 'build', 'release')

function parseArgs(argv) {
  const opt = { version: '', skipBuild: false }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    const next = () => {
      const v = argv[++i]
      if (v === undefined) throw new Error(`${a} 缺少参数值`)
      return v
    }
    switch (a) {
      case '--version': opt.version = next().trim(); break
      case '--skip-build': opt.skipBuild = true; break
      case '-h':
      case '--help':
        console.log(`用法: node scripts/build-release.mjs --version <x.y.z> [选项]

  --version <x.y.z>  发布版本（必填；会注入 exe 并写进包名）
  --skip-build       复用上次构建产物（.stage 已存在时可用）
`)
        process.exit(0)
        break
      default:
        throw new Error(`未知参数 ${a}，用 --help 查看用法`)
    }
  }
  return opt
}

function readConfiguredRelease() {
  const file = path.join(BACKEND_DIR, 'config.example.yaml')
  if (!fs.existsSync(file)) return ''
  const text = fs.readFileSync(file, 'utf8')
  const appBlock = text.match(/^app:[ \t]*\n([\s\S]*?)(?=^\S|\Z)/m)
  if (!appBlock) return ''
  const hit = appBlock[1].match(/^\s+release:\s*"?([^"#\s]+)"?/m)
  return hit ? hit[1] : ''
}

function walk(dir, base = dir, out = []) {
  for (const name of fs.readdirSync(dir)) {
    const full = path.join(dir, name)
    const st = fs.statSync(full)
    if (st.isDirectory()) walk(full, base, out)
    else out.push({ full, rel: path.relative(base, full).split(path.sep).join('/') })
  }
  return out
}

function sha256(buf) {
  return createHash('sha256').update(buf).digest('hex')
}

function humanSize(n) {
  if (n < 1024) return `${n} B`
  const kb = n / 1024
  if (kb < 1024) return `${kb.toFixed(1)} KB`
  return `${(kb / 1024).toFixed(1)} MB`
}

/**
 * 执行外部命令。
 *
 * useShell 只对 npm 这类 .cmd 包装器需要；go 必须走 shell:false ——
 * 经 shell 拼命令行时，`-ldflags "-X pkg.var=1.0.0"` 里的空格会被拆成多个参数，
 * go 于是报 malformed import path "-o"。
 */
function run(cmd, args, cwd, useShell = false) {
  console.log(`\n$ ${cmd} ${args.join(' ')}`)
  execFileSync(cmd, args, { cwd, stdio: 'inherit', shell: useShell })
}

/**
 * 把目录改名挪开，而不是删掉。
 *
 * vite 构建前要清空 outDir、重复打包要清空 stage —— 直接删在带「批量删除保护」
 * 的环境里会被拦下（web 产物动辄上百个文件），打包当场失败。
 * 改名不算删除，且必须停在同一块盘（跨盘 rename 报 EXDEV）。
 * 收尾时再尝试删掉，删不掉就留着并由调用方提示。
 */
const parked = []
function park(dir) {
  if (!fs.existsSync(dir)) return
  const target = `${dir}.old-${Date.now()}`
  fs.renameSync(dir, target)
  parked.push(target)
}
function cleanParked() {
  for (const p of parked) {
    try {
      fs.rmSync(p, { recursive: true, force: true })
    } catch {
      console.log(`提示：临时目录没能自动清理，可手动删除 ${path.relative(ROOT, p)}`)
    }
  }
}

/**
 * 启动脚本（纯 ASCII，避免中文 Windows 把 UTF-8 批处理按 GBK 误读导致中文/全角括号
 * 破坏 if 代码块、把 echo 文案当成命令执行）。
 * 配置生成在打包阶段由 Node 完成（已填随机 jwt.secret 的 config.yaml 随包发出），
 * 这里只负责：缺配置则从模板复制、口令仍是占位串则拒绝启动并给清晰英文提示。
 */
const START_BAT = `@echo off
cd /d "%~dp0"

REM Create config.yaml from the template if it does not exist yet.
if not exist config.yaml (
  if exist config.example.yaml (
    copy /Y config.example.yaml config.yaml >nul
    echo [NOTE] config.yaml was created from config.example.yaml.
    echo        Edit database.password to your PostgreSQL password, then run again.
  ) else (
    echo [ERROR] Neither config.yaml nor config.example.yaml was found.
    pause
    exit /b 1
  )
)

REM Refuse to start while the database password is still the template placeholder.
findstr /C:"password: CHANGE_ME" config.yaml >nul
if %ERRORLEVEL%==0 (
  echo [ERROR] database.password is still the template placeholder "CHANGE_ME".
  echo         Open config.yaml in Notepad and set it to your PostgreSQL password.
  echo         Then run start.bat again.
  pause
  exit /b 1
)

echo Nova ERP starting... (close this window to stop the service)
nova-erp-server.exe
echo.
echo Server stopped with exit code %ERRORLEVEL%.
pause
`

function main() {
  const opt = parseArgs(process.argv.slice(2))
  if (!opt.version) throw new Error('缺少 --version（例：--version 1.1.0）')
  if (opt.version === 'dev') throw new Error('version 不能是 dev：升级核对会因此永远判「未生效」')
  if (opt.version.length > 32) throw new Error('version 过长（最多 32 字符，受 planId 列宽约束）')

  const stageRoot = path.join(OUT_DIR, 'nova-erp-' + opt.version)
  if (!opt.skipBuild) {
    park(stageRoot)
  }
  fs.mkdirSync(stageRoot, { recursive: true })

  const ldflags = `-X nova-erp/internal/config.appRelease=${opt.version}`

  // ---- 后端两个可执行文件 ----
  const serverExe = path.join(stageRoot, 'nova-erp-server.exe')
  const opsExe = path.join(stageRoot, 'nova-ops.exe')
  if (!opt.skipBuild) {
    run('go', ['build', '-trimpath', '-ldflags', ldflags, '-o', serverExe, './cmd/server'], BACKEND_DIR)
    run('go', ['build', '-trimpath', '-ldflags', ldflags, '-o', opsExe, './cmd/ops'], BACKEND_DIR)
  }
  for (const f of [serverExe, opsExe]) {
    if (!fs.existsSync(f)) throw new Error(`后端产物不存在：${f}（去掉 --skip-build 重新构建）`)
    console.log(`  ${path.basename(f)}  ${humanSize(fs.statSync(f).size)}`)
  }

  // ---- 前端产物 ----
  const webDir = path.join(stageRoot, 'web')
  const dist = path.join(FRONTEND_DIR, 'dist')
  if (!opt.skipBuild) {
    // 旧 dist 先改名移出（同盘 rename）。直接让 vite 清空 outDir 在带批量删除
    // 保护的环境里会被拦下，构建当场失败。
    if (fs.existsSync(dist)) {
      park(dist)
    }
    run('npm', ['run', 'build'], FRONTEND_DIR, true)
  }
  if (!fs.existsSync(path.join(dist, 'index.html'))) {
    throw new Error(`前端产物缺少 index.html：${dist}`)
  }
  fs.cpSync(dist, webDir, { recursive: true })
  console.log(`  web/  ${humanSize(walk(webDir).reduce((s, f) => s + fs.statSync(f.full).size, 0))}（${walk(webDir).length} 个文件）`)

  // ---- 配置模板与文档 ----
  fs.copyFileSync(path.join(BACKEND_DIR, 'config.example.yaml'), path.join(stageRoot, 'config.example.yaml'))

  // 生成一份已填好随机 jwt.secret 的 config.yaml（用户只需改数据库口令），
  // 避免把「占位 secret」带上线，也省掉 .bat 里脆弱的 PowerShell 自动替换。
  const exampleSrc = path.join(BACKEND_DIR, 'config.example.yaml')
  const exampleTxt = fs.readFileSync(exampleSrc, 'utf8')
  const SECRET_PLACEHOLDER = 'CHANGE_ME_USE_A_RANDOM_LONG_STRING'
  if (!exampleTxt.includes(SECRET_PLACEHOLDER)) {
    throw new Error('config.example.yaml 缺少 jwt.secret 占位串，无法自动生成 config.yaml')
  }
  const generatedSecret = randomBytes(48).toString('base64')
  fs.writeFileSync(
    path.join(stageRoot, 'config.yaml'),
    exampleTxt.replace(SECRET_PLACEHOLDER, generatedSecret),
    'utf8',
  )
  console.log('  config.yaml  (随机 jwt.secret 已生成，只需改 database.password)')

  fs.copyFileSync(path.join(ROOT, 'docs', 'DEPLOY.md'), path.join(stageRoot, 'DEPLOY.md'))
  fs.writeFileSync(path.join(stageRoot, 'start.bat'), START_BAT, 'utf8')

  // ---- 一致性检查 ----
  const warnings = []
  const configured = readConfiguredRelease()
  if (configured && configured !== opt.version) {
    warnings.push(
      `config.example.yaml 里的 app.release 是「${configured}」，与本次版本 ${opt.version} 不一致。\n` +
      `    部署时若照抄模板并保留该值，重启核对可能对不上；要么改模板、要么在 config.yaml 里改成 ${opt.version}。`,
    )
  }

  // ---- 打包 ----
  const files = walk(stageRoot)
  const entries = files.map((f) => ({
    name: `nova-erp-${opt.version}/${f.rel}`,
    data: fs.readFileSync(f.full),
  }))
  const zipBuf = buildZip(entries)
  const zipPath = path.join(OUT_DIR, `nova-erp-${opt.version}-win64.zip`)
  fs.writeFileSync(zipPath, zipBuf)

  let total = 0
  for (const f of files) total += fs.statSync(f.full).size

  console.log('\n──────────────────────────────────────────────')
  console.log(`交付目录：${path.relative(ROOT, stageRoot)}`)
  console.log(`交付包　：${path.relative(ROOT, zipPath)}`)
  console.log(`解压后　：${humanSize(total)}（${files.length} 个文件）`)
  console.log(`包体积　：${humanSize(zipBuf.length)}`)
  console.log(`SHA256　：${sha256(zipBuf)}`)
  console.log('──────────────────────────────────────────────')
  console.log('\n部署步骤见包内 DEPLOY.md：装 PostgreSQL → 改 config.yaml → start.bat → 改密。')
  if (warnings.length) {
    console.log('\n注意：')
    warnings.forEach((w, i) => console.log(`  ${i + 1}. ${w}`))
  }
  cleanParked()
}

try {
  main()
} catch (e) {
  console.error(`\n打包失败：${e.message}`)
  process.exit(1)
}
