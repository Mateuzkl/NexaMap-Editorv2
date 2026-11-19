<script lang="ts">
	import { tick } from "svelte";
	import { debugConsole, type LogFilter } from "../lib/debug/console.svelte";

	let logEl = $state<HTMLDivElement>();

	$effect(() => {
		const count = debugConsole.filtered.length;
		if (debugConsole.open && debugConsole.autoScroll && count >= 0) {
			void tick().then(() => {
				if (logEl) logEl.scrollTop = logEl.scrollHeight;
			});
		}
	});

	const FILTERS: { id: LogFilter; label: string }[] = [
		{ id: "all", label: "All" },
		{ id: "info", label: "Info" },
		{ id: "warn", label: "Warn" },
		{ id: "error", label: "Error" },
	];
</script>

{#if debugConsole.open}
	<section class="dbg-drawer" aria-label="Debug console">
		<header class="dbg-bar">
			<span class="dbg-title">Debug Console</span>

			{#if debugConsole.stats}
				<div class="dbg-metrics" title="Live Go runtime memory & goroutine telemetry">
					<span>Heap <b>{debugConsole.stats.heapAllocMb.toFixed(1)} MB</b></span>
					<span>Inuse <b>{debugConsole.stats.heapInuseMb.toFixed(1)} MB</b></span>
					<span>Sys <b>{debugConsole.stats.sysMb.toFixed(1)} MB</b></span>
					<span>Objs <b>{debugConsole.stats.heapObjects.toLocaleString()}</b></span>
					<span>Goroutines <b>{debugConsole.stats.goroutines}</b></span>
					<span>GC <b>{debugConsole.stats.numGc}</b></span>
				</div>
			{/if}

			<div class="dbg-grow"></div>

			<div class="dbg-filters" role="group" aria-label="Log level filter">
				{#each FILTERS as f (f.id)}
					<button
						type="button"
						class="dbg-pill"
						class:active={debugConsole.filter === f.id}
						onclick={() => (debugConsole.filter = f.id)}>{f.label}</button
					>
				{/each}
			</div>

			<input class="dbg-search" type="search" placeholder="Filter logs…" bind:value={debugConsole.query} />

			<button
				type="button"
				class="dbg-btn"
				title="Run Go garbage collector and release unused memory back to OS"
				onclick={() => void debugConsole.forceGC()}>Force GC</button
			>
			<button
				type="button"
				class="dbg-btn"
				class:active={debugConsole.stats?.nativeConsole}
				title="Open or close a native Windows terminal console window"
				onclick={() => void debugConsole.toggleNativeConsole()}
			>
				{debugConsole.stats?.nativeConsole ? "Close Win Terminal" : "Win Terminal"}
			</button>
			<button type="button" class="dbg-btn" onclick={() => void debugConsole.copyAll()}>Copy</button>
			<button type="button" class="dbg-btn" onclick={() => void debugConsole.clear()}>Clear</button>
			<button
				type="button"
				class="dbg-btn"
				title="Close Debug Console (F12)"
				onclick={() => debugConsole.setOpen(false)}>×</button
			>
		</header>

		<div class="dbg-body" bind:this={logEl}>
			{#if debugConsole.filtered.length === 0}
				<div class="dbg-empty">No log entries matching the current filter.</div>
			{:else}
				{#each debugConsole.filtered as entry (entry.id)}
					<div class="dbg-line dbg-{entry.level}">
						<span class="dbg-time">{entry.time}</span>
						<span class="dbg-lvl">{entry.level.toUpperCase()}</span>
						<span class="dbg-src">[{entry.source}]</span>
						<span class="dbg-msg">{entry.message}</span>
					</div>
				{/each}
			{/if}
		</div>
	</section>
{/if}

<style>
	.dbg-drawer {
		flex: none;
		height: 230px;
		display: flex;
		flex-direction: column;
		background: var(--ui-bg, #161616);
		border-top: 1px solid var(--ui-border, #444);
		color: var(--ui-text, #dcdcdc);
		z-index: 30;
	}
	.dbg-bar {
		flex: none;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		padding: 6px 10px;
		background: var(--ui-sidebar, #222);
		border-bottom: 1px solid var(--ui-border, #333);
		font-size: 11.5px;
	}
	.dbg-title {
		font-weight: 700;
		color: var(--ui-text, #fff);
		letter-spacing: 0.02em;
	}
	.dbg-metrics {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		color: var(--ui-muted, #9aa);
		font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
		font-size: 11px;
	}
	.dbg-metrics b {
		color: var(--ui-link, #6aa6e8);
		font-weight: 600;
	}
	.dbg-grow {
		flex: 1;
	}
	.dbg-filters {
		display: inline-flex;
		gap: 2px;
		padding: 2px;
		border-radius: 6px;
		background: var(--ui-bg, #141414);
		border: 1px solid var(--ui-border, #333);
	}
	.dbg-pill {
		border: 0;
		background: transparent;
		color: var(--ui-muted, #aaa);
		font: inherit;
		font-size: 11px;
		padding: 2px 7px;
		border-radius: 4px;
		cursor: pointer;
	}
	.dbg-pill.active {
		background: var(--ui-primary, #5566aa);
		color: #fff;
		font-weight: 600;
	}
	.dbg-search {
		height: 24px;
		width: 140px;
		padding: 0 8px;
		border-radius: 6px;
		border: 1px solid var(--ui-border, #444);
		background: var(--ui-bg, #111);
		color: var(--ui-text, #eee);
		font: inherit;
		font-size: 11.5px;
	}
	.dbg-btn {
		height: 24px;
		padding: 0 8px;
		border-radius: 6px;
		border: 1px solid var(--ui-border, #444);
		background: var(--ui-surface, #2a2a2a);
		color: var(--ui-text, #ddd);
		font: inherit;
		font-size: 11px;
		font-weight: 600;
		cursor: pointer;
	}
	.dbg-btn:hover {
		border-color: var(--ui-primary, #7c4df2);
	}
	.dbg-btn.active {
		background: var(--ui-primary, #7c4df2);
		border-color: var(--ui-primary, #7c4df2);
		color: #fff;
	}
	.dbg-body {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 6px 10px;
		font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
		font-size: 11.5px;
		line-height: 1.5;
		user-select: text;
	}
	.dbg-empty {
		color: var(--ui-muted, #888);
		padding: 8px 0;
	}
	.dbg-line {
		display: flex;
		gap: 8px;
		align-items: baseline;
		word-break: break-word;
	}
	.dbg-time {
		color: var(--ui-muted, #778);
		flex: none;
	}
	.dbg-lvl {
		width: 40px;
		flex: none;
		font-weight: 700;
	}
	.dbg-info .dbg-lvl {
		color: var(--ui-link, #3ad0de);
	}
	.dbg-warn .dbg-lvl,
	.dbg-warn .dbg-msg {
		color: var(--ui-warning, #efc46c);
	}
	.dbg-error .dbg-lvl,
	.dbg-error .dbg-msg {
		color: var(--ui-danger, #ef7180);
	}
	.dbg-src {
		color: var(--ui-muted, #889);
		flex: none;
	}
	.dbg-msg {
		flex: 1;
		min-width: 0;
		color: var(--ui-text, #e0e0e0);
	}
</style>
