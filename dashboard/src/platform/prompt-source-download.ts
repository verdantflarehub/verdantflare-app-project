const limit = 4 * 1024 * 1024
const fail = (): never => { throw Error('来源正文下载不完整或与固定版本不一致。') }
export async function downloadPromptSource(path: unknown, size: number, sha256: string, signal: AbortSignal, fetcher: typeof fetch = fetch): Promise<string> {
  const uuid = '[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}'
  if (typeof path !== 'string' || !new RegExp(`^/studio/dashboards/${uuid}/content/${uuid}$`).test(path)
    || !Number.isSafeInteger(size) || size < 0 || size > limit || !/^[a-f0-9]{64}$/.test(sha256)) fail()
  signal.throwIfAborted()
  const response = await fetcher(path as string, { signal, credentials: 'same-origin', redirect: 'error', cache: 'no-store' })
  if (!response.ok || !response.body) fail()
  const reader = response.body!.getReader(), parts: Uint8Array[] = []
  const cancel = () => { void reader.cancel().catch(() => {}) }
  signal.addEventListener('abort', cancel, { once: true })
  let received = 0
  try {
    for (;;) {
      signal.throwIfAborted()
      const next = await reader.read()
      if (next.done) break
      received += next.value.byteLength
      if (received > size || received > limit) fail()
      parts.push(new Uint8Array(next.value))
    }
  } catch (error) { await reader.cancel().catch(() => {}); throw error }
  finally { signal.removeEventListener('abort', cancel); reader.releaseLock() }
  signal.throwIfAborted()
  if (received !== size) fail()
  const bytes = new Uint8Array(received)
  let offset = 0
  for (const part of parts) { bytes.set(part, offset); offset += part.byteLength }
  const digest = [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))].map(v => v.toString(16).padStart(2, '0')).join('')
  signal.throwIfAborted()
  if (digest !== sha256) fail()
  return new TextDecoder('utf-8', { fatal: true }).decode(bytes)
}
