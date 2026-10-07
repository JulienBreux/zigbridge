<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'

const props = withDefaults(
  defineProps<{
    open?: boolean
    show?: boolean
    title: string
    maxWidth?: string
  }>(),
  {
    open: undefined,
    show: undefined,
    maxWidth: 'max-w-lg',
  }
)

const emit = defineEmits<{
  (e: 'close'): void
}>()

const isOpen = computed(() => props.open ?? props.show ?? false)

function onKeyDown(e: KeyboardEvent) {
  if (e.key === 'Escape' && isOpen.value) {
    emit('close')
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKeyDown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown)
})
</script>

<template>
  <Transition
    enter-active-class="transition duration-150 ease-out"
    enter-from-class="opacity-0"
    enter-to-class="opacity-100"
    leave-active-class="transition duration-100 ease-in"
    leave-from-class="opacity-100"
    leave-to-class="opacity-0"
  >
    <div
      v-if="isOpen"
      class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm"
      @click.self="emit('close')"
    >
      <div
        class="bg-white dark:bg-[#161b22] border border-gray-200 dark:border-[#30363d] rounded-xl shadow-2xl w-full p-6 text-gray-900 dark:text-gray-100 transition-all max-h-[90vh] overflow-y-auto"
        :class="maxWidth || 'max-w-lg'"
      >
        <div class="flex items-center justify-between pb-3 mb-4 border-b border-gray-100 dark:border-[#30363d]">
          <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
            {{ title }}
          </h3>
          <button
            type="button"
            class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 text-lg leading-none p-1 rounded-md hover:bg-gray-100 dark:hover:bg-[#21262d] transition-colors"
            @click="emit('close')"
          >
            ✕
          </button>
        </div>

        <div class="mb-5 text-sm text-gray-700 dark:text-gray-300">
          <slot />
        </div>

        <div v-if="$slots.footer" class="flex justify-end gap-2 pt-3 border-t border-gray-100 dark:border-[#30363d]">
          <slot name="footer" />
        </div>
      </div>
    </div>
  </Transition>
</template>
