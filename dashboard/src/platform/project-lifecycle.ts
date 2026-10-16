import type { AppSDK, Context, Json } from './types.ts'
import { commitID } from './asset-import.ts'
import type { ProjectItem } from './project-content.ts'

export interface LifecycleRecord {
  action: 'delete' | 'restore'; name: string
  request: { project_id: string; change_id: string; expected_revision_id: string; expected_lifecycle_version: number }
}
export interface LifecycleResult { project_id: string; change_id: string; state: 'completed'; deleted: boolean; lifecycle_version: number; head_revision_id: string }
const validID = (v: unknown) => typeof v === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(v)
export const lifecycleKey = (c: Readonly<Context>) => 'project.lifecycle.v1:' + JSON.stringify([c.userId, c.organizationId, c.stationId, c.installationId])
export function readLifecycle(storage: Pick<Storage, 'getItem'>, key: string): LifecycleRecord | null {
  const raw = storage.getItem(key); if (!raw) return null
  const r = JSON.parse(raw) as LifecycleRecord, q = r?.request
  if (!r || !['delete', 'restore'].includes(r.action) || typeof r.name !== 'string' || !q || !validID(q.project_id) || !validID(q.change_id) || !validID(q.expected_revision_id) || !Number.isSafeInteger(q.expected_lifecycle_version) || q.expected_lifecycle_version < 0 || Object.keys(q).sort().join(',') !== 'change_id,expected_lifecycle_version,expected_revision_id,project_id') throw new Error('原删除或恢复记录损坏，请保留记录后联系管理员。')
  return r
}
export function newLifecycle(item: ProjectItem, action: 'delete' | 'restore'): LifecycleRecord {
  if (!item.can_delete || !Number.isSafeInteger(item.lifecycle_version) || item.lifecycle_version! < 0) throw new Error('请刷新项目权限后重试。')
  return { action, name: item.name, request: { project_id: item.project_id, change_id: commitID(), expected_revision_id: item.head_revision_id, expected_lifecycle_version: item.lifecycle_version! } }
}
export class LifecycleError extends Error { constructor(message: string, readonly definitive: boolean) { super(message) } }
export async function runLifecycle(sdk: AppSDK, record: LifecycleRecord, recover: boolean): Promise<LifecycleResult> {
  const context = sdk.context.get(), identity = JSON.stringify(context)
  if (context.appId !== 'project') throw new LifecycleError('应用身份不匹配。', false)
  const request = async (action: string, body: Json) => {
    const response = await sdk.app.request({ method: 'POST', path: '/api/v1/project/' + action, body })
    if (JSON.stringify(sdk.context.get()) !== identity) throw new LifecycleError('会话已切换，原请求记录已保留。', false)
    return response
  }
  let response = recover ? await request('lifecycle_status', { project_id: record.request.project_id, change_id: record.request.change_id }) : undefined
  if (!response || response.status === 404) response = await request(record.action, record.request)
  if (response.status < 200 || response.status >= 300) {
    const code = (response.data as { code?: string })?.code
    const message = code === 'PROJECT_BUSY' ? '项目正在保存或被其他操作使用，请稍后重试。' : code === 'REVISION_CONFLICT' || code === 'LIFECYCLE_CONFLICT' ? '项目已发生变化，请刷新后重新操作。' : response.status === 403 ? '当前账号没有删除或恢复此项目的权限。' : '结果尚未确认，请查询原请求。'
    throw new LifecycleError(message, [400, 403, 409].includes(response.status))
  }
  const result = response.data as unknown as LifecycleResult
  if (result.project_id !== record.request.project_id || result.change_id !== record.request.change_id || result.state !== 'completed' || result.deleted !== (record.action === 'delete') || !validID(result.head_revision_id) || !Number.isSafeInteger(result.lifecycle_version) || result.lifecycle_version !== record.request.expected_lifecycle_version + 1) throw new LifecycleError('返回结果不完整，原请求记录已保留。', false)
  return result
}
