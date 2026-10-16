import { useHost } from './host'
let owner: object | null = null, revision = 0
export function closePreview(key: object) {
  if (owner !== key) return
  owner = null; revision++; useHost().appSDK.media?.close()
}
export async function previewImage(key: object, source: string, title: string): Promise<boolean> {
  const media = useHost().appSDK.media
  if (!media) return false
  if (!source.startsWith('blob:')) throw new Error('图片尚未准备好。')
  owner = key; const epoch = ++revision
  const blob = await (await fetch(source)).blob()
  const bytes = await blob.arrayBuffer()
  if (epoch !== revision || owner !== key) return true
  await media.preview({ bytes, mime: blob.type, title: title.slice(0, 256) })
  return true
}
