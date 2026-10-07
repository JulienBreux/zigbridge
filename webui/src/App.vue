<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import type {
  BridgeStatus,
  Device,
  DeviceDefinition,
  Binding,
  Recommendation,
  VirtualDeviceSummary,
  ActivityItem,
  RawLogItem,
  TabId,
} from '@/types'
import { useZigBridgeApi } from '@/composables/useZigBridgeApi'
import { useZigBridgeWs } from '@/composables/useZigBridgeWs'
import { useRouter } from '@/composables/useRouter'
import { sortDevicesByFriendlyName } from '@/utils/formatters'

// Common UI
import AppHeader from '@/components/common/AppHeader.vue'
import AppFooter from '@/components/common/AppFooter.vue'
import OfflineBanner from '@/components/common/OfflineBanner.vue'

// Tab Views
import DevicesTab from '@/components/devices/DevicesTab.vue'
import DeviceDetail from '@/components/devices/DeviceDetail.vue'
import BindingsTab from '@/components/bindings/BindingsTab.vue'
import AiTab from '@/components/ai/AiTab.vue'
import ActivityTab from '@/components/activity/ActivityTab.vue'
import DiagnosticsTab from '@/components/system/DiagnosticsTab.vue'
import SimulationTab from '@/components/simulation/SimulationTab.vue'

// Modals
import PermitJoinModal from '@/components/modals/PermitJoinModal.vue'
import RenameModal from '@/components/modals/RenameModal.vue'
import CreateBindingModal from '@/components/modals/CreateBindingModal.vue'
import SpawnDeviceModal from '@/components/modals/SpawnDeviceModal.vue'

// API & WS Clients
const api = useZigBridgeApi()
const { addEventListener } = useZigBridgeWs()

// State
const status = ref<BridgeStatus | null>(null)
const devices = ref<Device[]>([])
const bindings = ref<Binding[]>([])
const recommendations = ref<Recommendation[]>([])
const virtualDevices = ref<VirtualDeviceSummary[]>([])

// Router and navigation
const {
  activeTab,
  currentDeviceIeee,
  getRoutePath,
  navigateToTab,
  navigateToDevice,
  navigateToHome: routerNavigateToHome,
  initRouter,
} = useRouter()

const currentDetailDevice = ref<Device | null>(null)
const currentDetailDefinition = ref<DeviceDefinition | null>(null)
let cleanupRouter: (() => void) | null = null

// Feed & logs
const activityItems = ref<ActivityItem[]>([])
const rawLogs = ref<RawLogItem[]>([])
const permitJoinRemaining = ref<number>(0)

// Modals
const showPermitJoin = ref(false)
const showRename = ref(false)
const deviceToRename = ref<Device | null>(null)
const showCreateBinding = ref(false)
const showSpawnDevice = ref(false)

// Polling and WS handles
let pollTimer: ReturnType<typeof setInterval> | null = null
let removeWsListener: (() => void) | null = null

// Computed
const isCoordinatorOnline = computed(() => {
  return Boolean(
    status.value?.connected &&
      (status.value?.coordinator?.status === 'ready' || status.value?.coordinator?.status === 'running')
  )
})

const showAiTab = computed(() => Boolean(status.value?.ai_enabled))
const showSimulationTab = computed(() => status.value?.coordinator?.type === 'mock')

const deviceCount = computed(() => devices.value.length || status.value?.device_count || 0)
const bindingCount = computed(() => bindings.value.length || status.value?.binding_count || 0)
const simCount = computed(() => virtualDevices.value.length)
const recCount = computed(() => recommendations.value.length)

// Device name lookup
function getDeviceName(ieee: string): string {
  if (!ieee) return 'Unknown'
  const dev = devices.value.find((d) => d.ieee === ieee)
  return dev?.friendly_name || ieee
}

// Data loaders
async function loadStatus() {
  try {
    const data = await api.getStatus()
    status.value = data
    if (typeof data.permit_join_remaining === 'number') {
      permitJoinRemaining.value = data.permit_join_remaining
    }
  } catch (err) {
    console.error('Failed to load status:', err)
  }
}

async function loadDevices() {
  try {
    const list = await api.getDevices()
    devices.value = sortDevicesByFriendlyName(list || [])
  } catch (err) {
    console.error('Failed to load devices:', err)
  }
}

async function loadBindings() {
  try {
    const list = await api.getBindings()
    bindings.value = list || []
  } catch (err) {
    console.error('Failed to load bindings:', err)
  }
}

