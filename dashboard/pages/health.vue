<template>
  <div>
    <PageHeader
      eyebrow="Backends"
      title="Endpoint health"
      description="Current reachability for the RTXWS tunnel targets."
    >
      <button class="button-primary" :disabled="checking" @click="runCheck">
        {{ checking ? 'Checking...' : 'Run check' }}
      </button>
    </PageHeader>

    <section class="grid gap-4 lg:grid-cols-2">
      <EndpointStatus
        v-for="endpoint in endpoints"
        :key="endpoint.id"
        :endpoint="endpoint"
      />
    </section>

    <section class="panel mt-6 p-4">
      <h3 class="mb-4 text-lg font-semibold text-zinc-50">Recent check results</h3>
      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-zinc-800">
          <thead>
            <tr>
              <th class="table-heading">Time</th>
              <th class="table-heading">Endpoint</th>
              <th class="table-heading">Status</th>
              <th class="table-heading">HTTP</th>
              <th class="table-heading">Latency</th>
              <th class="table-heading">Error</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-zinc-800">
            <tr v-for="result in checkResults" :key="`${result.endpoint_id}-${result.checked_at}-${result.id || ''}`">
              <td class="table-cell whitespace-nowrap text-zinc-400">{{ formatDateTime(result.checked_at) }}</td>
              <td class="table-cell font-mono text-xs">{{ result.endpoint_id }}</td>
              <td class="table-cell">
                <span
                  class="rounded-full px-2 py-1 text-xs font-semibold"
                  :class="result.status === 'healthy' ? 'bg-emerald-500/15 text-emerald-300' : 'bg-red-500/15 text-red-300'"
                >
                  {{ result.status }}
                </span>
              </td>
              <td class="table-cell">{{ result.status_code || 'n/a' }}</td>
              <td class="table-cell">{{ formatDuration(result.latency_ms) }}</td>
              <td class="table-cell max-w-md truncate text-red-200">{{ result.error || '' }}</td>
            </tr>
            <tr v-if="checkResults.length === 0">
              <td class="table-cell text-zinc-500" colspan="6">No health checks recorded yet.</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { HealthCheckLog, HealthCheckResponse, HealthResponse } from '~/types/admin'

const checking = ref(false)

const { data: health, refresh } = await useFetch<HealthResponse>('/api/admin/health')
const { data: checks, refresh: refreshChecks } = await useFetch<{ checks: HealthCheckLog[] }>('/api/admin/health/checks', {
  query: { limit: 50 },
})
const endpoints = computed(() => health.value?.endpoints || [])
const checkResults = computed(() => checks.value?.checks || [])

async function runCheck() {
  checking.value = true
  try {
    await $fetch<HealthCheckResponse>('/api/admin/health/check', { method: 'POST' })
    await refresh()
    await refreshChecks()
  } finally {
    checking.value = false
  }
}
</script>
