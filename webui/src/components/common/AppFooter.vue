<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  version?: string
  commit?: string
}>()

const formattedVersion = computed(() => {
  if (!props.version) return 'v1.0.0'
  return props.version.startsWith('v') || props.version === 'dev' ? props.version : `v${props.version}`
})
</script>

<template>
  <footer
    class="bg-white dark:bg-[#161b22] border-t border-gray-200 dark:border-[#30363d] px-6 py-3.5 mt-auto text-xs text-gray-500 dark:text-gray-400 transition-colors"
  >
    <div class="max-w-[1300px] mx-auto flex items-center justify-between flex-wrap gap-3">
      <!-- Left: GitHub & Version -->
      <div class="flex items-center gap-2.5">
        <a
          href="https://github.com/julienbreux/zigbridge"
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex items-center gap-1.5 hover:text-gray-900 dark:hover:text-gray-200 transition-colors"
        >
          <svg class="w-4 h-4 fill-current" viewBox="0 0 16 16" aria-hidden="true">
            <path
              d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"
            />
          </svg>
          <span>GitHub</span>
        </a>
        <span class="text-gray-300 dark:text-gray-600">•</span>
        <span class="font-mono text-[11px]" :title="commit ? `Commit: ${commit}` : 'Zigbridge Version'">
          Zigbridge <span class="text-blue-500 font-semibold">{{ formattedVersion }}</span>
        </span>
      </div>

      <!-- Right: Author attribution -->
      <div class="flex items-center">
        <span>
          With <span class="text-rose-500 inline-block hover:scale-125 transition-transform" title="love">❤️</span> by
          <a
            href="https://github.com/julienbreux"
            target="_blank"
            rel="noopener noreferrer"
            class="text-gray-700 dark:text-gray-300 hover:text-blue-500 dark:hover:text-blue-400 font-medium transition-colors"
          >
            Julien Breux
          </a>
        </span>
      </div>
    </div>
  </footer>
</template>
