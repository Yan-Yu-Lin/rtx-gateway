<template>
  <ClientOnly>
    <Line :data="chartData" :options="chartOptions" />
    <template #fallback>
      <div class="h-72 animate-pulse rounded-md bg-zinc-800/60" />
    </template>
  </ClientOnly>
</template>

<script setup lang="ts">
import {
  CategoryScale,
  Chart as ChartJS,
  Filler,
  Legend,
  LinearScale,
  LineElement,
  PointElement,
  Tooltip,
  type ChartData,
  type ChartOptions,
} from 'chart.js'
import { Line } from 'vue-chartjs'
import type { UsageSummaryRow } from '~/types/admin'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Filler, Tooltip, Legend)

const props = defineProps<{
  rows: UsageSummaryRow[]
}>()

const chartData = computed<ChartData<'line'>>(() => ({
  labels: props.rows.map((row) => row.bucket),
  datasets: [
    {
      label: 'Requests',
      data: props.rows.map((row) => row.requests),
      borderColor: '#22d3ee',
      backgroundColor: 'rgba(34, 211, 238, 0.12)',
      fill: true,
      tension: 0.3,
    },
    {
      label: 'Errors',
      data: props.rows.map((row) => row.errors),
      borderColor: '#f87171',
      backgroundColor: 'rgba(248, 113, 113, 0.08)',
      fill: true,
      tension: 0.3,
    },
  ],
}))

const chartOptions: ChartOptions<'line'> = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: {
      labels: { color: '#d4d4d8' },
    },
  },
  scales: {
    x: {
      grid: { color: 'rgba(63, 63, 70, 0.35)' },
      ticks: { color: '#a1a1aa' },
    },
    y: {
      beginAtZero: true,
      grid: { color: 'rgba(63, 63, 70, 0.35)' },
      ticks: { color: '#a1a1aa', precision: 0 },
    },
  },
}
</script>
