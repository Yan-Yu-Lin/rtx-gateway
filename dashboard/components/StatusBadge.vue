<template>
  <span
    class="rounded-full px-2 py-1 text-xs font-semibold"
    :class="status < 400 ? 'bg-emerald-500/15 text-emerald-300' : 'bg-red-500/15 text-red-300'"
  >
    {{ status }}
  </span>
</template>

<script setup lang="ts">
defineProps<{ status: number }>()
</script>