async function loadSimulationLab() {
  try {
    const list = await api.getVirtualDevices()
    virtualDevices.value = list || []
  } catch (err) {
    console.error('Failed to load virtual devices:', err)
  }
}

async function loadRecommendations() {
  try {
    const list = await api.getRecommendations()
    recommendations.value = list || []
  } catch (err) {
    console.error('Failed to load recommendations:', err)
  }
}

function loadTabData(tabId: TabId) {
  if (tabId === 'devices') loadDevices()
  else if (tabId === 'bindings') loadBindings()
  else if (tabId === 'diagnostics') loadStatus()
  else if (tabId === 'simulation') loadSimulationLab()
  else if (tabId === 'ai') loadRecommendations()
}

// Device Detail Navigation
async function loadDeviceDetail(ieee: string) {
  try {
    const data = await api.getDeviceDetail(ieee)
    currentDetailDevice.value = data.device
    currentDetailDefinition.value = data.definition || null
  } catch (err: any) {
    console.error('Failed to load device detail:', err)
    alert(`Failed to load device: ${err?.message || 'Device not found'}`)
    navigateToDevices()
  }
}

function openDeviceDetail(ieee: string) {
  navigateToDevice(ieee)
  loadDeviceDetail(ieee)
}

function navigateToDevices() {
  currentDetailDevice.value = null
  currentDetailDefinition.value = null
  navigateToTab('devices')
  loadDevices()
}

function navigateToHome() {
  showPermitJoin.value = false
  showRename.value = false
  showCreateBinding.value = false
  showSpawnDevice.value = false
  currentDetailDevice.value = null
  currentDetailDefinition.value = null
  routerNavigateToHome()
  loadDevices()
}

function onNavigateHome(event: MouseEvent) {
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
    return
  }
  event.preventDefault()
  navigateToHome()
}

function switchTab(tabId: TabId) {
  currentDetailDevice.value = null
  currentDetailDefinition.value = null
  navigateToTab(tabId)
  loadTabData(tabId)
}

function onTabClick(event: MouseEvent, tabId: TabId) {
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
    return
  }
  event.preventDefault()
  switchTab(tabId)
}

// Activity feed and log console
function dispatchFriendlyActivity(type: string, payload: any) {
  const time = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  const id = `act-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`

  if (type === 'device_join') {
    const devName = payload.friendly_name || payload.ieee || 'New Device'
    activityItems.value.unshift({
      id,
      iconType: 'join',
      iconClass: 'join',
      title: '✨ New Device Paired',
      meta: `${devName} joined the Zigbee mesh network.`,
      time,
    })
  } else if (type === 'device_state_change' || type === 'device_state') {
    const devName = getDeviceName(payload.ieee)
    let stateDesc = 'Reported attribute update.'
    if (payload.state && typeof payload.state.state === 'string') {
      stateDesc = `Turned ${payload.state.state.toUpperCase()}.`
    } else if (payload.state && payload.state.occupancy !== undefined) {
      stateDesc = payload.state.occupancy ? 'Detected motion.' : 'Motion clear.'
    } else if (payload.state && payload.state.temperature !== undefined) {
      stateDesc = `Temperature measured: ${payload.state.temperature}°C.`
    }
    activityItems.value.unshift({
      id,
      iconType: 'light',
      iconClass: 'light',
      title: `💡 ${devName}`,
      meta: stateDesc,
      time,
    })
  } else if (type === 'permit_join') {
    if (payload.remaining > 0) {
      activityItems.value.unshift({
        id,
        iconType: 'system',
        iconClass: 'system',
        title: '🔓 Pairing Mode Active',
        meta: `Network is open to pair new devices (${payload.remaining}s remaining).`,
        time,
      })
    } else {
      activityItems.value.unshift({
        id,
        iconType: 'system',
        iconClass: 'system',
        title: '🔒 Pairing Window Closed',
        meta: 'Network pairing window has closed.',
        time,
      })
    }
  } else if (type === 'binding_change') {
    activityItems.value.unshift({
      id,
      iconType: 'switch',
      iconClass: 'switch',
      title: '🔗 Direct Link Synchronized',
      meta: `Direct binding updated for ${getDeviceName(payload.source_ieee || payload.src_ieee)}.`,
      time,
    })
  } else if (type === 'system_warning') {
    activityItems.value.unshift({
      id,
      iconType: 'system',
      iconClass: 'system',
      title: '⚠️ Notice',
      meta: String(payload),
      time,
    })
  } else if (type === 'system' && payload?.title) {
    activityItems.value.unshift({
      id,
      iconType: 'system',
      iconClass: 'system',
      title: payload.title,
      meta: payload.meta || '',
      time,
    })
  }

  if (activityItems.value.length > 50) {
    activityItems.value.length = 50
  }
}

