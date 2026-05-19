<template>
  <div>
    <PageHeader
      eyebrow="Security"
      title="IP bans"
      description="Scanner controls, active bans, and recent rejected requests."
    />

    <section class="grid gap-4 md:grid-cols-3">
      <MetricCard label="Active bans" :value="formatNumber(activeBans.length)" />
      <MetricCard label="Recent events" :value="formatNumber(recentEvents.length)" />
      <MetricCard label="Top rejected IP" :value="topRejectedIp?.label || 'n/a'" :hint="topRejectedIp ? `${topRejectedIp.count} events` : undefined" />
    </section>

    <section class="panel mt-6 p-4">
      <h3 class="text-lg font-semibold text-zinc-50">Manual ban</h3>
      <form class="mt-4 grid gap-3 md:grid-cols-[1fr_1fr_1fr_auto]" @submit.prevent="createBan">
        <input v-model="newBan.client_ip" class="field" placeholder="Client IP" />
        <input v-model="newBan.reason" class="field" placeholder="Reason" />
        <select v-model.number="newBan.duration_seconds" class="field">
          <option :value="300">5 minutes</option>
          <option :value="1800">30 minutes</option>
          <option :value="7200">2 hours</option>
          <option :value="86400">24 hours</option>
        </select>
        <button class="button-primary" type="submit" :disabled="creating">Ban</button>
      </form>
      <p v-if="errorMessage" class="mt-3 text-sm text-red-300">{{ errorMessage }}</p>
    </section>

    <section class="panel mt-6 p-4">
      <div class="mb-4 flex items-center justify-between">
        <h3 class="text-lg font-semibold text-zinc-50">Active bans</h3>
        <button class="button-secondary" @click="refreshAll">Refresh</button>
      </div>
      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-zinc-800">
          <thead>
            <tr>
              <th class="table-heading">IP</th>
              <th class="table-heading">Reason</th>
              <th class="table-heading">Strikes</th>
              <th class="table-heading">Until</th>
              <th class="table-heading">Type</th>
              <th class="table-heading">Action</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-zinc-800">
            <tr v-for="ban in activeBans" :key="ban.id">
              <td class="table-cell font-mono text-xs">{{ ban.client_ip }}</td>
              <td class="table-cell">{{ ban.reason }}</td>
              <td class="table-cell">{{ ban.strikes }}</td>
              <td class="table-cell whitespace-nowrap text-zinc-400">{{ formatDateTime(ban.banned_until) }}</td>
              <td class="table-cell">{{ ban.manual ? 'manual' : 'auto' }}</td>
              <td class="table-cell">
                <button class="button-secondary px-2 py-1 text-xs" @click="liftBan(ban.id)">Lift</button>
              </td>
            </tr>
            <tr v-if="activeBans.length === 0">
              <td class="table-cell text-zinc-500" colspan="6">No active bans.</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="mt-6 grid gap-4 lg:grid-cols-[minmax(0,1fr)_22rem]">
      <div class="panel p-4">
        <h3 class="mb-4 text-lg font-semibold text-zinc-50">Recent security events</h3>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-zinc-800">
            <thead>
              <tr>
                <th class="table-heading">Time</th>
                <th class="table-heading">IP</th>
                <th class="table-heading">Type</th>
                <th class="table-heading">Path</th>
                <th class="table-heading">Detail</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-zinc-800">
              <tr v-for="event in recentEvents" :key="event.id">
                <td class="table-cell whitespace-nowrap text-zinc-400">{{ formatDateTime(event.created_at) }}</td>
                <td class="table-cell font-mono text-xs">{{ event.client_ip }}</td>
                <td class="table-cell font-mono text-xs">{{ event.event_type }}</td>
                <td class="table-cell max-w-xs truncate font-mono text-xs">{{ event.path || 'n/a' }}</td>
                <td class="table-cell max-w-sm truncate text-zinc-300">{{ event.detail || 'n/a' }}</td>
              </tr>
              <tr v-if="recentEvents.length === 0">
                <td class="table-cell text-zinc-500" colspan="5">No security events.</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="panel p-4">
        <h3 class="text-lg font-semibold text-zinc-50">Top rejected IPs</h3>
        <div class="mt-4 space-y-3">
          <div v-for="row in topRejectedIps" :key="row.label">
            <div class="mb-1 flex justify-between text-sm">
              <span class="font-mono text-xs text-zinc-200">{{ row.label }}</span>
              <span class="text-zinc-400">{{ row.count }}</span>
            </div>
            <div class="h-2 rounded-full bg-zinc-800">
              <div class="h-2 rounded-full bg-red-400" :style="{ width: row.percent + '%' }" />
            </div>
          </div>
          <p v-if="topRejectedIps.length === 0" class="text-sm text-zinc-500">No rejected IPs in the recent sample.</p>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { SecurityBansResponse, SecurityEventsResponse } from '~/types/admin'

interface TopIPRow {
  label: string
  count: number
  percent: number
}

const { data: bans, refresh: refreshBans } = await useFetch<SecurityBansResponse>('/api/admin/security/bans')
const { data: events, refresh: refreshEvents } = await useFetch<SecurityEventsResponse>('/api/admin/security/events', {
  query: { limit: 500 },
})

const creating = ref(false)
const errorMessage = ref('')
const newBan = reactive({
  client_ip: '',
  reason: 'manual ban',
  duration_seconds: 1800,
})

const activeBans = computed(() => bans.value?.bans || [])
const recentEvents = computed(() => events.value?.events || [])

const topRejectedIps = computed<TopIPRow[]>(() => {
  const counts = new Map<string, number>()
  for (const event of recentEvents.value) {
    counts.set(event.client_ip, (counts.get(event.client_ip) || 0) + 1)
  }
  const max = Math.max(...counts.values(), 1)
  return [...counts.entries()]
    .map(([label, count]) => ({ label, count, percent: Math.round((count / max) * 100) }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 8)
})

const topRejectedIp = computed(() => topRejectedIps.value[0])

async function refreshAll() {
  await Promise.all([refreshBans(), refreshEvents()])
}

async function createBan() {
  creating.value = true
  errorMessage.value = ''
  try {
    await $fetch('/api/admin/security/bans', {
      method: 'POST',
      body: { ...newBan },
    })
    newBan.client_ip = ''
    await refreshAll()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Failed to create ban'
  } finally {
    creating.value = false
  }
}

async function liftBan(id: number) {
  await $fetch(`/api/admin/security/bans/${encodeURIComponent(id)}/lift`, { method: 'POST' })
  await refreshAll()
}
</script>
