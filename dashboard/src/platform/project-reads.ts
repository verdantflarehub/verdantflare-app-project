/** Bound read pressure per fixed project reader. This queue is never used for writes. */
export class ProjectReads {
  private running = 0
  private stopped = false
  private waiting: Array<{ resolve: () => void; reject: (error: Error) => void; interactive: boolean }> = []
  constructor(private retryable: (error: unknown) => boolean, private limit = 4) {}
  private active() { if (this.stopped) throw new Error('项目已切换。') }
  async run<T>(read: () => Promise<T>, retryLimit = 2, eligible: () => boolean = () => true, interactive = true): Promise<T> {
    this.active()
    if (this.running >= this.limit) await new Promise<void>((resolve, reject) => {
      const entry = { resolve, reject, interactive }
      const at = interactive ? this.waiting.findIndex(item => !item.interactive) : -1
      if (at < 0) this.waiting.push(entry)
      else this.waiting.splice(at, 0, entry)
    })
    else this.running++
    try {
      for (let attempt = 0; ; attempt++) {
        this.active()
        if (!eligible()) throw new DOMException('素材已离开可视范围。', 'AbortError')
        try { return await read() }
        catch (error) {
          if (attempt >= retryLimit || !this.retryable(error)) throw error
          await new Promise(resolve => setTimeout(resolve, 250 * (attempt + 1)))
        }
      }
    } finally {
      const next = this.waiting.shift()
      if (next) next.resolve()
      else this.running--
    }
  }
  dispose() {
    this.stopped = true
    for (const pending of this.waiting.splice(0)) pending.reject(new Error('项目已切换。'))
  }
}
