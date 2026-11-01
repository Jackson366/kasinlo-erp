import { MAX_STAMP_BYTES } from './printConfig'

/** Decode and re-encode locally: retain alpha, strip metadata, never persist a temporary blob URL. */
export async function readStampImage(file: File): Promise<string> {
  if (
    !file.name.toLowerCase().endsWith('.png') ||
    !file.size ||
    file.size > MAX_STAMP_BYTES
  ) {
    throw new Error('请选择不超过 256 KB 的透明背景 PNG 图片')
  }
  const bytes = new Uint8Array(await file.arrayBuffer())
  const signature = [137, 80, 78, 71, 13, 10, 26, 10]
  if (!signature.every((n, i) => bytes[i] === n) || bytes.length < 24) {
    throw new Error('文件不是有效的 PNG 图片，请重新导出后上传')
  }
  const view = new DataView(bytes.buffer)
  const width = view.getUint32(16),
    height = view.getUint32(20)
  if (!width || !height || width > 2048 || height > 2048) {
    throw new Error('印章图片宽高需在 1–2048 像素之间')
  }
  const url = URL.createObjectURL(file)
  try {
    const image = new Image()
    image.src = url
    await image.decode()
    const canvas = document.createElement('canvas')
    canvas.width = image.naturalWidth
    canvas.height = image.naturalHeight
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    if (!ctx) throw new Error('当前浏览器无法读取图片，请更换浏览器后重试')
    ctx.drawImage(image, 0, 0)
    const pixels = ctx.getImageData(0, 0, canvas.width, canvas.height).data
    let transparent = false,
      visible = false
    for (let i = 3; i < pixels.length; i += 4) {
      if (pixels[i] < 255) transparent = true
      if (pixels[i] > 0) visible = true
    }
    if (!visible) throw new Error('图片完全透明，没有可显示的印章')
    if (!transparent)
      throw new Error('图片没有透明背景，请去掉底色后导出为 PNG 再上传')
    const data = canvas.toDataURL('image/png')
    if (((data.length - 22) * 3) / 4 > MAX_STAMP_BYTES) {
      throw new Error('处理后的图片超过 256 KB，请降低分辨率后重试')
    }
    return data
  } catch (error) {
    if (error instanceof Error && error.name !== 'EncodingError') throw error
    throw new Error('PNG 图片无法读取，请重新导出后上传')
  } finally {
    URL.revokeObjectURL(url)
  }
}
