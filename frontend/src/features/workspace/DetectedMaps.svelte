<script lang="ts">
	import { formatSize } from "../../lib/util/format";
	import { workspace } from "./workspace.svelte";

	const maps = $derived(workspace.inspection?.server.maps ?? []);
</script>

<section class="ws-section" aria-label="Detected maps">
	<header class="ws-section-head">
		<h3 class="ws-section-title">DETECTED MAPS ({maps.length})</h3>
		{#if !workspace.canOpenEditor && maps.length > 0}
			<span class="ws-hint">Select a valid client to open maps</span>
		{/if}
	</header>

	{#if maps.length === 0}
		<p class="ws-empty">
			{workspace.serverRoot
				? "No .otbm or .otmap files detected in this server folder."
				: "Select a server folder to list its maps here, or use Open Map in the sidebar."}
		</p>
	{:else}
		<div class="ws-map-list">
			{#each maps as entry (entry.path)}
				<div class="ws-map-item" title={entry.path}>
					<span class="ws-fmt ws-fmt-{entry.format}">{entry.format.toUpperCase()}</span>
					<div class="ws-map-info">
						<span class="ws-map-name">{entry.name}</span>
						<span class="ws-map-meta">
							{entry.relDir || "."} · {formatSize(entry.size)}
						</span>
					</div>
					<button
						type="button"
						class="ws-btn ws-btn-secondary ws-btn-sm"
						disabled={!workspace.canOpenEditor}
						onclick={() => workspace.openEditor("map", entry.path)}
					>
						Open
					</button>
				</div>
			{/each}
		</div>
	{/if}
</section>

<style>
	.ws-section {
		background: var(--ui-surface);
		border: 1px solid var(--ui-border);
		border-radius: var(--ui-radius);
		padding: 16px 18px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.ws-section-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
	}
	.ws-section-title {
		margin: 0;
		font-size: 12px;
		font-weight: 700;
		letter-spacing: 0.08em;
		color: var(--ui-muted);
	}
	.ws-hint {
		font-size: 11.5px;
		color: var(--ui-warning);
	}
	.ws-empty {
		margin: 0;
		font-size: 12.5px;
		color: var(--ui-muted);
	}
	.ws-map-list {
		display: flex;
		flex-direction: column;
		gap: 6px;
		max-height: 240px;
		overflow-y: auto;
	}
	.ws-map-item {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 12px;
		border-radius: 9px;
		background: var(--ui-bg);
		border: 1px solid var(--ui-border);
		min-width: 0;
	}
	.ws-fmt {
		font-size: 10px;
		font-weight: 700;
		padding: 3px 7px;
		border-radius: 6px;
		flex: none;
	}
	.ws-fmt-otbm {
		background: color-mix(in srgb, var(--ui-primary) 18%, transparent);
		color: var(--ui-primary);
	}
	.ws-fmt-otmap {
		background: color-mix(in srgb, var(--ui-link) 18%, transparent);
		color: var(--ui-link);
	}
	.ws-map-info {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.ws-map-name {
		font-size: 13px;
		font-weight: 600;
		color: var(--ui-text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ws-map-meta {
		font-size: 11.5px;
		color: var(--ui-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
