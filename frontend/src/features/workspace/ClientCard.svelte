<script lang="ts">
	import { Format, ResourceState } from "../../lib/api";
	import { commands } from "../../lib/commands";
	import { workspace } from "./workspace.svelte";

	const client = $derived(workspace.inspection?.client ?? null);
	const files = $derived(workspace.clientFiles);
	const isAssets = $derived(files?.format === Format.FormatAssets);

	const status = $derived.by(() => {
		if (workspace.scanning) return { label: "Scanning…", kind: "info" };
		if (!workspace.clientRoot) return { label: "Not selected", kind: "neutral" };
		if (client?.ambiguous) return { label: "Choose file", kind: "warn" };
		if (client?.state === ResourceState.ResourceMissing) return { label: "Missing", kind: "error" };
		if (client?.state === ResourceState.ResourceInvalid) return { label: "Invalid", kind: "error" };
		if (files && !files.detected && !isAssets) return { label: "Custom version", kind: "warn" };
		if (files) return { label: "Ready", kind: "ok" };
		return { label: "Not selected", kind: "neutral" };
	});

	const formatSummary = $derived.by(() => {
		if (!files) return "";
		if (isAssets) return "Tibia 12+ assets (catalog-content.json)";
		return files.hasOtfi ? "Tibia.dat / Tibia.spr · features from .otfi" : "Tibia.dat / Tibia.spr";
	});
</script>

<section class="ws-card" aria-label="Client configuration">
	<header class="ws-card-head">
		<div class="ws-card-title-group">
			<span class="ws-step">01</span>
			<div>
				<h2 class="ws-card-title">CLIENT</h2>
				<p class="ws-card-sub">Select `.dat/.spr` folder or Tibia 12+ `assets`</p>
			</div>
		</div>
		<span class="ws-status ws-status-{status.kind}">{status.label}</span>
	</header>

	<div class="ws-card-actions">
		<button type="button" class="ws-btn ws-btn-primary" onclick={() => workspace.pickClientFolder()}>
			{workspace.clientRoot ? "Change Client Folder" : "Select Client Folder"}
		</button>
		{#if workspace.clientRoot}
			<button type="button" class="ws-btn ws-btn-ghost" onclick={() => commands.open(workspace.clientRoot)}>
				Configure…
			</button>
			<button type="button" class="ws-btn ws-btn-ghost" onclick={() => workspace.clearClient()}>Clear</button>
		{/if}
	</div>

	{#if workspace.clientRoot}
		<div class="ws-path-box" title={workspace.clientRoot}>
			<span class="ws-path-label">Folder</span>
			<span class="ws-path-value">{workspace.clientRoot}</span>
		</div>
	{/if}

	{#if client?.ambiguous && client.candidates?.length}
		<div class="ws-alert ws-alert-warn">
			<strong>Multiple clients found.</strong> Pick which client file to inspect:
			<div class="ws-candidates">
				{#each client.candidates as candidate (candidate.path)}
					<button
						type="button"
						class="ws-candidate"
						title={candidate.path}
						onclick={() => workspace.selectClientCandidate(candidate.path)}
					>
						<span class="ws-tag">{candidate.kind.toUpperCase()}</span>
						<span class="ws-candidate-label">{candidate.label}</span>
					</button>
				{/each}
			</div>
		</div>
	{:else if client?.error}
		<div class="ws-alert ws-alert-error">
			<span>{client.error}</span>
			<button type="button" class="ws-link-btn" onclick={() => commands.open(workspace.clientRoot)}>
				Open manual selector…
			</button>
		</div>
	{:else if files}
		<div class="ws-meta-grid">
			<div class="ws-meta-item">
				<span class="ws-meta-k">Detected Version</span>
				<span class="ws-meta-v">
					{#if files.detected}
						<strong>{files.detected.name}</strong>
					{:else}
						<span class="ws-warn-text">Unknown — choose version</span>
					{/if}
				</span>
			</div>
			<div class="ws-meta-item">
				<span class="ws-meta-k">Format</span>
				<span class="ws-meta-v">{formatSummary}</span>
			</div>
			{#if !isAssets}
				<div class="ws-meta-item full" title={files.datPath}>
					<span class="ws-meta-k">Files</span>
					<span class="ws-meta-v mono">{files.datPath}</span>
				</div>
			{/if}
		</div>
		{#if !files.detected && !isAssets}
			<div class="ws-alert ws-alert-warn">
				Signatures were not recognized automatically. Click <strong>Open Editor</strong> or
				<strong>Configure…</strong> to pick the version and features manually.
			</div>
		{/if}
	{:else}
		<p class="ws-empty-hint">
			Required to open Object Editor or Map Editor. Supports classic `Tibia.dat`/`Tibia.spr`, OTClient `.otfi`,
			and Tibia 12+ `assets/`.
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
		background: color-mix(in srgb, var(--ui-primary) 18%, transparent);
		color: var(--ui-primary);
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
	.ws-warn-text {
		color: var(--ui-warning);
		font-weight: 600;
	}
	.ws-candidates {
		display: flex;
		flex-direction: column;
		gap: 6px;
		margin-top: 8px;
	}
	.ws-candidate {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 7px 10px;
		border-radius: 8px;
		border: 1px solid var(--ui-border);
		background: var(--ui-surface-raised);
		color: var(--ui-text);
		cursor: pointer;
		font: inherit;
		font-size: 12.5px;
		text-align: left;
	}
	.ws-candidate:hover {
		border-color: var(--ui-primary);
	}
	.ws-tag {
		font-size: 10px;
		font-weight: 700;
		padding: 2px 6px;
		border-radius: 5px;
		background: color-mix(in srgb, var(--ui-link) 18%, transparent);
		color: var(--ui-link);
	}
	.ws-candidate-label {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ws-empty-hint {
		margin: 0;
		font-size: 12px;
		color: var(--ui-muted);
		line-height: 1.45;
	}
</style>
