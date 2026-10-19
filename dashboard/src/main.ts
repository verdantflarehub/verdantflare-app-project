import { createApp, h } from 'vue'
import { createRouter, RouterView } from 'vue-router'
import Browser from './pages/ProjectBrowser.vue'
import WorkspacePage from './pages/WorkspacePage.vue'
import UseAssetPage from './pages/UseAssetPage.vue'
import PublicationPage from './pages/PublicationPage.vue'
import CreateProjectPage from './pages/CreateProjectPage.vue'
import CreateFromPromptPage from './pages/CreateFromPromptPage.vue'
import { connectHost, disconnectHost } from './platform/host'
import { canonicalRoute } from './platform/route-links'
import { createProjectHistory } from './platform/project-history'
import './base.css'

const root = document.querySelector<HTMLElement>('#app')!

async function mount() {
  const { appSDK: sdk, dashboardHost: host } = connectHost('project')
  const router = createRouter({ history: createProjectHistory(), routes: [
    { path: '/', redirect: '/projects' },
    { path: '/projects', component: Browser },
    { path: '/create', component: CreateProjectPage },
    { path: '/create/from-prompt', component: CreateFromPromptPage },
    { path: '/workspace', component: WorkspacePage },
    { path: '/use-asset', component: UseAssetPage },
    { path: '/publication', component: PublicationPage },
    { path: '/:pathMatch(.*)*', redirect: '/projects' },
  ] })
  const app = createApp({ render: () => h(RouterView) }).use(router)
  document.body.dataset.theme = sdk.theme.get()
  await router.replace(canonicalRoute('project', sdk.navigation.get().path))
  await router.isReady()
  app.mount(root)
  let disposed = false, revision = sdk.navigation.get().revision
  const hostPaths = new Set<string>()
  const stopTheme = sdk.theme.subscribe(theme => { if (!disposed) document.body.dataset.theme = theme })
  const stopRoute = sdk.navigation.subscribe(route => {
    if (disposed || route.revision <= revision) return
    revision = route.revision
    const path = canonicalRoute('project', route.path)
    if (path === router.currentRoute.value.fullPath) return
    hostPaths.add(path)
    void router.replace(path).finally(() => hostPaths.delete(path))
  })
  const stopAfter = router.afterEach((to, _from, failure) => {
    if (failure || disposed || hostPaths.has(to.fullPath) || to.fullPath === sdk.navigation.get().path) return
    void sdk.navigation.request({ target: 'app', path: to.fullPath }).catch(() => {
      if (disposed) return
      const path = sdk.navigation.get().path
      hostPaths.add(path)
      void router.replace(path).finally(() => hostPaths.delete(path))
    })
  })
  const resize = new ResizeObserver(() => { if (!disposed) host.resize(Math.ceil(root.getBoundingClientRect().height)) })
  resize.observe(root)
  const stopLifecycle = sdk.lifecycle.subscribe(event => {
    if (event !== 'dispose' || disposed) return
    disposed = true; resize.disconnect(); stopTheme(); stopRoute(); stopAfter(); stopLifecycle(); app.unmount(); disconnectHost()
  })
  host.ready()
  host.setDirty(false)
  host.resize(Math.ceil(root.getBoundingClientRect().height))
}
void mount().catch(error => { disconnectHost(); root.textContent = error instanceof Error ? error.message : '应用暂时无法打开。' })
