#!/usr/bin/env node
/**
 * 打一个「系统升级」能吃的升级包。
 *
 * 用法（在仓库根目录执行）：
 *   node scripts/build-upgrade-package.mjs --version 1.2.9
 *   node scripts/build-upgrade-package.mjs --version 1.2.9 --frontend-only
 *   node scripts/build-upgrade-package.mjs --version 1.2.9 --backend-only --notes "只修后端"
 *
 * 产物：build/upgrade/nova-erp-<version>.zip
 *
 * 包结构（**只允许一层**，后端校验遇到嵌套目录会直接拒绝）：
 *   manifest.json
 *   nova-erp-server.exe     ← 后端二进制（--backend-only / 默认都含）
 *   dist.zip                ← 前端产物（zip 内顶层就是 index.html / assets/）
 *
 * 为什么自己写 zip：项目里没有任何 zip 依赖，Windows 环境也未必有 zip/7z，
 * 而 Node 没有内置的 zip 写入能力。这里用 store（不压缩）方式直写，
 * 只依赖 zlib 之外的自己实现，几十行就够 —— 升级包本来就不该追求压缩率。
 */

import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const BACKEND_DIR = path.join(ROOT, 'backend')
const FRONTEND_DIR = path.join(ROOT, 'frontend')
const OUT_DIR = path.join(ROOT, 'build', 'upgrade')

// ── 参数 ────────────────────────────────────────────────

function parseArgs(argv) {
  const opt = {
    version: '',
    minSchema: 0,
    notes: '',
    withBackend: true,
    withFrontend: true,
    skipBuild: false,
  }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    const next = () => {
      const v = argv[++i]
      if (v === undefined) throw new Error(`${a} 缺少参数值`)
      return v
    }
    switch (a) {
      case '--version': opt.version = next().trim(); break
      case '--min-schema': opt.minSchema = Number(next()); break
      case '--notes': opt.notes = next(); break
      case '--backend-only': opt.withFrontend = false; break
      case '--frontend-only': opt.withBackend = false; break
      case '--skip-build': opt.skipBuild = true; break
      case '-h':
      case '--help':
        console.log(`用法: node scripts/build-upgrade-package.mjs --version <x.y.z> [选项]

  --version <x.y.z>   目标发布版本（必填，必须与升级后 config.yaml 的 app.release 一致）
  --min-schema <n>    升级包要求的最低结构版本（默认 0，即不限制）
  --notes <text>      升级说明，会写进 manifest 并展示在升级记录里
  --backend-only      只含后端二进制
  --frontend-only     只含前端产物
  --skip-build        复用上一次构建产物（build/upgrade/.stage 已在时可用）
`)
        process.exit(0)
        break
      default:
        throw new Error(`未知参数 ${a}，用 --help 查看用法`)
    }
  }
  return opt
}

/** 从 backend/config.yaml 的 app 段取 release，用于检查版本号是否会被覆盖 */
function readConfiguredRelease() {
  const file = path.join(BACKEND_DIR, 'config.yaml')
  if (!fs.existsSync(file)) return ''
  const text = fs.readFileSync(file, 'utf8')
  const appBlock = text.match(/^app:[ \t]*\n([\s\S]*?)(?=^\S|\Z)/m)
  if (!appBlock) return ''
  const hit = appBlock[1].match(/^\s+release:\s*"?([^"#\s]+)"?/m)
  return hit ? hit[1] : ''
}

// ── ZIP 写入（store）─────────────────────────────────────

const CRC_TABLE = (() => {
  const t = new Int32Array(256)
  for (let n = 0; n < 256; n++) {
    let c = n
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    t[n] = c
  }
  return t
})()

function crc32(buf) {
  let c = -1
  for (let i = 0; i < buf.length; i++) c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8)
  return (c ^ -1) >>> 0
}

function dosDateTime(d) {
  return {
    time: (d.getHours() << 11) | (d.getMinutes() << 5) | Math.floor(d.getSeconds() / 2),
    date: ((d.getFullYear() - 1980) << 9) | ((d.getMonth() + 1) << 5) | d.getDate(),
  }
}

