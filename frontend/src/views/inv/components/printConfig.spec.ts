import { describe, it, expect } from 'vitest'
import {
  DEFAULT_STAMP,
  parseContent,
  normalizeStamp,
  stampLayout,
  stampImageSource,
} from './printConfig'

describe('print stamp persistence and layout', () => {
  it('keeps old and empty templates un-stamped and gives independent defaults', () => {
    const a = parseContent(),
      b = parseContent('{"footer":{"text":"旧页脚"}}')
    expect(a.stamp.enabled).toBe(false)
    expect(b.stamp.enabled).toBe(false)
    a.stamp.xMm = 12
    expect(parseContent().stamp.xMm).toBe(DEFAULT_STAMP.xMm)
    expect(b.footer.text).toBe('旧页脚')
    expect(parseContent('null').stamp.enabled).toBe(false)
  })
  it('round trips the image, exact millimeter geometry and off switch', () => {
    const cfg = parseContent()
    cfg.stamp = {
      enabled: true,
      image: 'data:image/png;base64,aGVsbG8=',
      xMm: 30.5,
      yMm: 8,
      widthMm: 44,
      heightMm: 36,
    }
    expect(parseContent(JSON.stringify(cfg)).stamp).toEqual(cfg.stamp)
    cfg.stamp.enabled = false
    expect(parseContent(JSON.stringify(cfg)).stamp.image).toBe(cfg.stamp.image)
  })
  it('rejects external URLs, svg and unsafe source values', () => {
    for (const value of [
      'javascript:alert(1)',
      'https://example.com/seal.png',
      'data:image/svg+xml;base64,AAA=',
      null,
    ]) {
      expect(stampImageSource(value)).toBe('')
    }
    expect(
      normalizeStamp({
        enabled: 'yes',
        xMm: Infinity,
        yMm: -5,
        widthMm: 500,
        heightMm: 0,
      }),
    ).toMatchObject({
      enabled: false,
      xMm: 100,
      yMm: 0,
      widthMm: 80,
      heightMm: 10,
    })
  })
  it('keeps the whole stamp inside the A5 and A4 printable widths', () => {
    const cfg = { ...DEFAULT_STAMP, xMm: 200, widthMm: 80 }
    expect(stampLayout(cfg, 'A5', 30).xMm).toBe(8)
    expect(stampLayout(cfg, 'A4', 12).xMm).toBe(106)
  })
})
