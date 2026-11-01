import { it, expect, vi } from 'vitest'
import { waitForPrintImages } from './printImages'
const root = (images: unknown[]) =>
  ({ querySelectorAll: () => images }) as unknown as HTMLElement
it('waits for unfinished images before printing and rejects failed images', async () => {
  let finish!: () => void
  const image = {
    complete: false,
    naturalWidth: 0,
    decode: vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = () => {
            image.naturalWidth = 12
            resolve()
          }
        }),
    ),
  }
  let ready = false
  const pending = waitForPrintImages(root([image])).then(() => {
    ready = true
  })
  await Promise.resolve()
  expect(ready).toBe(false)
  finish()
  await pending
  expect(ready).toBe(true)
  await expect(
    waitForPrintImages(
      root([
        {
          complete: true,
          naturalWidth: 0,
          decode: () => Promise.reject(new Error('load failed')),
        },
      ]),
    ),
  ).rejects.toThrow('load failed')
})