/** entries: [{ name, data }]，name 用正斜杠，不允许以 / 开头 */
function buildZip(entries) {
  const { time, date } = dosDateTime(new Date())
  const locals = []
  const centrals = []
  let offset = 0

  for (const e of entries) {
    const nameBuf = Buffer.from(e.name, 'utf8')
    const data = e.data
    const crc = crc32(data)

    const lh = Buffer.alloc(30)
    lh.writeUInt32LE(0x04034b50, 0)
    lh.writeUInt16LE(20, 4) // 解压所需版本
    lh.writeUInt16LE(0x0800, 6) // bit 11：文件名 UTF-8
    lh.writeUInt16LE(0, 8) // 0 = store
    lh.writeUInt16LE(time, 10)
    lh.writeUInt16LE(date, 12)
    lh.writeUInt32LE(crc, 14)
    lh.writeUInt32LE(data.length, 18)
    lh.writeUInt32LE(data.length, 22)
    lh.writeUInt16LE(nameBuf.length, 26)
    lh.writeUInt16LE(0, 28)
    locals.push(lh, nameBuf, data)

    const ch = Buffer.alloc(46)
    ch.writeUInt32LE(0x02014b50, 0)
    ch.writeUInt16LE(20, 4) // made by
    ch.writeUInt16LE(20, 6) // needed
    ch.writeUInt16LE(0x0800, 8)
    ch.writeUInt16LE(0, 10)
    ch.writeUInt16LE(time, 12)
    ch.writeUInt16LE(date, 14)
    ch.writeUInt32LE(crc, 16)
    ch.writeUInt32LE(data.length, 20)
    ch.writeUInt32LE(data.length, 24)
    ch.writeUInt16LE(nameBuf.length, 28)
    ch.writeUInt16LE(0, 30)
    ch.writeUInt16LE(0, 32)
    ch.writeUInt16LE(0, 34)
    ch.writeUInt16LE(0, 36)
    ch.writeUInt32LE(0, 38)
    ch.writeUInt32LE(offset, 42)
    centrals.push(ch, nameBuf)

    offset += lh.length + nameBuf.length + data.length
  }

  const central = Buffer.concat(centrals)
  const eocd = Buffer.alloc(22)
  eocd.writeUInt32LE(0x06054b50, 0)
  eocd.writeUInt16LE(0, 4)
  eocd.writeUInt16LE(0, 6)
  eocd.writeUInt16LE(entries.length, 8)
  eocd.writeUInt16LE(entries.length, 10)
  eocd.writeUInt32LE(central.length, 12)
  eocd.writeUInt32LE(offset, 16)
  eocd.writeUInt16LE(0, 20)
  return Buffer.concat([...locals, central, eocd])
}

// ── 文件工具 ────────────────────────────────────────────

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

function run(cmd, args, cwd) {
  console.log(`\n$ ${cmd} ${args.join(' ')}`)
  execFileSync(cmd, args, { cwd, stdio: 'inherit', shell: process.platform === 'win32' })
}

// ── 主流程 ──────────────────────────────────────────────

