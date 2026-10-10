/**
 * 后端接口地址的唯一来源。
 *
 * 后端前缀由 config.yaml 的 app.prefix + app.version 拼成（默认 /api + /v1）。
 * 此前 "/v1" 硬编码在 request.ts、download.ts 与每个 api/*.ts 里：后端一旦把
 * app.version 改成 v2（或前缀改走网关），前端所有请求会**静默 404**，
 * 既没有构建期报错，运行时也只是「什么都没返回」。
 *
 * 现在收敛成这一处，并允许通过环境变量覆盖：
 *   VITE_API_BASE_URL  —— 网关前缀，默认 /api
 *   VITE_API_VERSION   —— 版本段，默认 v1
 * 二者拼成 api 实例的 baseURL（例如 /api/v1），各 api 模块只写「/auth/login」这类相对路径。
 */
const rawBase = import.meta.env.VITE_API_BASE_URL || '/api'
const rawVersion = import.meta.env.VITE_API_VERSION || 'v1'

const trimmedBase = rawBase.replace(/\/+$/, '')
const trimmedVersion = rawVersion.replace(/^\/+|\/+$/g, '')

/** 完整的接口前缀，例如 /api/v1 */
export const API_PREFIX = `${trimmedBase}/${trimmedVersion}`