function appendRawLog(type: string, payload: any) {
  let typeClass: RawLogItem['typeClass'] = 'frame'
  if (type === 'device_join') typeClass = 'join'
  else if (type === 'binding_change') typeClass = 'binding'
  else if (type === 'system_warning') typeClass = 'warn'

  const message = typeof payload === 'string' ? payload : JSON.stringify(payload)
  const time = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  const id = `log-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`

  rawLogs.value.unshift({
    id,
    time,
    type: type.toUpperCase(),
    message,
    typeClass,
  })

  if (rawLogs.value.length > 200) {
    rawLogs.value.length = 200
  }
}

function handleLiveEvent(type: string, payload: any) {
  // 1. Dispatch activity timeline item
  dispatchFriendlyActivity(type, payload)

  // 2. Append to raw log stream
  appendRawLog(type, payload)

  // 3. Dynamic UI updates
  if (type === 'permit_join') {
    if (typeof payload?.remaining === 'number') {
      permitJoinRemaining.value = payload.remaining
    }
  } else if (type === 'device_join' || type === 'device_state_change' || type === 'device_state') {
    loadDevices()
    loadStatus()
    if (currentDetailDevice.value && payload?.ieee === currentDetailDevice.value.ieee) {
      if (payload.state) {
        Object.assign(currentDetailDevice.value, {
          ...payload.state,
          battery: payload.state.battery ?? currentDetailDevice.value.battery,
          voltage: payload.state.voltage ?? currentDetailDevice.value.voltage,
        })
      }
    }
  } else if (type === 'binding_change') {
    loadBindings()
    loadStatus()
  }
}

function clearActivityFeed() {
  activityItems.value = []
  rawLogs.value = []
}

// Modal actions
async function onPermitJoinSubmit(duration: number) {
  try {
    await api.permitJoin(duration)
    showPermitJoin.value = false
    await loadStatus()
  } catch (err: any) {
    alert(`Failed to open network: ${err?.message || 'Unknown error'}`)
  }
}

async function onPermitJoinStop() {
  try {
    await api.permitJoin(0)
    showPermitJoin.value = false
    await loadStatus()
  } catch (err: any) {
    alert(`Failed to close network: ${err?.message || 'Unknown error'}`)
  }
}

function openRenameModal(device: Device) {
  deviceToRename.value = device
  showRename.value = true
}

function onDeviceRenamed(ieee: string, newName: string) {
  if (currentDetailDevice.value && currentDetailDevice.value.ieee === ieee) {
    currentDetailDevice.value.friendly_name = newName
  }
  loadDevices()
}

function onBindingCreated() {
  loadBindings()
  loadStatus()
}

function onDeviceSpawned() {
  loadSimulationLab()
  loadDevices()
  loadStatus()
}

function onAiApplied() {
  loadBindings()
  loadStatus()
}

async function refreshDetailDevice() {
  const ieee = currentDeviceIeee.value
  if (!ieee) return
  try {
    const data = await api.getDeviceDetail(ieee)
    currentDetailDevice.value = data.device
    currentDetailDefinition.value = data.definition || null
  } catch (err) {
    console.error('Failed to refresh device detail:', err)
  }
}

// Watchers
watch(
  () => status.value?.ai_enabled,
  (enabled) => {
    if (enabled === false && activeTab.value === 'ai') {
      navigateToTab('devices', true)
      loadDevices()
    }
  }
)

// Lifecycle
onMounted(() => {
  // Load initial status and data
  loadStatus()
  loadDevices()
  loadBindings()

  // Initialize router and listen to popstate/hashchange events
  cleanupRouter = initRouter((route) => {
    if (route.deviceIeee) {
      loadDeviceDetail(route.deviceIeee)
    } else {
      currentDetailDevice.value = null
      currentDetailDefinition.value = null
      loadTabData(route.tab)
    }
  })

  // Polling every 3s
  pollTimer = setInterval(loadStatus, 3000)

  // WebSocket event stream
  removeWsListener = addEventListener((type: string, payload: any) => {
    handleLiveEvent(type, payload)
  })
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
  if (cleanupRouter) cleanupRouter()
  if (removeWsListener) removeWsListener()
})
</script>

