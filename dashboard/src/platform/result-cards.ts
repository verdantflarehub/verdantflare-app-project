export function resultCardRatio(width: number, height: number): number {
  if (!(width > 0 && height > 0) || !Number.isFinite(width / height)) return 9 / 16
  const ratio = width / height
  return Math.max(9 / 16, Math.min(16 / 9, ratio))
}

export function resultColumnCount(width: number): number {
  return width < 720 ? 2 : Math.max(2, Math.floor((width + 12) / 252))
}

// Called only when the collection or column count changes. Image loads and
// prompt expansion must not move a card to another column.
export function placeResultCards(ratios: number[], count: number, width = 240): number[][] {
  const columns: number[][] = Array.from({ length: count }, () => [])
  const heights = Array(count).fill(0) as number[]
  ratios.forEach((ratio, index) => {
    const column = heights.indexOf(Math.min(...heights))
    columns[column].push(index)
    heights[column] += width / resultCardRatio(ratio, 1) + 44
  })
  return columns
}
