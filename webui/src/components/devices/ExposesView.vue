<script setup lang="ts">
import { ref, computed } from 'vue'
import type { Device, DeviceDefinition, DeviceExpose } from '@/types'
import { getExposeIcon, getDeviceExposes } from '@/utils/formatters'
import { useZigbridgeApi } from '@/composables/useZigbridgeApi'

const props = defineProps<{
  device: Device
  definition?: DeviceDefinition | null
  coordinatorOnline: boolean
}>()

const emit = defineEmits<{
  (e: 'updated'): void
}>()

const api = useZigbridgeApi()

const actionStates = ref<Record<string, { loading: boolean; done: boolean }>>({})

const exposes = computed(() => getDeviceExposes(props.device, props.definition))

function getPropertyValue(prop: string): any {
  if (props.device.state && props.device.state[prop] !== undefined) {
    return props.device.state[prop]
  }
  if (prop === 'linkquality') return props.device.lqi
  if (prop === 'battery') return props.device.battery
  if (prop === 'voltage' && props.device.voltage) {
    return Number((props.device.voltage / 1000).toFixed(1))
  }
  return undefined
}

async function setProperty(prop: string, val: any) {
  if (!props.coordinatorOnline) {
    alert('Coordinator is offline. Cannot send control commands to device.')
    return
  }
  try {
    // Optimistically update device state locally
    if (!props.device.state) props.device.state = {}
    props.device.state[prop] = val

    await api.setDeviceProperty(props.device.ieee, { [prop]: val })
    emit('updated')
  } catch (err: any) {
    alert(`Failed to set ${prop}: ${err?.message || 'Unknown error'}`)
    emit('updated')
  }
}

function handleBinaryToggle(exp: DeviceExpose, currentChecked: boolean) {
  const values = exp.values && exp.values.length >= 2 ? exp.values : ['OFF', 'ON']
  const offVal = values[0]
  const onVal = values[1]
  const nextChecked = !currentChecked
  const chosenVal = nextChecked ? onVal : offVal
  let finalVal: any = chosenVal
  if (chosenVal === 'true') finalVal = true
  else if (chosenVal === 'false') finalVal = false
  setProperty(exp.property, finalVal)
}

function isBinaryChecked(exp: DeviceExpose, val: any): boolean {
  const values = exp.values && exp.values.length >= 2 ? exp.values : ['OFF', 'ON']
  const onVal = values[1]
  if (typeof val === 'boolean') return val
  if (val !== undefined && val !== null) {
    return String(val).toUpperCase() === String(onVal).toUpperCase()
  }
  return false
}

async function handleAction(action: string) {
  if (!props.coordinatorOnline) {
    alert('Coordinator is offline. Cannot trigger device actions.')
    return
  }

  actionStates.value[action] = { loading: true, done: false }
  try {
    await api.triggerDeviceAction(props.device.ieee, action)
    actionStates.value[action] = { loading: false, done: true }
    setTimeout(() => {
      actionStates.value[action] = { loading: false, done: false }
    }, 2000)
    emit('updated')
  } catch (err: any) {
    actionStates.value[action] = { loading: false, done: false }
    alert(`Failed to trigger ${action}: ${err?.message || 'Unknown error'}`)
  }
}
</script>