<template>
  <div class="min-h-screen flex flex-col bg-gray-50 dark:bg-[#0d1117] text-gray-900 dark:text-gray-100 transition-colors duration-200">
    <!-- Header -->
    <AppHeader :status="status" @navigate-home="onNavigateHome" />

    <!-- Navigation Tabs -->
    <nav class="bg-white dark:bg-[#161b22] border-b border-gray-200 dark:border-[#30363d] px-6 transition-colors">
      <div class="max-w-[1300px] mx-auto flex items-center space-x-1 sm:space-x-2 overflow-x-auto">
        <!-- Devices Tab -->
        <a
          :href="getRoutePath('devices')"
          :class="[
            'group inline-flex items-center gap-2 py-3 px-3 text-xs sm:text-sm font-medium border-b-2 transition-colors whitespace-nowrap cursor-pointer',
            activeTab === 'devices' && !currentDetailDevice
              ? 'border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400'
              : 'border-transparent text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'
          ]"
          @click="onTabClick($event, 'devices')"
        >
          <span>Devices</span>
          <span
            :class="[
              'px-1.5 py-0.5 rounded-full text-[11px] font-semibold transition-colors',
              activeTab === 'devices' && !currentDetailDevice
                ? 'bg-blue-100 dark:bg-blue-950/80 text-blue-700 dark:text-blue-300'
                : 'bg-gray-100 dark:bg-[#21262d] text-gray-600 dark:text-gray-400'
            ]"
          >
            {{ deviceCount }}
          </span>
        </a>

        <!-- Bindings Tab -->
        <a
          :href="getRoutePath('bindings')"
          :class="[
            'group inline-flex items-center gap-2 py-3 px-3 text-xs sm:text-sm font-medium border-b-2 transition-colors whitespace-nowrap cursor-pointer',
            activeTab === 'bindings' && !currentDetailDevice
              ? 'border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400'
              : 'border-transparent text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'
          ]"
          @click="onTabClick($event, 'bindings')"
        >
          <span>Bindings</span>
          <span
            :class="[
              'px-1.5 py-0.5 rounded-full text-[11px] font-semibold transition-colors',
              activeTab === 'bindings' && !currentDetailDevice
                ? 'bg-blue-100 dark:bg-blue-950/80 text-blue-700 dark:text-blue-300'
                : 'bg-gray-100 dark:bg-[#21262d] text-gray-600 dark:text-gray-400'
            ]"
          >
            {{ bindingCount }}
          </span>
        </a>

        <!-- Smart Suggestions (AI) Tab -->
        <a
          v-if="showAiTab"
          :href="getRoutePath('ai')"
          :class="[
            'group inline-flex items-center gap-2 py-3 px-3 text-xs sm:text-sm font-medium border-b-2 transition-colors whitespace-nowrap cursor-pointer',
            activeTab === 'ai' && !currentDetailDevice
              ? 'border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400'
              : 'border-transparent text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'
          ]"
          @click="onTabClick($event, 'ai')"
        >
          <span>Suggestions</span>
          <span
            v-if="recCount > 0"
            :class="[
              'px-1.5 py-0.5 rounded-full text-[11px] font-semibold transition-colors',
              activeTab === 'ai' && !currentDetailDevice
                ? 'bg-blue-100 dark:bg-blue-950/80 text-blue-700 dark:text-blue-300'
                : 'bg-gray-100 dark:bg-[#21262d] text-gray-600 dark:text-gray-400'
            ]"
          >
            {{ recCount }}
          </span>
        </a>

        <!-- Activity Tab -->
        <a
          :href="getRoutePath('events')"
          :class="[
            'group inline-flex items-center gap-2 py-3 px-3 text-xs sm:text-sm font-medium border-b-2 transition-colors whitespace-nowrap cursor-pointer',
            activeTab === 'events' && !currentDetailDevice
              ? 'border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400'
              : 'border-transparent text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'
          ]"
          @click="onTabClick($event, 'events')"
        >
          <span>Activity</span>
        </a>

        <!-- System (Diagnostics) Tab -->
        <a
          :href="getRoutePath('diagnostics')"
          :class="[
            'group inline-flex items-center gap-2 py-3 px-3 text-xs sm:text-sm font-medium border-b-2 transition-colors whitespace-nowrap cursor-pointer',
            activeTab === 'diagnostics' && !currentDetailDevice
              ? 'border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400'
              : 'border-transparent text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'
          ]"
          @click="onTabClick($event, 'diagnostics')"
        >
          <span>System</span>
        </a>

        <!-- Simulation Lab Tab (Only for mock coordinator) -->
        <a
          v-if="showSimulationTab"
          :href="getRoutePath('simulation')"
          :class="[
            'group inline-flex items-center gap-2 py-3 px-3 text-xs sm:text-sm font-medium border-b-2 transition-colors whitespace-nowrap cursor-pointer',
            activeTab === 'simulation' && !currentDetailDevice
              ? 'border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400'
              : 'border-transparent text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'
          ]"
          @click="onTabClick($event, 'simulation')"
        >
          <span>Simulation</span>
          <span
            v-if="simCount > 0"
            :class="[
              'px-1.5 py-0.5 rounded-full text-[11px] font-semibold transition-colors',
              activeTab === 'simulation' && !currentDetailDevice
                ? 'bg-blue-100 dark:bg-blue-950/80 text-blue-700 dark:text-blue-300'
                : 'bg-gray-100 dark:bg-[#21262d] text-gray-600 dark:text-gray-400'
            ]"
          >
            {{ simCount }}
          </span>
        </a>
      </div>
    </nav>

    <!-- Coordinator Disconnected Alert Banner -->
    <OfflineBanner :visible="!isCoordinatorOnline" />

    <!-- Main Content Area -->
    <main class="flex-1 max-w-[1300px] w-full mx-auto p-4 sm:p-6">
      <!-- Device Detail View -->
      <DeviceDetail
        v-if="currentDetailDevice"
        :device="currentDetailDevice"
        :definition="currentDetailDefinition"
        :coordinator-online="isCoordinatorOnline"
        @back="navigateToDevices"
        @rename="openRenameModal"
        @updated="refreshDetailDevice"
      />

      <!-- Devices Tab -->
      <DevicesTab
        v-else-if="activeTab === 'devices'"
        :devices="devices"
        :permit-join-remaining="permitJoinRemaining"
        :coordinator-online="isCoordinatorOnline"
        @select-device="openDeviceDetail"
        @open-permit-join="showPermitJoin = true"
      />

      <!-- Bindings Tab -->
      <BindingsTab
        v-else-if="activeTab === 'bindings'"
        :bindings="bindings"
        :devices="devices"
        :coordinator-online="isCoordinatorOnline"
        @create="showCreateBinding = true"
        @refresh="loadBindings"
      />

      <!-- Smart Suggestions Tab -->
      <AiTab
        v-else-if="activeTab === 'ai'"
        v-model:recommendations="recommendations"
        :devices="devices"
        :coordinator-online="isCoordinatorOnline"
        @applied="onAiApplied"
      />

      <!-- Activity Tab -->
      <ActivityTab
        v-else-if="activeTab === 'events'"
        :activity-items="activityItems"
        :raw-logs="rawLogs"
        @clear="clearActivityFeed"
      />

      <!-- Diagnostics Tab -->
      <DiagnosticsTab
        v-else-if="activeTab === 'diagnostics'"
        :status="status"
        @refresh="loadStatus"
      />

      <!-- Simulation Lab Tab -->
      <SimulationTab
        v-else-if="activeTab === 'simulation'"
        :devices="virtualDevices"
        @refresh="loadSimulationLab"
        @spawn="showSpawnDevice = true"
      />
    </main>

    <!-- Standard Footer -->
    <AppFooter :version="status?.version" :commit="status?.commit" />

    <!-- Global Modals -->
    <PermitJoinModal
      :open="showPermitJoin"
      :is-coordinator-online="isCoordinatorOnline"
      @close="showPermitJoin = false"
      @submit="onPermitJoinSubmit"
      @stop="onPermitJoinStop"
    />

    <RenameModal
      :show="showRename"
      :device="deviceToRename"
      @close="showRename = false"
      @saved="onDeviceRenamed"
    />

    <CreateBindingModal
      :show="showCreateBinding"
      :devices="devices"
      :coordinator-online="isCoordinatorOnline"
      @close="showCreateBinding = false"
      @created="onBindingCreated"
    />

    <SpawnDeviceModal
      :show="showSpawnDevice"
      @close="showSpawnDevice = false"
      @spawned="onDeviceSpawned"
    />
  </div>
</template>
