<script setup lang="ts">
import { ref } from 'vue'

const isDarkMode = ref(false)

function toggleDarkMode() {
  isDarkMode.value = !isDarkMode.value
  if (isDarkMode.value) {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
}
</script>

<template>
  <div class="min-h-screen flex flex-col items-center justify-center p-6 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-gray-100 transition-colors duration-200">
    <header class="text-center mb-8">
      <h1 class="text-3xl font-bold tracking-tight sm:text-4xl">Zigbridge Web UI</h1>
      <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
        Vue 3 + TypeScript + Tailwind CSS
      </p>
    </header>

    <main class="w-full max-w-md p-6 bg-white dark:bg-gray-800 rounded-xl shadow-md border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <span class="text-base font-medium">Dark Mode</span>
          <p class="text-xs text-gray-500 dark:text-gray-400">Toggle dark variant styling</p>
        </div>
        <button
          type="button"
          class="px-4 py-2 text-sm font-medium rounded-lg transition-colors bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
          @click="toggleDarkMode"
        >
          {{ isDarkMode ? '🌙 Dark' : '☀️ Light' }}
        </button>
      </div>
    </main>
  </div>
</template>
