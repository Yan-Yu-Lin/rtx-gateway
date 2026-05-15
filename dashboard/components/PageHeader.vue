<template>
  <div class="mb-6 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
    <div>
      <p v-if="eyebrow" class="text-xs font-semibold uppercase tracking-[0.2em] text-cyan-400">
        {{ eyebrow }}
      </p>
      <h2 class="mt-1 text-2xl font-semibold text-zinc-50 sm:text-3xl">
        {{ title }}
      </h2>
      <p v-if="description" class="mt-2 max-w-2xl text-sm text-zinc-400">
        {{ description }}
      </p>
    </div>
    <slot />
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  eyebrow?: string
  description?: string
}>()
</script>
