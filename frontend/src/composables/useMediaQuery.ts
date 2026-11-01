import { onBeforeUnmount, onMounted, ref } from 'vue'

/** Keep responsive DOM ownership in sync with the same breakpoint used by CSS. */
export function useMediaQuery(query: string) {
  const media = window.matchMedia(query)
  const matches = ref(media.matches)
  const update = () => {
    matches.value = media.matches
  }
  onMounted(() => {
    update()
    media.addEventListener('change', update)
  })
  onBeforeUnmount(() => media.removeEventListener('change', update))
  return matches
}
