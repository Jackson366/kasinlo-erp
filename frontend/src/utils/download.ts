import type { AxiosRequestConfig } from 'axios'
import instance, { ApiError } from '@/utils/request'
import type { ApiResult } from '@/types/api'

/**
 * 文件下载与上传的封装。
 *
 * 为什么不能直接复用 utils/request.ts 里的 http.* ？
 * 因为 http.* 全部经过 unwrap()，它假定响应体是 { code, message, data } 的 JSON。
 * 导出接口返回的是二进制流，unwrap 会取不到 body.code 而直接抛错。
 * 所以这里直接使用原始的 axios 实例。
 */

/** 触发浏览器下载。走 Blob + <a download>，无需后端配合或额外依赖。 */
function triggerDownload(blob: Blob, filename: string): void {
  const href = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = href
  link.download = filename
  link.style.display = 'none'
  document.body.appendChild(link)
  link.click()
  link.remove()
  // 立刻 revoke 会让部分浏览器（Safari）拿不到数据，延迟释放
  window.setTimeout(() => URL.revokeObjectURL(href), 1000)
}

/**
 * 解析 Content-Disposition 中的文件名。
 * 后端同时给出 filename（ASCII 回退名）与 filename*（RFC 5987 UTF-8），优先取后者。
 */
export function parseFilename(disposition?: string): string | undefined {
  if (!disposition) return undefined

  const utf8 = /filename\*=UTF-8''([^;]+)/i.exec(disposition)
  if (utf8?.[1]) {
    try {
      return decodeURIComponent(utf8[1])
    } catch {
      // 编码异常时退回下面的普通 filename
    }
  }
  const plain = /filename="?([^";]+)"?/i.exec(disposition)
  return plain?.[1]
}

/** 下载文件并触发浏览器保存 */
export async function downloadFile(
  url: string,
  params?: Record<string, unknown>,
  fallbackName = 'export.xlsx',
): Promise<void> {
  try {
    const response = await instance.request<Blob>({
      url,
      method: 'GET',
      params,
      responseType: 'blob',
    })
    const disposition = response.headers['content-disposition'] as string | undefined
    triggerDownload(response.data, parseFilename(disposition) || fallbackName)
  } catch (error) {
    // 失败了什么也没发生，给用户一个兜底提示（具体业务原因由响应拦截器负责弹出）
    if (error instanceof ApiError) throw error
    throw new ApiError(-1, '下载失败，请稍后重试')
  }
}

/** 上传文件（multipart/form-data），返回解包后的业务数据 */
export async function uploadFile<T>(
  url: string,
  file: File,
  options: { params?: Record<string, unknown>; config?: AxiosRequestConfig } = {},
): Promise<T> {
  const form = new FormData()
  form.append('file', file)

  const response = await instance.request<ApiResult<T>>({
    url,
    method: 'POST',
    data: form,
    params: options.params,
    // 必须让 axios 用 multipart 而不是实例默认的 application/json；
    // boundary 由浏览器自动补充，不能手动写死。
    headers: { 'Content-Type': 'multipart/form-data' },
    ...options.config,
  })

  const body = response.data
  if (body.code !== 0) {
    throw new ApiError(body.code, body.message, body.traceId)
  }
  return body.data
}
