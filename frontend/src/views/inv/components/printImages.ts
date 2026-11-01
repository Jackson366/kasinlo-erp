/** Image decoding must finish before the browser snapshots the sheet for printing. */
export async function waitForPrintImages(root: HTMLElement): Promise<void> {
  let timeout: ReturnType<typeof setTimeout> | undefined
  try {
    await Promise.race([
      Promise.all(
        Array.from(root.querySelectorAll('img')).map(async (image) => {
          if (!image.complete || !image.naturalWidth) await image.decode()
          if (!image.naturalWidth)
            throw new Error('打印图片未能加载，请重新打开预览后重试')
        }),
      ),
      new Promise<never>((_, reject) => {
        timeout = setTimeout(
          () => reject(new Error('打印图片加载超时，请重试')),
          10000,
        )
      }),
    ])
  } finally {
    if (timeout !== undefined) clearTimeout(timeout)
  }
}
