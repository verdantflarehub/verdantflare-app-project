import { createMemoryHistory } from 'vue-router'

/** Keep app-local return navigation available inside the host's memory router. */
export function createProjectHistory() {
  const history = createMemoryHistory()
  const push = history.push.bind(history), replace = history.replace.bind(history)
  history.push = (to, data) => push(to, { ...data, back: history.location || null })
  // Revision pinning and filter changes replace the current page, retaining its parent.
  history.replace = (to, data) => replace(to, { ...history.state, ...data })
  return history
}
