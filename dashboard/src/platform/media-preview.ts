import { useHost } from './host'
export interface PreviewGallery { id: string; index: number; count: number }
let owner: object | null = null, revision = 0
export function closePreview(key: object) {
  if (owner !== key) return
  owner = null; revision++; useHost().appSDK.media?.close()
}
export async function previewImage(key: object, source: string, title: string, gallery?: PreviewGallery): Promise<boolean> {
  const media = useHost().appSDK.media
  if (!media) return false
  if (!source.startsWith('blob:')) throw new Error('图片尚未准备好。')
  owner = key; const epoch = ++revision
  const blob = await (await fetch(source)).blob()
  const bytes = await blob.arrayBuffer()
  if (epoch !== revision || owner !== key) return true
  const payload: { bytes: ArrayBuffer; mime: string; title: string; gallery?: PreviewGallery } = {
    bytes,
    mime: blob.type,
    title: title.slice(0, 256),
  }
  if (gallery && gallery.count > 1) payload.gallery = gallery
  await (media.preview as (req: typeof payload) => Promise<void>)(payload)
  return true
}
