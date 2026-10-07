<script setup lang="ts">
defineProps<{
  visible: boolean
}>()
</script>

<template>
  <Transition
    enter-active-class="transition duration-200 ease-out"
    enter-from-class="-translate-y-2 opacity-0"
    enter-to-class="translate-y-0 opacity-100"
    leave-active-class="transition duration-150 ease-in"
    leave-from-class="translate-y-0 opacity-100"
    leave-to-class="-translate-y-2 opacity-0"
  >
    <div
      v-if="visible"
      role="alert"
      class="bg-amber-500/15 border-b border-amber-500/35 text-amber-500 dark:text-amber-400 px-6 py-3 flex items-center gap-3.5"
    >
      <div class="text-xl leading-none">⚠️</div>
      <div class="flex flex-col text-xs sm:text-sm">
        <strong class="font-semibold text-gray-900 dark:text-gray-100">Coordinator Disconnected</strong>
        <span class="text-amber-600 dark:text-amber-300">
          Radio coprocessor is offline. Pairing devices, managing direct bindings, and device controls are disabled until the connection is restored.
        </span>
      </div>
    </div>
  </Transition>
</template>
