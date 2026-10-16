export interface CoverPosition { x: number; y: number }
export const defaultCoverPosition = (): CoverPosition => ({ x: 50, y: 0 })
/** Keep the edit dialog and card on the same legacy portrait fallback. */
export function coverFile(content: { cover_placeholder?: boolean; cover_file_id?: string | null; character?: { portrait_file_id: string | null } } | null | undefined): string | null {
  return content?.cover_placeholder ? null : content?.cover_file_id || content?.character?.portrait_file_id || null
}
export function coverMetadata(file: string | null, position: CoverPosition) {
  return { cover_file_id: file || null, cover_position: file ? coverPosition(position) : null, cover_placeholder: !file }
}
export function coverPosition(value: unknown): CoverPosition {
  if (value == null) return defaultCoverPosition()
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw Error('封面裁切位置不正确。')
  const v = value as Record<string, unknown>
  if (Object.keys(v).sort().join(',') !== 'x,y' || [v.x, v.y].some(n => typeof n !== 'number' || !Number.isFinite(n) || n < 0 || n > 100)) throw Error('封面裁切位置不正确。')
  return { x: v.x as number, y: v.y as number }
}
export function dragCover(position: CoverPosition, dx: number, dy: number, width: number, height: number, naturalWidth: number, naturalHeight: number): CoverPosition {
  if (![dx, dy, width, height, naturalWidth, naturalHeight].every(Number.isFinite) || [width, height, naturalWidth, naturalHeight].some(n => n <= 0)) return { ...position }
  const scale = Math.max(width / naturalWidth, height / naturalHeight)
  const move = (start: number, delta: number, space: number) => space >= -.01 ? start : Math.round(Math.max(0, Math.min(100, start + delta / space * 100)) * 100) / 100
  return { x: move(position.x, dx, width - naturalWidth * scale), y: move(position.y, dy, height - naturalHeight * scale) }
}
