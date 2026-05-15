<template>
  <div>
    <PageHeader
      eyebrow="Telemetry"
      title="Usage"
      description="Request volume, token counts, and recent gateway traffic."
    />

    <section class="panel p-4">
      <div class="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <h3 class="text-lg font-semibold text-zinc-50">Daily traffic</h3>
        <button class="button-secondary" @click="refreshAll">Refresh</button>
      </div>
      <div class="h-72">
        <UsageChart :rows="summaryRows" />
      </div>
    </section>

    <section class="mt-6 grid gap-4 lg:grid-cols-2">
      <div class="panel p-4">
        <h3 class="text-lg font-semibold text-zinc-50">By endpoint</h3>
        <div class="mt-4 space-y-3">
          <div v-for="row in endpointBreakdown" :key="row.label">
            <div class="mb-1 flex justify-between text-sm">
              <span class="font-medium text-zinc-200">{{ row.label }}</span>
              <span class="text-zinc-400">{{ formatNumber(row.requests) }} requests</span>
            </div>
            <div class="h-2 rounded-full bg-zinc-800">
              <div class="h-2 rounded-full bg-cyan-400" :style="{ width: row.percent + '%' }" />
            </div>
          </div>
          <p v-if="endpointBreakdown.length === 0" class="text-sm text-zinc-500">No recent requests.</p>
        </div>
      </div>

      <div class="panel p-4">
        <h3 class="text-lg font-semibold text-zinc-50">By key</h3>
        <div class="mt-4 space-y-3">
          <div v-for="row in keyBreakdown" :key="row.label">
            <div class="mb-1 flex justify-between text-sm">
              <span class="font-mono text-xs text-zinc-200">{{ row.label }}</span>
              <span class="text-zinc-400">{{ formatNumber(row.tokens) }} tokens</span>
            </div>
            <div class="h-2 rounded-full bg-zinc-800">
              <div class="h-2 rounded-full bg-emerald-400" :style="{ width: row.percent + '%' }" />
            </div>
          </div>
          <p v-if="keyBreakdown.length === 0" class="text-sm text-zinc-500">No token usage in the recent sample.</p>
        </div>
      </div>
    </section>

    <section class="panel mt-6 p-4">
      <h3 class="mb-4 text-lg font-semibold text-zinc-50">Recent requests</h3>
      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-zinc-800">
          <thead>
            <tr>
              <th class="table-heading">Time</th>
              <th class="table-heading">Key</th>
              <th class="table-heading">Endpoint</th>
              <th class="table-heading">Path</th>
              <th class="table-heading">Model</th>
              <th class="table-heading">Prompt</th>
              <th class="table-heading">Completion</th>
              <th class="table-heading">Latency</th>
              <th class="table-heading">Status</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-zinc-800">
            <tr v-for="request in recentRequests" :key="request.request_id">
              <td class="table-cell whitespace-nowrap text-zinc-400">{{ formatDateTime(request.created_at) }}</td>
              <td class="table-cell font-mono text-xs">{{ request.api_key_prefix || 'none' }}</td>
              <td class="table-cell font-mono text-xs">{{ request.endpoint_id }}</td>
              <td class="table-cell max-w-xs truncate font-mono text-xs">{{ request.path }}</td>
              <td class="table-cell">{{ request.model || 'n/a' }}</td>
              <td class="table-cell">{{ formatNumber(request.prompt_tokens) }}</td>
              <td class="table-cell">{{ formatNumber(request.completion_tokens) }}</td>
              <td class="table-cell">{{ formatDuration(request.latency_ms) }}</td>
              <td class="table-cell"><StatusBadge :status="request.status_code" /></td>
            </tr>
            <tr v-if="recentRequests.length === 0">
              <td class="table-cell text-zinc-500" colspan="9">No requests logged yet.</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { UsageRequest, UsageRequestsResponse, UsageSummaryResponse } from '~/types/admin'

interface BreakdownRow {
  label: string
  requests: number
  tokens: number
  percent: number
}

const from = daysAgo(13).toISOString()
const to = tomorrow().toISOString()

const { data: summary, refresh: refreshSummary } = await useFetch<UsageSummaryResponse>('/api/admin/usage/summary', {
  query: { from, to, group_by: 'day' },
})
const { data: requests, refresh: refreshRequests } = await useFetch<UsageRequestsResponse>('/api/admin/usage/requests', {
  query: { limit: 100 },
})

const summaryRows = computed(() => summary.value?.rows || [])
const recentRequests = computed(() => requests.value?.requests || [])

const endpointBreakdown = computed(() => {
  const rows = groupRequests(recentRequests.value, (request) => request.endpoint_id)
  const max = Math.max(...rows.map((row) => row.requests), 1)
  return rows.map((row) => ({ ...row, percent: Math.round((row.requests / max) * 100) }))
})

const keyBreakdown = computed(() => {
  const rows = groupRequests(recentRequests.value, (request) => request.api_key_prefix || 'unauthenticated')
    .filter((row) => row.tokens > 0)
  const max = Math.max(...rows.map((row) => row.tokens), 1)
  return rows.map((row) => ({ ...row, percent: Math.round((row.tokens / max) * 100) }))
})

async function refreshAll() {
  await Promise.all([refreshSummary(), refreshRequests()])
}

function groupRequests(requests: UsageRequest[], labelFor: (request: UsageRequest) => string): BreakdownRow[] {
  const groups = new Map<string, BreakdownRow>()
  for (const request of requests) {
    const label = labelFor(request)
    const existing = groups.get(label) || { label, requests: 0, tokens: 0, percent: 0 }
    existing.requests += 1
    existing.tokens += request.total_tokens || 0
    groups.set(label, existing)
  }
  return [...groups.values()].sort((a, b) => b.requests - a.requests)
}
</script>
