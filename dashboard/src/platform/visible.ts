import { onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { useHost } from './host'

/** Observe the rendered element in the host viewport, including clipped ancestors.
 * No preloading margin: media requests start only when a card actually appears. */
export function useVisible(element: Ref<HTMLElement | null | undefined>, margin = '0px') {
  const visible = ref(false)
  let intersecting = false, active = true, observer: IntersectionObserver | undefined
  let stopLifecycle = () => {}, stopWatch = () => {}, stopVisibility = () => {}
  const update = () => { visible.value = active && intersecting && (typeof document === 'undefined' || document.visibilityState !== 'hidden') }
  onMounted(() => {
    observer = new IntersectionObserver(entries => {
      const current = entries.filter(e => e.target === element.value)
      if (!current.length) return
      intersecting = current.some(e => e.isIntersecting && e.intersectionRect.width > 0 && e.intersectionRect.height > 0)
      update()
    }, { rootMargin: margin })
    stopWatch = watch(element, (next, old) => {
      if (old) observer?.unobserve(old)
      intersecting = false; update()
      if (next) observer?.observe(next)
    }, { immediate: true, flush: 'post' })
    if (typeof document !== 'undefined') {
      const changed = () => {
        intersecting = false; update()
        if (element.value) {
          observer?.unobserve(element.value)
          if (active && document.visibilityState !== 'hidden') observer?.observe(element.value)
        }
      }
      document.addEventListener('visibilitychange', changed)
      stopVisibility = () => document.removeEventListener('visibilitychange', changed)
    }
    stopLifecycle = useHost().appSDK.lifecycle.subscribe(event => {
      active = event === 'active'; update()
      if (!active && element.value) observer?.unobserve(element.value)
      else if (element.value) { intersecting = false; update(); observer?.observe(element.value) }
    })
  })
  onBeforeUnmount(() => { stopWatch(); stopLifecycle(); stopVisibility(); observer?.disconnect() })
  return visible
}
