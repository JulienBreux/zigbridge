import { ref, computed } from 'vue'
import type { TabId } from '@/types'

export interface RouteState {
  tab: TabId
  deviceIeee: string | null
}

// Canonical route paths for navigation tabs
export const TAB_ROUTES: Record<TabId, string> = {
  devices: '/devices',
  bindings: '/bindings',
  ai: '/suggestions',
  events: '/activity',
  diagnostics: '/system',
  simulation: '/simulation',
}

// Generates canonical path for given tab and optional device IEEE
export function getRoutePath(tab: TabId, deviceIeee?: string | null): string {
  if (tab === 'devices' && deviceIeee) {
    return `/devices/${encodeURIComponent(deviceIeee)}`
  }
  return TAB_ROUTES[tab] || '/devices'
}

// Parses pathname and optional hash into a RouteState
export function parseRoute(pathname: string, hash: string = ''): RouteState {
  // Check if hash-based routing is used (#/... or #...)
  let routeStr = pathname
  if (hash && (hash.startsWith('#/') || hash.startsWith('#'))) {
    routeStr = hash.replace(/^#\/?/, '/')
  }

  // Strip query string if present
  const queryIdx = routeStr.indexOf('?')
  if (queryIdx !== -1) {
    routeStr = routeStr.slice(0, queryIdx)
  }

  // Normalize duplicate and trailing slashes
  routeStr = routeStr.replace(/\/+/g, '/').replace(/\/$/, '') || '/'

  // 1. Device detail match: /devices/:ieee
  const deviceDetailMatch = routeStr.match(/^\/devices\/(.+)$/i)
  if (deviceDetailMatch && deviceDetailMatch[1]) {
    try {
      const decodedIeee = decodeURIComponent(deviceDetailMatch[1].trim())
      if (decodedIeee) {
        return { tab: 'devices', deviceIeee: decodedIeee }
      }
    } catch {
      return { tab: 'devices', deviceIeee: deviceDetailMatch[1].trim() }
    }
  }

  // 2. Exact tab paths and aliases (case-insensitive)
  const normalized = routeStr.toLowerCase()
  switch (normalized) {
    case '/':
    case '/devices':
      return { tab: 'devices', deviceIeee: null }
    case '/bindings':
      return { tab: 'bindings', deviceIeee: null }
    case '/suggestions':
    case '/ai':
      return { tab: 'ai', deviceIeee: null }
    case '/activity':
    case '/events':
      return { tab: 'events', deviceIeee: null }
    case '/system':
    case '/diagnostics':
      return { tab: 'diagnostics', deviceIeee: null }
    case '/simulation':
      return { tab: 'simulation', deviceIeee: null }
    default:
      return { tab: 'devices', deviceIeee: null }
  }
}

// Shared router reactive state
const currentRoute = ref<RouteState>({ tab: 'devices', deviceIeee: null })

export function useRouter() {
  const activeTab = computed(() => currentRoute.value.tab)
  const currentDeviceIeee = computed(() => currentRoute.value.deviceIeee)

  function navigateTo(tab: TabId, deviceIeee: string | null = null, replace = false) {
    const targetPath = getRoutePath(tab, deviceIeee)
    const currentPath = window.location.pathname

    if (targetPath !== currentPath || (window.location.hash && window.location.hash !== '')) {
      if (replace) {
        window.history.replaceState(null, '', targetPath)
      } else {
        window.history.pushState(null, '', targetPath)
      }
    }

    currentRoute.value = { tab, deviceIeee }
  }

  function navigateToTab(tab: TabId, replace = false) {
    navigateTo(tab, null, replace)
  }

  function navigateToDevice(ieee: string, replace = false) {
    navigateTo('devices', ieee, replace)
  }

  function syncFromLocation(): RouteState {
    const parsed = parseRoute(window.location.pathname, window.location.hash)

    // Automatically normalize legacy hash routes to clean HTML5 path if present
    if (window.location.hash && window.location.hash.startsWith('#/')) {
      const canonical = getRoutePath(parsed.tab, parsed.deviceIeee)
      window.history.replaceState(null, '', canonical)
    }

    currentRoute.value = parsed
    return parsed
  }

  function initRouter(onRouteChanged?: (route: RouteState) => void): () => void {
    const handleEvent = () => {
      const route = syncFromLocation()
      if (onRouteChanged) {
        onRouteChanged(route)
      }
    }

    // Set initial route from window location
    handleEvent()

    window.addEventListener('popstate', handleEvent)
    window.addEventListener('hashchange', handleEvent)

    return () => {
      window.removeEventListener('popstate', handleEvent)
      window.removeEventListener('hashchange', handleEvent)
    }
  }

  return {
    currentRoute,
    activeTab,
    currentDeviceIeee,
    getRoutePath,
    navigateTo,
    navigateToTab,
    navigateToDevice,
    syncFromLocation,
    initRouter,
  }
}
