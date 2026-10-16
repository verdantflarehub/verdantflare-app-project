import test from 'node:test'
import assert from 'node:assert/strict'
import { createRouter } from 'vue-router'
import { createProjectHistory } from '../src/platform/project-history.ts'

test('app memory history returns to the filtered page after detail, version and revision navigation', async () => {
  const history = createProjectHistory()
  const router = createRouter({ history, routes: [{ path: '/projects', component: {} }] })
  const list = '/projects?project=one&tab=materials&kind=prop&find=ring&status=ready'
  await router.push(list)
  assert.equal(history.state.back, undefined)
  await router.push(list + '&material=ring')
  assert.equal(history.state.back, list)
  await router.replace(list + '&material=ring&revision=fixed')
  assert.equal(history.state.back, list)
  await router.push(list + '&material=ring&revision=fixed&version=v2')
  assert.equal(history.state.back, list + '&material=ring&revision=fixed')
  const back = () => new Promise<void>(resolve => { const stop = router.afterEach(() => { stop(); resolve() }); router.back() })
  await back()
  assert.equal(router.currentRoute.value.query.revision, 'fixed')
  assert.equal(history.state.back, list)
  await back()
  assert.equal(router.currentRoute.value.fullPath, list)
})