<template>
  <div class="space-y-4">
    <!-- Coordinator Offline Warning -->
    <div
      v-if="!coordinatorOnline"
      class="flex items-center space-x-3 p-3 text-xs bg-amber-50 dark:bg-amber-950/40 text-amber-800 dark:text-amber-200 border border-amber-200 dark:border-amber-800 rounded-lg"
    >
      <span class="text-base">⚠️</span>
      <div>
        <strong>Coordinator Offline:</strong> Controls are disabled. Physical commands cannot be transmitted over the radio mesh until the coordinator is reconnected.
      </div>
    </div>

    <!-- Empty state -->
    <div
      v-if="exposes.length === 0"
      class="text-center py-10 text-xs text-slate-500 dark:text-slate-400"
    >
      No controllable exposes or sensor metrics available for this device.
    </div>

    <!-- Expose Rows -->
    <div v-else class="divide-y divide-slate-100 dark:divide-slate-800/80 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-xs overflow-hidden">
      <div
        v-for="exp in exposes"
        :key="exp.property"
        class="flex flex-col sm:flex-row sm:items-center justify-between p-4 gap-4 hover:bg-slate-50/50 dark:hover:bg-slate-800/30 transition-colors"
      >
        <!-- Info: Icon + Name + Tooltip + Description -->
        <div class="flex items-start space-x-3.5">
          <div class="w-9 h-9 flex items-center justify-center text-lg rounded-lg bg-slate-100 dark:bg-slate-800 shrink-0">
            {{ getExposeIcon(exp.property, exp.type) }}
          </div>
          <div>
            <div class="flex items-center space-x-2">
              <span class="text-xs font-semibold text-slate-800 dark:text-slate-100">
                {{ exp.name || exp.property }}
              </span>
              <span class="px-1.5 py-0.2 text-[10px] font-mono rounded bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 border border-slate-200 dark:border-slate-700">
                {{ exp.property }}
              </span>
            </div>
            <div v-if="exp.description" class="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">
              {{ exp.description }}
            </div>
          </div>
        </div>

        <!-- Control / Readout -->
        <div class="shrink-0 flex items-center">
          <!-- Binary toggle -->
          <template v-if="exp.type === 'binary'">
            <div class="flex items-center space-x-2">
              <span
                class="text-[11px] font-medium transition-colors cursor-pointer select-none"
                :class="[!isBinaryChecked(exp, getPropertyValue(exp.property)) ? 'text-blue-600 dark:text-blue-400 font-bold' : 'text-slate-400 dark:text-slate-500']"
                @click="!coordinatorOnline ? null : setProperty(exp.property, exp.values?.[0] || 'OFF')"
              >
                {{ exp.values?.[0] || 'OFF' }}
              </span>

              <button
                type="button"
                role="switch"
                :aria-checked="isBinaryChecked(exp, getPropertyValue(exp.property))"
                :disabled="!coordinatorOnline"
                :title="!coordinatorOnline ? 'Coordinator offline: controls disabled' : undefined"
                class="relative inline-flex h-5 w-10 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
                :class="[isBinaryChecked(exp, getPropertyValue(exp.property)) ? 'bg-blue-600' : 'bg-slate-300 dark:bg-slate-700']"
                @click="handleBinaryToggle(exp, isBinaryChecked(exp, getPropertyValue(exp.property)))"
              >
                <span
                  aria-hidden="true"
                  class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-sm ring-0 transition duration-200 ease-in-out"
                  :class="[isBinaryChecked(exp, getPropertyValue(exp.property)) ? 'translate-x-5' : 'translate-x-0']"
                />
              </button>

              <span
                class="text-[11px] font-medium transition-colors cursor-pointer select-none"
                :class="[isBinaryChecked(exp, getPropertyValue(exp.property)) ? 'text-blue-600 dark:text-blue-400 font-bold' : 'text-slate-400 dark:text-slate-500']"
                @click="!coordinatorOnline ? null : setProperty(exp.property, exp.values?.[1] || 'ON')"
              >
                {{ exp.values?.[1] || 'ON' }}
              </span>
            </div>
          </template>

          <!-- Enum chips -->
          <template v-else-if="exp.type === 'enum'">
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="v in exp.values || []"
                :key="String(v)"
                type="button"
                :disabled="!coordinatorOnline"
                :title="!coordinatorOnline ? 'Coordinator offline: controls disabled' : undefined"
                :class="[
                  'px-2.5 py-1 text-xs rounded-md font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50',
                  String(getPropertyValue(exp.property) || '').toLowerCase() === String(v).toLowerCase()
                    ? 'bg-blue-600 text-white shadow-xs'
                    : 'bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-700'
                ]"
                @click="setProperty(exp.property, v)"
              >
                {{ v }}
              </button>
            </div>
          </template>

          <!-- Settable Numeric slider -->
          <template v-else-if="exp.type === 'numeric' && (((exp.access || 0) & 2) !== 0) && exp.min !== undefined && exp.max !== undefined">
            <div class="flex items-center space-x-3 w-64">
              <div class="flex-1">
                <input
                  type="range"
                  :min="exp.min"
                  :max="exp.max"
                  :value="getPropertyValue(exp.property) ?? exp.min"
                  :disabled="!coordinatorOnline"
                  :title="!coordinatorOnline ? 'Coordinator offline: controls disabled' : undefined"
                  class="w-full accent-blue-600 dark:accent-blue-400 cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
                  @change="(e: any) => setProperty(exp.property, Number(e.target.value))"
                />
                <div class="flex justify-between text-[10px] text-slate-400 dark:text-slate-500">
                  <span>{{ exp.min }}</span>
                  <span>{{ exp.max }}</span>
                </div>
              </div>

              <div class="flex items-center space-x-1 shrink-0">
                <input
                  type="number"
                  :min="exp.min"
                  :max="exp.max"
                  :value="getPropertyValue(exp.property) ?? exp.min"
                  :disabled="!coordinatorOnline"
                  :title="!coordinatorOnline ? 'Coordinator offline: controls disabled' : undefined"
                  class="w-16 px-2 py-1 text-xs text-right bg-white dark:bg-slate-800 border border-slate-300 dark:border-slate-700 rounded-md text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
                  @change="(e: any) => setProperty(exp.property, Number(e.target.value))"
                />
                <span v-if="exp.unit" class="text-xs text-slate-500 dark:text-slate-400">{{ exp.unit }}</span>
              </div>
            </div>
          </template>

          <!-- Action button -->
          <template v-else-if="exp.type === 'action'">
            <button
              type="button"
              :disabled="!coordinatorOnline || actionStates[exp.property]?.loading"
              :title="!coordinatorOnline ? 'Coordinator offline: actions disabled' : undefined"
              class="px-3 py-1.5 text-xs font-medium rounded-md bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-800 dark:text-slate-200 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              @click="handleAction(exp.property)"
            >
              <span v-if="actionStates[exp.property]?.loading">⏳ Sending...</span>
              <span v-else-if="actionStates[exp.property]?.done">✓ Sent</span>
              <span v-else>{{ exp.name || exp.property }}</span>
            </button>
          </template>

          <!-- Readonly Metric readout (numeric or other) -->
          <template v-else>
            <div class="flex items-baseline space-x-1 px-3 py-1.5 rounded-md bg-slate-50 dark:bg-slate-800/60 border border-slate-200/80 dark:border-slate-800">
              <span class="text-xs font-semibold text-slate-800 dark:text-slate-200 font-mono">
                {{ getPropertyValue(exp.property) !== undefined && getPropertyValue(exp.property) !== null ? getPropertyValue(exp.property) : '--' }}
              </span>
              <span v-if="exp.unit" class="text-[11px] text-slate-500 dark:text-slate-400">
                {{ exp.unit }}
              </span>
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
