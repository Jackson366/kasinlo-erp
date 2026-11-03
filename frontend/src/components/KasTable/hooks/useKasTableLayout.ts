import { nextTick, onActivated, onBeforeUnmount, onMounted, ref, watch } from 'vue'

/** 按既有工作区计算表体空间；ListPanel 与 KasTable 共用同一尺寸 owner。 */
export function useKasTableLayout(fit: () => boolean) {
  const panel = ref<HTMLElement>()
  const height = ref('auto')
  const minimum = ref('auto')
  let observer: ResizeObserver | undefined
  let frame = 0

  function measure(): void {
    const el = panel.value
    const main = el?.closest<HTMLElement>('.layout__content')
    if (!fit() || !el || !main || !el.getClientRects().length) return
    const style = getComputedStyle(el)
    const padding = parseFloat(style.paddingTop) + parseFloat(style.paddingBottom)
    let chrome = padding + 2
    for (const child of el.children) {
      if (child.classList.contains('el-table') || !(child instanceof HTMLElement))
        continue
      const css = getComputedStyle(child)
      if (css.display === 'none' || css.position === 'absolute') continue
      chrome +=
        child.getBoundingClientRect().height +
        parseFloat(css.marginTop) +
        parseFloat(css.marginBottom)
    }
    // Measure against the canonical content scroller, never the window or a shared shell.
    const top =
      el.getBoundingClientRect().top -
      main.getBoundingClientRect().top +
      main.scrollTop
    const outerCard = el.parentElement?.closest<HTMLElement>('.page-card')
    const inset = outerCard
      ? parseFloat(getComputedStyle(outerCard).paddingBottom) + 1
      : 0
    height.value = `${Math.floor(main.clientHeight - top - inset)}px`
    minimum.value = `${Math.ceil(chrome + 200)}px`
  }
  function schedule(): void {
    cancelAnimationFrame(frame)
    frame = requestAnimationFrame(measure)
  }
  onMounted(() => {
    observer = new ResizeObserver(schedule)
    const el = panel.value
    if (el) {
      observer.observe(el)
      for (const child of el.children) observer.observe(child)
      const main = el.closest('.layout__content')
      if (main) observer.observe(main)
      if (el.parentElement) observer.observe(el.parentElement)
    }
    schedule()
  })
  onActivated(() => void nextTick(schedule))
  watch(
    () => fit(),
    () => void nextTick(schedule),
  )
  onBeforeUnmount(() => {
    observer?.disconnect()
    cancelAnimationFrame(frame)
  })
  return { panel, height, minimum }
}
