<script lang="ts">
	import { Format } from "../../lib/api";
	import { map } from "../../lib/map/state.svelte";
	import { app } from "../../lib/state/app.svelte";
	import { workspace, type ItemIdPreference } from "./workspace.svelte";

	const files = $derived(workspace.clientFiles);
	const server = $derived(workspace.inspection?.server ?? null);

	const ID_MODES: { value: ItemIdPreference; label: string; hint: string }[] = [
		{
			value: "auto",
			label: "Auto (recommended)",
			hint: "Delegates to the per-map ID planner (honors each map's native format)",
		},
		{ value: "client", label: "ClientID", hint: "Use client object IDs directly without items.otb" },
		{ value: "server", label: "ServerID", hint: "Translate server IDs using the validated items.otb" },
	];
</script>

<section class="ws-overview" aria-label="Workspace summary and launch">
	<div class="ws-overview-top">
		<div>
			<h3 class="ws-section-title">WORKSPACE SUMMARY</h3>
			<p class="ws-overview-line">
				<span>
					<strong>Client:</strong>
					{files?.detected?.name ?? (files ? "Custom version" : "Not selected")}
					{#if files}
						({files.format === Format.FormatAssets ? "12+ Assets" : "DAT/SPR"})
					{/if}
				</span>
				<span class="dot">·</span>
				<span
					><strong>Server:</strong>
					{workspace.serverRoot ? (server?.profile ?? "Unknown/Generic") : "None"}</span
				>
				<span class="dot">·</span>
				<span><strong>Maps:</strong> {server?.maps?.length ?? 0}</span>
			</p>
		</div>

		<div class="ws-id-picker" role="group" aria-label="Preferred item ID mode">
			<span class="ws-id-label">ID Mode:</span>
			{#each ID_MODES as mode (mode.value)}
				<button
					type="button"
					class="ws-id-btn"
					class:active={workspace.preferredItemIds === mode.value}
					title={mode.hint}
					onclick={() => (workspace.preferredItemIds = mode.value)}
				>
					{mode.label}
				</button>
			{/each}
		</div>
	</div>

	{#if workspace.scanError}
		<div class="ws-alert ws-alert-error">
			<span>{workspace.scanError}</span>
			<button type="button" class="ws-link-btn" onclick={() => workspace.inspect()}>Retry scan</button>
		</div>
	{:else if workspace.openBlockedReason}
		<div class="ws-note">{workspace.openBlockedReason}</div>
	{/if}

	<div class="ws-overview-actions">
		{#if workspace.scanning}
			<button type="button" class="ws-btn ws-btn-ghost" onclick={() => workspace.cancelScan()}>
				Cancel Scan
			</button>
		{:else if workspace.clientRoot || workspace.serverRoot}
			<button type="button" class="ws-btn ws-btn-ghost" onclick={() => workspace.inspect()}> Rescan </button>
			<button type="button" class="ws-btn ws-btn-ghost" onclick={() => workspace.saveCurrentProfile()}>
				Save Workspace
			</button>
		{/if}

		{#if app.open}
			<button
				type="button"
				class="ws-btn ws-btn-ghost"
				onclick={() => {
					app.showWorkspace = false;
					map.dialog = "data";
				}}
			>
				Review Server Data…
			</button>
		{/if}

		<div class="ws-grow"></div>

		<button
			type="button"
			class="ws-btn ws-btn-secondary"
			disabled={!workspace.canOpenEditor}
			onclick={() => workspace.openEditor("map")}
		>
			Open Map Editor
		</button>
		<button
			type="button"
			class="ws-btn ws-btn-primary ws-btn-lg"
			disabled={!workspace.canOpenEditor}
			onclick={() => workspace.openEditor("objects")}
		>
			{workspace.opening ? "Opening…" : "Open Editor"}
		</button>
	</div>
</section>

<style>
	.ws-overview {
		background: var(--ui-surface);
		border: 1px solid var(--ui-border);
		border-radius: var(--ui-radius);
		padding: 16px 18px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.ws-overview-top {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
	}
	.ws-section-title {
		margin: 0;
		font-size: 12px;
		font-weight: 700;
		letter-spacing: 0.08em;
		color: var(--ui-muted);
	}
	.ws-overview-line {
		margin: 6px 0 0;
		font-size: 13px;
		color: var(--ui-text);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px;
	}
	.dot {
		color: var(--ui-muted);
	}
	.ws-id-picker {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px;
	}
	.ws-id-label {
		font-size: 12px;
		font-weight: 600;
		color: var(--ui-muted);
		margin-right: 2px;
	}
	.ws-id-btn {
		height: 30px;
		padding: 0 10px;
		border-radius: 7px;
		border: 1px solid var(--ui-border);
		background: var(--ui-bg);
		color: var(--ui-muted);
		font: inherit;
		font-size: 12px;
		font-weight: 500;
		cursor: pointer;
	}
	.ws-id-btn:hover {
		color: var(--ui-text);
		border-color: var(--ui-primary);
	}
	.ws-id-btn.active {
		background: color-mix(in srgb, var(--ui-primary) 20%, transparent);
		border-color: var(--ui-primary);
		color: var(--ui-text);
		font-weight: 600;
	}
	.ws-note {
		font-size: 12px;
		color: var(--ui-warning);
	}
	.ws-overview-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		padding-top: 6px;
		border-top: 1px solid var(--ui-border);
	}
	.ws-grow {
		flex: 1;
	}
</style>