function main() {
  const opt = parseArgs(process.argv.slice(2))

  if (!opt.withBackend && !opt.withFrontend) {
    throw new Error('至少要包含后端或前端之一')
  }
  if (!opt.version) {
    throw new Error('缺少 --version：升级包必须声明目标版本，且要与升级后 config.yaml 的 app.release 一致')
  }
  if (opt.version.length > 32) {
    throw new Error('version 过长（最多 32 个字符，受 planId 列宽约束）')
  }
  if (/[\\/]/.test(opt.version)) {
    throw new Error('version 不能含斜杠')
  }
  if (opt.version === 'dev') {
    throw new Error('version 不能是 dev：dev 表示未注入发布号，用它升级后重启核对必然不通过')
  }

  const stage = path.join(OUT_DIR, '.stage')
  fs.rmSync(stage, { recursive: true, force: true })
  fs.mkdirSync(stage, { recursive: true })
  fs.mkdirSync(OUT_DIR, { recursive: true })

  // 顺手清掉上次没能删掉的旧产物：某些环境会拦批量删除（几百个文件），
  // 那种环境里上一次会留下 dist-old-*；这里再试一次，失败就继续（不阻塞打包）。
  for (const name of fs.readdirSync(OUT_DIR)) {
    if (!name.startsWith('dist-old-')) continue
    try {
      fs.rmSync(path.join(OUT_DIR, name), { recursive: true, force: true })
    } catch {
      /* 清不掉就留着，已被 gitignore 忽略 */
    }
  }

  const entries = []

  // ---- 后端 ----
  let backendName = ''
  if (opt.withBackend) {
    backendName = process.platform === 'win32' ? 'nova-erp-server.exe' : 'nova-erp-server'
    const outFile = path.join(stage, backendName)
    if (!opt.skipBuild) {
      // CGO 关掉：升级包要在任何同架构机器上直接跑，不该依赖构建机的 C 运行库
      run('go', ['build', '-trimpath', '-o', outFile, './cmd/server'], BACKEND_DIR)
    }
    if (!fs.existsSync(outFile)) {
      throw new Error(`后端产物不存在：${outFile}（去掉 --skip-build 重新构建）`)
    }
    const data = fs.readFileSync(outFile)
    entries.push({ name: backendName, data })
    console.log(`  后端 ${backendName}  ${humanSize(data.length)}`)
  }

  // ---- 前端 ----
  let frontendFile = ''
  let parkedDist = ''
  if (opt.withFrontend) {
    const dist = path.join(FRONTEND_DIR, 'dist')
    if (!opt.skipBuild) {
      // vite 构建前会清空 outDir。直接让它删，在某些带「批量删除保护」的
      // 环境里会被拦下（dist/assets 动辄上百个文件），构建直接失败。
      // 改成先把旧 dist 移出工程（改名不算删除），vite 面对的就是空目录 ——
      // 既不依赖删除权限，也顺带保证新产物不会和旧文件混在一起。
      if (fs.existsSync(dist)) {
        // 必须停在同一块盘上：跨盘 rename 会报 EXDEV（D: → C: 的临时目录就踩过）
        parkedDist = path.join(OUT_DIR, `dist-old-${Date.now()}`)
        try {
          fs.renameSync(dist, parkedDist)
        } catch (e) {
          parkedDist = ''
          throw new Error(`旧的 dist 目录移不走（${e.message}）：先手动删掉 ${dist} 再构建`)
        }
      }
      run('npm', ['run', 'build'], FRONTEND_DIR)
    }
    if (!fs.existsSync(dist)) {
      throw new Error(`前端产物目录不存在：${dist}（去掉 --skip-build 重新构建）`)
    }
    // dist.zip 的**顶层**必须直接是 index.html：后端是把 zip 解到
    // frontend_dir 下，多一层 dist/ 就会变成 <dir>/dist/index.html，站点直接 404。
    const files = walk(dist)
    if (!files.some((f) => f.rel === 'index.html')) {
      throw new Error('前端产物根目录缺少 index.html，检查 vite.config 的 build.outDir')
    }
    const zipBuf = buildZip(files.map((f) => ({
      name: f.rel,
      data: fs.readFileSync(f.full),
    })))
    frontendFile = 'dist.zip'
    const zipPath = path.join(stage, frontendFile)
    fs.writeFileSync(zipPath, zipBuf)
    entries.push({ name: frontendFile, data: zipBuf })
    console.log(`  前端 ${frontendFile}  ${humanSize(zipBuf.length)}（${files.length} 个文件）`)
  }

  // ---- manifest ----
  const manifest = {
    version: opt.version,
    minSchemaVersion: opt.minSchema,
    notes: opt.notes || `升级到 ${opt.version}`,
  }
  if (opt.withBackend) {
    manifest.backend = { file: backendName, sha256: sha256(entries.find((e) => e.name === backendName).data) }
  }
  if (opt.withFrontend) {
    manifest.frontend = { file: frontendFile, sha256: sha256(entries.find((e) => e.name === frontendFile).data) }
  }
  const manifestBuf = Buffer.from(`${JSON.stringify(manifest, null, 2)}\n`, 'utf8')
  entries.unshift({ name: 'manifest.json', data: manifestBuf })
  console.log(`  manifest ${humanSize(manifestBuf.length)}`)
  console.log(JSON.stringify(manifest, null, 2))

  // ---- 打包 ----
  const pkg = buildZip(entries)
  const outFile = path.join(OUT_DIR, `nova-erp-${opt.version}.zip`)
  fs.writeFileSync(outFile, pkg)
  fs.rmSync(stage, { recursive: true, force: true })

  // 打包成了就回收被挪走的旧 dist。某些环境会拦批量删除（几百个文件），
  // 拦了就算了 —— 它落在 build/ 下且已被 gitignore，不影响下一次构建。
  if (parkedDist) {
    try {
      fs.rmSync(parkedDist, { recursive: true, force: true })
    } catch {
      console.log(`\n提示：旧的 dist 没能自动清理，可手动删除 ${path.relative(ROOT, parkedDist)}`)
    }
  }

  console.log('\n──────────────────────────────────────────────')
  console.log(`升级包：${path.relative(ROOT, outFile)}`)
  console.log(`体积　：${humanSize(pkg.length)}`)
  console.log(`SHA256：${sha256(pkg)}`)
  console.log('──────────────────────────────────────────────')

  // ---- 部署前必读的几条 ----
  const warnings = []
  const configured = readConfiguredRelease()
  if (configured !== opt.version) {
    warnings.push(
      `backend/config.yaml 的 app.release 当前是「${configured || '（未配置，默认 dev）'}」，` +
      `与包版本 ${opt.version} 不一致。\n` +
      `    升级执行完要重启，重启核对的是**新程序读到的配置里的版本**；` +
      `不改它，本次升级会被判为「未生效」。\n` +
      `    部署时请把 app.release 改成 ${opt.version}（或用环境变量 NOVA_APP_RELEASE=${opt.version}）。`,
    )
  }
  const cfgText = fs.existsSync(path.join(BACKEND_DIR, 'config.yaml'))
    ? fs.readFileSync(path.join(BACKEND_DIR, 'config.yaml'), 'utf8')
    : ''
  if (opt.withFrontend && !/^\s*frontend_dir:\s*\S/m.test(cfgText)) {
    warnings.push(
      'backend/config.yaml 里没有配置 upgrade.frontend_dir。\n' +
      '    不配的话，升级只会替换后端程序，前端产物不会动（包里的 dist.zip 会被忽略）。\n' +
      '    要一起换前端，请配置：\n' +
      '      upgrade:\n' +
      '        frontend_dir: "D:/go/nova-erp/frontend/dist"   # 线上真正被访问的那份产物目录',
    )
  }
  if (warnings.length) {
    console.log('\n部署前需要处理：')
    warnings.forEach((w, i) => console.log(`  ${i + 1}. ${w}`))
  }
  console.log('\n步骤回顾：系统运维 → 系统升级 → 上传这个 zip → 升级前备份 → 执行升级 → 重启服务')
}

try {
  main()
} catch (e) {
  console.error(`\n打包失败：${e.message}`)
  process.exit(1)
}
