<script lang="ts">
	import { ResourceState } from "../../lib/api";
	import { workspace } from "./workspace.svelte";

	const server = $derived(workspace.inspection?.server ?? null);

	const status = $derived.by(() => {
		if (workspace.scanning) return { label: "Scanning…", kind: "info" };
		if (!workspace.serverRoot) return { label: "Optional", kind: "neutral" };
		if (server?.state === ResourceState.ResourceMissing) return { label: "Missing", kind: "error" };
		if (server?.state === ResourceState.ResourceInvalid) return { label: "Error", kind: "error" };
		if (server?.state === ResourceState.ResourceFound) {
			const hasOtb = server.itemsOtb.state === ResourceState.ResourceFound;
			const hasXml = server.itemsXml.state === ResourceState.ResourceFound;
			if (hasOtb && hasXml) return { label: "Ready", kind: "ok" };
			return { label: "Partial", kind: "warn" };
		}
		return { label: "Optional", kind: "neutral" };
	});
</script>

<section class="ws-card" aria-label="Server configuration">
	<header class="ws-card-head">
		<div class="ws-card-title-group">
			<span class="ws-step">02</span>
			<div>
				<h2 class="ws-card-title">SERVER</h2>
				<p class="ws-card-sub">Auto-detect `items.otb`, `items.xml`, maps &amp; creatures</p>
			</div>
		</div>
		<span class="ws-status ws-status-{status.kind}">{status.label}</span>
	</header>

	<div class="ws-card-actions">
		<button type="button" class="ws-btn ws-btn-secondary" onclick={() => workspace.pickServerFolder()}>
			{workspace.serverRoot ? "Change Server Folder" : "Select Server Folder"}
		</button>
		{#if workspace.serverRoot}
			<button type="button" class="ws-btn ws-btn-ghost" onclick={() => workspace.clearServer()}>Clear</button>
		{/if}
	</div>

	{#if workspace.serverRoot}
		<div class="ws-path-box" title={workspace.serverRoot}>
			<span class="ws-path-label">Root</span>
			<span class="ws-path-value">{workspace.serverRoot}</span>
		</div>
	{/if}

	{#if server?.error}
		<div class="ws-alert ws-alert-warn">{server.error}</div>
	{:else if server && server.state === ResourceState.ResourceFound}
		<div class="ws-meta-grid">
			<div class="ws-meta-item" title={server.profileNote}>
				<span class="ws-meta-k">Detected Profile</span>
				<span class="ws-meta-v"><strong>{server.profile}</strong></span>
			</div>
			<div class="ws-meta-item">
				<span class="ws-meta-k">Detected Maps</span>
				<span class="ws-meta-v">{server.maps?.length ?? 0}</span>
			</div>
			{#if server.serverDir}
				<div class="ws-meta-item full" title={server.serverDir}>
					<span class="ws-meta-k">Data Directory</span>
					<span class="ws-meta-v mono">{server.serverDir}</span>
				</div>
			{/if}
		</div>
	{:else}
		<p class="ws-empty-hint">
			Optional for Object Editor and ClientID maps. Select your server folder to automatically discover
			`items.otb`, `items.xml`, `.otbm`/`.otmap` maps, monsters, and NPCs.
		</p>
	{/if}
</section>

<style>
	.ws-card {
		background: var(--ui-surface);
		border: 1px solid var(--ui-border);
		border-radius: var(--ui-radius);
		padding: 18px;
		display: flex;
		flex-direction: column;
		gap: 12px;
		min-width: 0;
	}
	.ws-card-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 10px;
	}
	.ws-card-title-group {
		display: flex;
		align-items: center;
		gap: 10px;
		min-width: 0;
	}
	.ws-step {
		width: 28px;
		height: 28px;
		border-radius: 8px;
		background: color-mix(in srgb, var(--ui-link) 18%, transparent);
		color: var(--ui-link);
		font-size: 12px;
		font-weight: 700;
		display: grid;
		place-items: center;
		flex: none;
	}
	.ws-card-title {
		margin: 0;
		font-size: 13px;
		font-weight: 700;
		letter-spacing: 0.06em;
		color: var(--ui-text);
	}
	.ws-card-sub {
		margin: 2px 0 0;
		font-size: 12px;
		color: var(--ui-muted);
	}
	.ws-card-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}
	.ws-path-box {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 7px 10px;
		border-radius: 8px;
		background: var(--ui-bg);
		border: 1px solid var(--ui-border);
		min-width: 0;
	}
	.ws-path-label {
		font-size: 11px;
		font-weight: 600;
		color: var(--ui-muted);
		flex: none;
	}
	.ws-path-value {
		font-size: 12px;
		color: var(--ui-text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}
	.ws-meta-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 8px 12px;
		padding-top: 2px;
	}
	.ws-meta-item {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	.ws-meta-item.full {
		grid-column: 1 / -1;
	}
	.ws-meta-k {
		font-size: 11px;
		color: var(--ui-muted);
	}
	.ws-meta-v {
		font-size: 12.5px;
		color: var(--ui-text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ws-meta-v.mono {
		font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
		font-size: 11.5px;
		color: var(--ui-muted);
	}
	.ws-empty-hint {
		margin: 0;
		font-size: 12px;
		color: var(--ui-muted);
		line-height: 1.45;
	}
</style>
