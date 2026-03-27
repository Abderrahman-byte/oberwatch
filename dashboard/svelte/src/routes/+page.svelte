<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchJSON } from '$lib/api';
  import { connectStream } from '$lib/sse';
  import { AlertItem, KPICard, LineChart } from '$lib/components';
  import type { Agent, AgentsResponse, Alert, CostBreakdown, CostsResponse, HealthResponse } from '$lib/types';
  import type { ChartDataset } from 'chart.js';

  type HourlyCostBreakdown = CostBreakdown & {
    hour?: string;
    timestamp?: string;
    bucket?: string;
    period?: string;
    time?: string;
  };

  let loading = $state(true);
  let errorMessage = $state<string | null>(null);
  let totalSpendToday = $state(0);
  let activeAgents = $state(0);
  let alertsToday = $state(0);
  let uptimeSeconds = $state(0);
  let labels = $state<string[]>([]);
  let values = $state<number[]>([]);

  const mockAlerts: Alert[] = [
    {
      id: 'mock-1',
      type: 'budget_threshold',
      agent: 'email-agent',
      message: 'Email agent crossed 80% budget threshold',
      severity: 'warning',
      timestamp: new Date(Date.now() - 4 * 60 * 1000).toISOString()
    },
    {
      id: 'mock-2',
      type: 'runaway_detected',
      agent: 'qa-agent',
      message: 'Runaway pattern detected and request rate throttled',
      severity: 'error',
      timestamp: new Date(Date.now() - 27 * 60 * 1000).toISOString()
    },
    {
      id: 'mock-3',
      type: 'budget_exceeded',
      agent: 'research-agent',
      message: 'Research agent exceeded daily budget and was downgraded',
      severity: 'warning',
      timestamp: new Date(Date.now() - 55 * 60 * 1000).toISOString()
    }
  ];

  const lineDatasets = $derived<ChartDataset<'line', number[]>[]>([
    {
      label: 'Cost (USD)',
      data: values,
      borderColor: '#3B82F6',
      backgroundColor: '#3B82F6'
    }
  ]);

  function formatUSD(amount: number): string {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      maximumFractionDigits: 2
    }).format(amount);
  }

  function formatUptime(seconds: number): string {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${mins}m`;
    return `${mins}m`;
  }

  function toHourLabel(item: HourlyCostBreakdown, index: number): string {
    const candidate =
      item.hour ?? item.timestamp ?? item.bucket ?? item.period ?? item.time ?? `hour-${index + 1}`;
    const parsed = new Date(candidate);
    if (Number.isNaN(parsed.getTime())) {
      return `H${index + 1}`;
    }
    return parsed.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  async function loadOverview(): Promise<void> {
    loading = true;
    errorMessage = null;

    try {
      const from = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString();
      const [costs, agentsRes, health] = await Promise.all([
        fetchJSON<CostsResponse>(`/costs?group_by=hour&from=${encodeURIComponent(from)}`),
        fetchJSON<AgentsResponse>('/agents'),
        fetchJSON<HealthResponse>('/health')
      ]);

      const hourly = costs.breakdown as HourlyCostBreakdown[];
      labels = hourly.map((point, index) => toHourLabel(point, index));
      values = hourly.map((point) => point.cost_usd);

      totalSpendToday = costs.total_usd;
      activeAgents = agentsRes.agents.filter((agent: Agent) => agent.status === 'active').length;
      alertsToday = mockAlerts.length;
      uptimeSeconds = health.uptime_seconds;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load overview data.';
      errorMessage = message;
    } finally {
      loading = false;
    }
  }

  async function emergencyStop(): Promise<void> {
    if (!confirm('Emergency stop will disable all agents. Continue?')) {
      return;
    }

    try {
      await fetchJSON('/kill-all', { method: 'POST' });
      await loadOverview();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Emergency stop failed.';
      errorMessage = message;
    }
  }

  onMount(() => {
    void loadOverview();

    const stream = connectStream((eventName) => {
      if (eventName === 'cost_update') {
        void loadOverview();
      }
    });

    return () => {
      stream.close();
    };
  });
</script>

<section class="space-y-6">
  <header class="space-y-1">
    <h1 class="text-2xl font-semibold text-text-primary">Overview</h1>
    <p class="text-sm text-text-secondary">Live spend, alerts, and system health.</p>
  </header>

  {#if errorMessage}
    <div class="rounded-lg border border-danger/40 bg-danger/10 p-4">
      <p class="text-sm text-danger">{errorMessage}</p>
      <button
        type="button"
        class="mt-3 rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-white hover:bg-accent-hover"
        onclick={() => loadOverview()}
      >
        Retry
      </button>
    </div>
  {/if}

  {#if loading}
    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      {#each Array.from({ length: 4 }) as _, index (index)}
        <div class="h-28 animate-pulse rounded-lg border border-border-default bg-surface"></div>
      {/each}
    </div>
  {:else}
    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      <KPICard title="Total Spend Today" value={formatUSD(totalSpendToday)} subtitle="Last 24 hours" />
      <KPICard title="Active Agents" value={activeAgents} subtitle="Currently serving traffic" />
      <div class={alertsToday > 0 ? 'rounded-lg ring-1 ring-warning/60' : ''}>
        <KPICard
          title="Alerts Today"
          value={alertsToday}
          subtitle="Recent alert events"
          trend={alertsToday > 0 ? 'down' : 'up'}
          trendLabel={alertsToday > 0 ? 'Needs attention' : 'All clear'}
        />
      </div>
      <KPICard title="Uptime" value={formatUptime(uptimeSeconds)} subtitle="Proxy process uptime" />
    </div>
  {/if}

  <LineChart {labels} datasets={lineDatasets} height={320} />

  <section class="space-y-3 rounded-lg border border-border-default bg-surface p-4">
    <h2 class="text-lg font-semibold text-text-primary">Recent Alerts</h2>
    {#if mockAlerts.length === 0}
      <p class="text-sm text-text-muted">No alerts yet.</p>
    {:else}
      <div class="space-y-2">
        {#each mockAlerts.slice(0, 5) as alert (alert.id)}
          <AlertItem {alert} />
        {/each}
      </div>
    {/if}
  </section>

  <button
    type="button"
    class="w-full rounded-md bg-danger px-4 py-3 text-sm font-semibold text-white transition-colors hover:bg-red-600"
    onclick={emergencyStop}
  >
    Emergency Stop
  </button>
</section>
