<script setup lang="ts">
import { ref } from 'vue'
import ModalDialog from '@/components/common/ModalDialog.vue'

defineProps<{
  open: boolean
  isCoordinatorOnline: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'submit', duration: number): void
  (e: 'stop'): void
}>()

const selectedDuration = ref<number>(60)

function onStart() {
  emit('submit', Number(selectedDuration.value))
}

function onStop() {
  emit('stop')
}
</script>

<template>
  <ModalDialog :open="open" title="Pair New Devices (Permit Join)" @close="emit('close')">
    <p class="text-xs sm:text-sm text-gray-600 dark:text-gray-400 mb-4">
      Opening the Zigbee network allows new battery switches, sensors, and smart plugs to pair. Put your Zigbee device into pairing mode now.
    </p>

    <div class="space-y-2">
      <label for="permit-join-duration-select" class="block text-xs font-semibold uppercase text-gray-500 dark:text-gray-400">
        Pairing Window Duration
      </label>
      <select
        id="permit-join-duration-select"
        v-model.number="selectedDuration"
        class="w-full bg-gray-50 dark:bg-[#0d1117] border border-gray-300 dark:border-[#30363d] rounded-lg px-3 py-2 text-sm text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500 transition-colors"
      >
        <option :value="60">1 Minute (60s)</option>
        <option :value="120">2 Minutes (120s)</option>
        <option :value="180">3 Minutes (180s)</option>
        <option :value="254">Maximum (254s)</option>
      </select>
    </div>

    <template #footer>
      <button
        type="button"
        class="px-3 py-1.5 text-xs font-semibold rounded-lg bg-rose-500/15 text-rose-600 dark:text-rose-400 border border-rose-500/30 hover:bg-rose-500 hover:text-white transition-colors disabled:opacity-50"
        :disabled="!isCoordinatorOnline"
        @click="onStop"
      >
        Stop Pairing (0s)
      </button>
      <button
        type="button"
        class="px-4 py-1.5 text-xs font-semibold rounded-lg bg-blue-600 hover:bg-blue-500 text-white transition-colors disabled:opacity-50"
        :disabled="!isCoordinatorOnline"
        @click="onStart"
      >
        Start Pairing
      </button>
    </template>
  </ModalDialog>
</template>
