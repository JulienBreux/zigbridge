<script setup lang="ts">
import { ref, watch } from 'vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { useMode } from '@/composables/useMode'
import { useZigBridgeApi } from '@/composables/useZigBridgeApi'
import type { DefinitionSummary } from '@/types'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'spawned'): void
}>()

const { isAdvanced } = useMode()
const api = useZigBridgeApi()

const definitions = ref<DefinitionSummary[]>([])
const selectedModel = ref('')
const customIeee = ref('')
const customNwk = ref('')

const isLoading = ref(false)
const isSubmitting = ref(false)
const errorMessage = ref<string | null>(null)

watch(
  () => props.show,
  async (open) => {
    if (open) {
      errorMessage.value = null
      customIeee.value = ''
      customNwk.value = ''
      if (definitions.value.length === 0) {
        isLoading.value = true
        try {
          const list = await api.getTestDefinitions()
          definitions.value = list || []
          if (definitions.value.length > 0) {
            selectedModel.value = definitions.value[0].model
          }
        } catch (err: any) {
          errorMessage.value = err?.message || 'Failed to load device definitions'
        } finally {
          isLoading.value = false
        }
      } else if (!selectedModel.value && definitions.value.length > 0) {
        selectedModel.value = definitions.value[0].model
      }
    }
  }
)

async function handleSubmit() {
  if (!selectedModel.value) {
    errorMessage.value = 'Please select a device definition'
    return
  }

  isSubmitting.value = true
  errorMessage.value = null

  const payload: { model: string; ieee?: string; nwk?: number } = {
    model: selectedModel.value
  }

  if (customIeee.value.trim() !== '') {
    payload.ieee = customIeee.value.trim()
  }

  if (customNwk.value.trim() !== '') {
    const raw = customNwk.value.trim()
    const parsed = parseInt(raw, 10) || parseInt(raw, 16)
    if (!isNaN(parsed)) {
      payload.nwk = parsed
    }
  }

  try {
    await api.spawnTestDevice(payload)
    emit('spawned')
    emit('close')
  } catch (err: any) {
    errorMessage.value = err?.message || 'Failed to spawn virtual device'
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <ModalDialog :show="show" title="Spawn Virtual Zigbee Device" @close="emit('close')">
    <div class="space-y-4">
      <p class="text-xs text-slate-500 dark:text-slate-400">
        Select a device definition from the fixture registry. A virtual device will be instantiated and emit real Zigbee frames through the mock adapter.
      </p>

      <div v-if="errorMessage" class="p-3 text-xs text-red-700 bg-red-50 dark:bg-red-950/40 dark:text-red-300 rounded-md border border-red-200 dark:border-red-900">
        {{ errorMessage }}
      </div>

      <div v-if="isLoading" class="py-4 text-center text-xs text-slate-500">
        Loading device fixtures...
      </div>

      <div v-else class="space-y-4">
        <!-- Definition / Model -->
        <div class="space-y-1.5">
          <label for="spawn-model" class="block text-xs font-medium text-slate-700 dark:text-slate-300">
            Device Model / Definition
          </label>
          <select
            id="spawn-model"
            v-model="selectedModel"
            class="w-full px-3 py-2 text-xs bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-700 rounded-md text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option v-for="d in definitions" :key="d.model" :value="d.model">
              {{ d.vendor }} {{ d.model }} — {{ d.description || d.vendor }}
            </option>
          </select>
        </div>

        <!-- Custom IEEE (Advanced) -->
        <div v-if="isAdvanced" class="space-y-1.5">
          <label for="spawn-ieee" class="block text-xs font-medium text-slate-700 dark:text-slate-300">
            Custom IEEE Address (optional)
          </label>
          <input
            id="spawn-ieee"
            v-model="customIeee"
            type="text"
            placeholder="Leave empty for auto-generated"
            class="w-full px-3 py-2 text-xs font-mono bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-700 rounded-md text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>

        <!-- Custom NWK (Advanced) -->
        <div v-if="isAdvanced" class="space-y-1.5">
          <label for="spawn-nwk" class="block text-xs font-medium text-slate-700 dark:text-slate-300">
            Custom NWK Address (optional)
          </label>
          <input
            id="spawn-nwk"
            v-model="customNwk"
            type="text"
            placeholder="Leave empty for auto-generated"
            class="w-full px-3 py-2 text-xs font-mono bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-700 rounded-md text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>
      </div>
    </div>

    <template #footer>
      <button
        type="button"
        class="px-4 py-2 text-xs font-medium text-slate-700 dark:text-slate-300 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 rounded-md transition-colors"
        @click="emit('close')"
      >
        Cancel
      </button>
      <button
        type="button"
        :disabled="isSubmitting || isLoading || !selectedModel"
        class="px-4 py-2 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed rounded-md transition-colors flex items-center space-x-1.5 shadow-sm"
        @click="handleSubmit"
      >
        <span v-if="isSubmitting">Spawning...</span>
        <span v-else>Spawn Device</span>
      </button>
    </template>
  </ModalDialog>
</template>
