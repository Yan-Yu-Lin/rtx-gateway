<template>
  <div>
    <PageHeader
      eyebrow="Overview"
      title="Gateway dashboard"
      description="Live usage, token volume, and RTXWS endpoint health."
    />

    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <MetricCard label="Requests today" :value="formatNumber(totals.requests)" />
      <MetricCard label="Tokens today" :value="formatNumber(totals.totalTokens)" />
      <MetricCard label="Error rate" :value="formatPercent(totals.errorRate)" />
      <MetricCard label="Median latency sample" :value="formatDuration(medianLatency)" hint="Recent 100 requests" />
    </div>

    <section class="mt-6 grid gap-4 lg:grid-cols-2">
      <EndpointStatus
        v-for="endpoint in health?.endpoints || []"
        :key="endpoint.id"
        :endpoint="endpoint"
      />
    </section>

    <section class="panel mt-6 p-4">
      <div class="mb-4 flex items-center justify-between">
        <h3 class="text-lg font-semibold text-zinc-50">Recent requests</h3>
        <NuxtLink to="/usage" class="text-sm font-medium text-cyan-300 hover:text-cyan-200">View usage</NuxtLink>
      </div>
      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-zinc-800">
          <thead>
            <tr>
              <th class="table-heading">Time</th>
              <th class="table-heading">Endpoint</th>
              <th class="table-heading">Model</th>
              <th class="table-heading">Tokens</th>
              <th class="table-heading">Latency</th>
              <th class="table-heading">Status</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-zinc-800">
            <tr v-for="request in recentRequests" :key="request.request_id">
              <td class="table-cell whitespace-nowrap text-zinc-400">{{ formatDateTime(request.created_at) }}</td>
              <td class="table-cell font-mono text-xs">{{ request.endpoint_id }}</td>
              <td class="table-cell">{{ request.model || 'n/a' }}</td>
              <td class="table-cell">{{ formatNumber(request.total_tokens) }}</td>
              <td class="table-cell">{{ formatDuration(request.latency_ms) }}</td>
              <td class="table-cell"><StatusBadge :status="request.status_code" /></td>
            </tr>
            <tr v-if="recentRequests.length === 0">
              <td class="table-cell text-zinc-500" colspan="6">No requests logged yet.</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { HealthResponse, UsageRequestsResponse, UsageSummaryResponse } from '~/types/admin'

const todayFrom = startOfToday().toISOString()
const todayTo = tomorrow().toISOString()

const [{ data: summary }, { data: requests }, { data: health }] = await Promise.all([
  useFetch<UsageSummaryResponse>('/api/admin/usage/summary', {
    query: { from: todayFrom, to: todayTo, group_by: 'day' },
  }),
  useFetch<UsageRequestsResponse>('/api/admin/usage/requests', {
    query: { limit: 100 },
  }),
  useFetch<HealthResponse>('/api/admin/health'),
])

const totals = computed(() => {
  const row = summary.value?.rows[0]
  const requestsToday = row?.requests || 0
  return {
    requests: requestsToday,
    totalTokens: row?.total_tokens || 0,
    errorRate: requestsToday > 0 ? (row?.errors || 0) / requestsToday : 0,
  }
})

const recentRequests = computed(() => requests.value?.requests || [])
const medianLatency = computed(() => {
  const latencies = recentRequests.value.map((request) => request.latency_ms).sort((a, b) => a - b)
  if (latencies.length === 0) return undefined
  return latencies[Math.floor(latencies.length / 2)]
})
</script>
