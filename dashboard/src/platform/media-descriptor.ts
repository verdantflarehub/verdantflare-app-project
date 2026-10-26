import { useHost } from './host.ts'

const prefix = 'vf.fixed-media.v1:'
const pending = new Map<string, Promise<unknown>>()
/** Descriptors contain immutable metadata and a stable same-origin path, never credentials.
 * Keep the control lookup out of subsequent visits as well as caching the media bytes. */
export async function fixedMedia<T>(identity: unknown, load: () => Promise<T>): Promise<T> {
  let key = ''
  try {
    const c = useHost().appSDK.context.get()
    key = prefix + JSON.stringify([c.userId, c.organizationId, c.stationId, c.contextRevision, identity])
    const saved = localStorage.getItem(key)
    if (saved) return JSON.parse(saved) as T
  } catch { /* Storage is optional; HTTP caching still works when it is unavailable. */ }
  if (key && pending.has(key)) return pending.get(key) as Promise<T>
  const task = load().then(value => {
    if (key) try {
      const keys = Object.keys(localStorage).filter(k => k.startsWith(prefix))
      for (const old of keys.slice(0, Math.max(0, keys.length - 511))) localStorage.removeItem(old)
      localStorage.setItem(key, JSON.stringify(value))
    } catch { /* A storage quota cannot prevent reading media. */ }
    return value
  }).finally(() => { if (key) pending.delete(key) })
  if (key) pending.set(key, task)
  return task
}

export function forgetFixedMedia() {
  try { Object.keys(localStorage).filter(k => k.startsWith(prefix)).forEach(k => localStorage.removeItem(k)) } catch { /* optional */ }
}
