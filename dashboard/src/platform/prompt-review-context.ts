/** Read-only source metadata, never a Project approval or execution claim. */
export function promptReviewContext(doc: Record<string, unknown>, resultKey: string): Record<string, unknown> | undefined {
  if (doc.schema_version === 1) return
  const fail = (): never => { throw Error('来源用途审核信息不完整，未创建项目。') }
  const object = (v: unknown): Record<string, unknown> => v && typeof v === 'object' && !Array.isArray(v) ? v as Record<string, unknown> : fail()
  const list = (v: unknown): unknown[] => Array.isArray(v) ? v : fail()
  const keys = (v: unknown): string[] => {
    const values = list(v)
    if (values.some(k => typeof k !== 'string' || !k.trim()) || new Set(values).size !== values.length) fail()
    return values as string[]
  }
  if (doc.schema_version !== 2) fail()
  const curation = object(doc.curation), assessments = list(curation.assessments).map(object)
  const selected = assessments.filter(a => a.result_key === resultKey)
  if (selected.length !== 1) fail()
  const assessment = selected[0], packageKeys = keys(assessment.package_keys)
  if (!['unassessed', 'recommended', 'needs_review', 'excluded'].includes(String(assessment.selection)) || !keys(assessment.reason_codes).length) fail()
  if (typeof assessment.note !== 'string' || !assessment.note.trim()) fail()
  const evidence = list(assessment.evidence_refs)
  if (['recommended', 'excluded'].includes(String(assessment.selection)) && !evidence.length) fail()
  evidence.forEach(object); list(assessment.merged_from_refs).forEach(object)
  const previewKeys = new Set<string>()
  for (const raw of list(assessment.previews)) {
    const preview = object(raw)
    if (typeof preview.key !== 'string' || !preview.key.trim() || previewKeys.has(preview.key)
      || !['image', 'audio', 'video'].includes(String(preview.media_kind))
      || typeof preview.note !== 'string' || !preview.note.trim()) fail()
    previewKeys.add(preview.key as string)
    if (!['original_reference', 'generated_effect', 'reconstruction_preview'].includes(String(preview.role))) fail()
    if (preview.state === 'missing') {
      if (typeof preview.reason_code !== 'string' || !preview.reason_code.trim() || ['file_ref', 'locator', 'sha256', 'run_ref', 'input_refs'].some(k => k in preview)) fail()
    } else if (preview.state === 'ready') {
      object(preview.file_ref); object(preview.locator)
      if (typeof preview.sha256 !== 'string' || !/^[a-f0-9]{64}$/.test(preview.sha256) || 'reason_code' in preview) fail()
      if (preview.role === 'original_reference') { if ('run_ref' in preview || 'input_refs' in preview) fail() }
      else {
        const run = object(preview.run_ref)
        if (typeof run.service_id !== 'string' || !run.service_id.trim() || typeof run.run_id !== 'string' || !run.run_id.trim()) fail()
        list(preview.input_refs).forEach(object)
      }
    } else fail()
  }
  const allPackages = list(curation.packages).map(object)
  const packages = allPackages.filter(p => packageKeys.includes(String(p.key)))
  if (packages.length !== packageKeys.length || new Set(packages.map(p => p.key)).size !== packages.length) fail()
  for (const p of allPackages) {
    if (keys(p.result_keys).includes(resultKey) !== packageKeys.includes(String(p.key))) fail()
  }
  for (const p of packages) if (typeof p.purpose !== 'string' || !p.purpose.trim()) fail()
  if (!['manual', 'imported', 'generated'].includes(String(doc.origin))) fail()
  if (doc.origin === 'generated') {
    const snapshot = object(doc.processing_snapshot)
    object(doc.provenance)
    if (typeof snapshot.task_id !== 'string' || !/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(snapshot.task_id)
      || ![0, 2].includes(snapshot.pipeline_version as number)
      || typeof snapshot.assembly_version !== 'string' || !snapshot.assembly_version.trim()
      || typeof snapshot.adapter_revision !== 'string' || !snapshot.adapter_revision.trim()
      || typeof snapshot.plan_sha256 !== 'string' || !/^[a-f0-9]{64}$/.test(snapshot.plan_sha256)) fail()
  } else if ('processing_snapshot' in doc) fail()
  return JSON.parse(JSON.stringify({ schema_version: 2, curation: { packages, assessments: [assessment] },
    processing_snapshot: doc.processing_snapshot, provenance: doc.provenance })) as Record<string, unknown>
}
