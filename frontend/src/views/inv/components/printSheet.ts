import { waitForPrintImages } from './printImages'

/** Copy the same rendered sheet outside dialog/scroll containers so long tables can paginate. */
export async function printSheet(preview: HTMLElement): Promise<void> {
  const sheet = preview.querySelector('.sheet')
  if (!sheet) throw new Error('打印内容尚未准备好')
  await waitForPrintImages(preview)
  const portal = document.createElement('div')
  portal.className = 'print-portal'
  portal.append(sheet.cloneNode(true))
  document.body.append(portal)
  let timeout: ReturnType<typeof setTimeout> | undefined
  const done = () => {
    document.body.classList.remove('printing-sheet')
    portal.remove()
    window.removeEventListener('afterprint', done)
    if (timeout !== undefined) clearTimeout(timeout)
  }
  try {
    await waitForPrintImages(portal)
    await document.fonts.ready
    document.body.classList.add('printing-sheet')
    window.addEventListener('afterprint', done, { once: true })
    window.print()
    if (portal.isConnected) timeout = setTimeout(done, 2000)
  } catch (error) {
    done()
    throw error
  }
}
