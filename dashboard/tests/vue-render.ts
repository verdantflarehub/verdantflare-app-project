import { createRequire } from 'node:module'
import { readFileSync, existsSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createHash } from 'node:crypto'

// Compile the real SFCs in memory for service-free rendering tests, using existing dependencies.
const frontend = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const require = createRequire(resolve(frontend, 'package.json'))
const { parse, compileScript, compileStyle } = require('vue/compiler-sfc')
const ts = require('typescript')
const { createSSRApp, h } = require('vue')
const { renderToString } = require('vue/server-renderer')
const { createMemoryHistory, createRouter } = require('vue-router')
const context = { userId: 'test-user', organizationId: 'test-org', stationId: 'test-station', appId: 'project', installationId: 'test-installation', contextRevision: 1 }
const host = { appSDK: { context: { get: () => context, subscribe: () => () => {} }, lifecycle: { subscribe: () => () => {} } }, dashboardHost: { setDirty() {} } }

export function renderer() {
  const cache = new Map<string, any>(), styles = new Map<string, string>()
  function load(filename: string): any {
    if (cache.has(filename)) return cache.get(filename)
    if (filename.endsWith('/platform/host.ts') || filename.endsWith('\\platform\\host.ts')) return { useHost: () => host }
    let source = readFileSync(filename, 'utf8'), scope = ''
    if (filename.endsWith('.vue')) {
      const { descriptor, errors } = parse(source, { filename })
      if (errors.length) throw errors[0]
      scope = 'data-v-' + createHash('sha256').update(filename).digest('hex').slice(0, 8)
      source = compileScript(descriptor, { id: scope, inlineTemplate: true, templateOptions: { ssr: true } }).content
      styles.set(filename, descriptor.styles.map((style: any) => {
        const result = compileStyle({ source: style.content, filename, id: scope, scoped: style.scoped })
        if (result.errors.length) throw result.errors[0]
        return result.code
      }).join('\n'))
    }
    const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true } }).outputText
    const module = { exports: {} as any }
    cache.set(filename, module.exports)
    const localRequire = (name: string) => {
      if (!name.startsWith('.')) return require(name)
      const target = resolve(dirname(filename), name)
      const path = [target, target + '.ts', target + '.vue'].find(file => existsSync(file))
      if (!path) throw Error('Missing test dependency: ' + name)
      return load(path)
    }
    new Function('require', 'module', 'exports', code)(localRequire, module, module.exports)
    if (scope) module.exports.default.__scopeId = scope
    cache.set(filename, module.exports)
    return module.exports
  }
  return {
    async render(component: string, props: Record<string, unknown>, path = '/projects') {
      const view = load(resolve(frontend, 'src', component)).default
      const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { render: () => null } }] })
      await router.push(path); await router.isReady()
      return await renderToString(createSSRApp({ render: () => h(view, props) }).use(router))
    },
    css() { return readFileSync(resolve(frontend, 'src/tokens.css'), 'utf8') + '\n' + readFileSync(resolve(frontend, 'src/base.css'), 'utf8').replace(/@import[^;]+;/g, '') + '\n' + [...styles.values()].join('\n') },
  }
}
