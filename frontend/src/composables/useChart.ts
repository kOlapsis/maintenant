import { ref, onMounted, onUnmounted, type Ref } from 'vue'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'

export interface UseChartOptions {
  el: Ref<HTMLElement | null>
  opts: () => Partial<uPlot.Options>
  data: () => uPlot.AlignedData
}

export function useChart({ el, opts, data }: UseChartOptions) {
  let chart: uPlot | null = null
  let observer: ResizeObserver | null = null
  const ready = ref(false)

  function create() {
    if (!el.value) return
    destroy()

    const rect = el.value.getBoundingClientRect()
    const userOpts = opts()
    const fullOpts: uPlot.Options = {
      width: rect.width || 400,
      height: userOpts.height || 200,
      ...userOpts,
    } as uPlot.Options

    chart = new uPlot(fullOpts, data(), el.value)
    ready.value = true
  }

  function setData(d: uPlot.AlignedData) {
    if (chart) {
      chart.setData(d)
    }
  }

  function destroy() {
    if (chart) {
      chart.destroy()
      chart = null
      ready.value = false
    }
  }

  onMounted(() => {
    create()

    if (el.value) {
      observer = new ResizeObserver((entries) => {
        for (const entry of entries) {
          if (chart) {
            chart.setSize({
              width: entry.contentRect.width,
              height: chart.height,
            })
          }
        }
      })
      observer.observe(el.value)
    }
  })

  onUnmounted(() => {
    if (observer) {
      observer.disconnect()
      observer = null
    }
    destroy()
  })

  return { chart, ready, setData, create, destroy }
}
