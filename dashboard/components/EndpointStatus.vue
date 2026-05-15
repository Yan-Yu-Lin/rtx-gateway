<template>
  <section class="panel p-4">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h3 class="text-base font-semibold text-zinc-50">{{ endpoint.id.toUpperCase() }}</h3>
        <p class="mt-1 break-all text-sm text-zinc-400">{{ endpoint.host }}</p>
      </div>
      <span
        class="rounded-full px-2 py-1 text-xs font-semibold"
        :class="isHealthy ? 'bg-emerald-500/15 text-emerald-300' : 'bg-red-500/15 text-red-300'"
      >
        {{ endpoint.last_health_status || 'unknown' }}
      </span>
    </div>
    <dl class="mt-4 grid grid-cols-2 gap-3 text-sm">
      <div>
        <dt class="text-zinc-500">Latency</dt>
        <dd class="mt-1 font-medium text-zinc-100">{{ formatDuration(endpoint.last_health_latency_ms) }}</dd>
      </div>
      <div>
        <dt class="text-zinc-500">Status</dt>
        <dd class="mt-1 font-medium text-zinc-100">{{ endpoint.last_health_status_code || 'n/a' }}</dd>
      </div>
      <div class="col-span-2">
        <dt class="text-zinc-500">Last checked</dt>
        <dd class="mt-1 font-medium text-zinc-100">{{ formatDateTime(endpoint.last_health_checked_at) }}</dd>
      </div>
    </dl>
    <p v-if="endpoint.last_health_error" class="mt-3 rounded-md bg-red-950/50 p-2 text-xs text-red-200">
      {{ endpoint.last_health_error }}
    </p>
  </section>
</template>

<script setup lang="ts">
import type { EndpointHealth } from '~/types/admin'

const props = defineProps<{
  endpoint: EndpointHealth
}>()

const isHealthy = computed(() => props.endpoint.last_health_status === 'healthy')
</script>
