<template>
  <section class="panel p-4">
    <p class="text-sm text-zinc-400">{{ label }}</p>
    <p class="mt-3 text-3xl font-semibold text-zinc-50">{{ value }}</p>
    <p v-if="hint" class="mt-2 text-xs text-zinc-500">{{ hint }}</p>
  </section>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  value: string
  hint?: string
}>()
</script>
