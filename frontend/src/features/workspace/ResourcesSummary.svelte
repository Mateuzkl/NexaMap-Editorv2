<script lang="ts">
	import { Format, ResourceState, type ResourceResult } from "../../lib/api";
	import { workspace } from "./workspace.svelte";

	let editingPaths = $state(false);

	const server = $derived(workspace.inspection?.server ?? null);
	const files = $derived(workspace.clientFiles);

	function stateBadge(state: ResourceState): { text: string; kind: string } {
		switch (state) {
			case ResourceState.ResourceFound:
				return { text: "Found", kind: "ok" };
			case ResourceState.ResourceInvalid:
				return { text: "Invalid", kind: "error" };
			case ResourceState.ResourceMissing:
				return { text: "Missing", kind: "warn" };
			case ResourceState.ResourceUnsupported:
				return { text: "Unsupported", kind: "error" };
			default:
				return { text: "Not configured", kind: "neutral" };
		}
	}

	function chipMeta(res: ResourceResult | undefined): string {
		if (!res) return "Not configured";
		if (res.error) return res.error;
		return res.details || (res.state === ResourceState.ResourceFound ? "Found" : "Not found");
	}
</script>

<section class="ws-section" aria-label="Detected resources">
	<header class="ws-section-head">
		<h3 class="ws-section-title">DETECTED RESOURCES</h3>
		<button type="button" class="ws-link-btn" onclick={() => (editingPaths = !editingPaths)}>
			{editingPaths ? "Hide resource mapping" : "Manage resource mapping"}
		</button>
	</header>

	<div class="ws-res-grid">
		<div class="ws-res-chip" title={server?.itemsOtb.path || chipMeta(server?.itemsOtb)}>
			<div class="ws-res-top">
				<span class="ws-res-name">items.otb</span>
				<span
					class="ws-status ws-status-{stateBadge(
						server?.itemsOtb.state ?? ResourceState.ResourceNotConfigured,
					).kind}"
				>
					{stateBadge(server?.itemsOtb.state ?? ResourceState.ResourceNotConfigured).text}
				</span>
			</div>
			<span class="ws-res-detail">{chipMeta(server?.itemsOtb)}</span>
		</div>

		<div class="ws-res-chip" title={server?.itemsXml.path || chipMeta(server?.itemsXml)}>
			<div class="ws-res-top">
				<span class="ws-res-name">items.xml</span>
				<span
					class="ws-status ws-status-{stateBadge(
						server?.itemsXml.state ?? ResourceState.ResourceNotConfigured,
					).kind}"
				>
					{stateBadge(server?.itemsXml.state ?? ResourceState.ResourceNotConfigured).text}
				</span>
			</div>
			<span class="ws-res-detail">{chipMeta(server?.itemsXml)}</span>
		</div>

		<div class="ws-res-chip">
			<div class="ws-res-top">
				<span class="ws-res-name">Maps</span>
				<span class="ws-status ws-status-{(server?.maps?.length ?? 0) > 0 ? 'ok' : 'neutral'}">
					{server?.maps?.length ?? 0}
				</span>
			</div>
			<span class="ws-res-detail">
				{(server?.maps?.length ?? 0) > 0 ? ".otbm / .otmap files ready" : "No maps found"}
			</span>
		</div>

		<div class="ws-res-chip" title={server?.monsters.path || ""}>
			<div class="ws-res-top">
				<span class="ws-res-name">Monsters</span>
				<span
					class="ws-status ws-status-{stateBadge(
						server?.monsters.state ?? ResourceState.ResourceNotConfigured,
					).kind}"
				>
					{stateBadge(server?.monsters.state ?? ResourceState.ResourceNotConfigured).text}
				</span>
			</div>
			<span class="ws-res-detail">{chipMeta(server?.monsters)}</span>
		</div>

		<div class="ws-res-chip" title={server?.npcs.path || ""}>
			<div class="ws-res-top">
				<span class="ws-res-name">NPCs</span>
				<span
					class="ws-status ws-status-{stateBadge(server?.npcs.state ?? ResourceState.ResourceNotConfigured)
						.kind}"
				>
					{stateBadge(server?.npcs.state ?? ResourceState.ResourceNotConfigured).text}
				</span>
			</div>
			<span class="ws-res-detail">{chipMeta(server?.npcs)}</span>
		</div>

		<div class="ws-res-chip" title={files?.datPath || ""}>
			<div class="ws-res-top">
				<span class="ws-res-name">Client Assets</span>
				<span class="ws-status ws-status-{files ? 'ok' : 'neutral'}">
					{files ? (files.format === Format.FormatAssets ? "12+ Assets" : "DAT/SPR") : "None"}
				</span>
			</div>
			<span class="ws-res-detail">
				{files?.detected ? `Version ${files.detected.name}` : files ? "Custom version" : "Select client"}
			</span>
		</div>
	</div>

	{#if editingPaths}
		<div class="ws-mapping">
			<div class="ws-map-row">
				<span class="ws-map-label">items.otb</span>
				<input
					class="ws-input"
					readonly
					value={workspace.selectedOtbPath || server?.itemsOtb.path || ""}
					placeholder="Auto-detected or pick manually…"
					title={workspace.selectedOtbPath || server?.itemsOtb.path || ""}
				/>
				<button type="button" class="ws-btn ws-btn-ghost" onclick={() => workspace.pickOtbOverride()}>
					Browse…
				</button>
				{#if workspace.selectedOtbPath}
					<button type="button" class="ws-btn ws-btn-ghost" onclick={() => workspace.setOtbOverride("")}>
						Auto
					</button>
				{/if}
			</div>
			{#if (server?.itemsOtb.candidates?.length ?? 0) > 1}
				<div class="ws-cand-list">
					<span class="ws-cand-title">Multiple items.otb found:</span>
					{#each server?.itemsOtb.candidates ?? [] as cand (cand)}
						<button
							type="button"
							class="ws-cand-pill"
							class:active={cand === (workspace.selectedOtbPath || server?.itemsOtb.path)}
							title={cand}
							onclick={() => workspace.setOtbOverride(cand)}
						>
							{cand}
						</button>
					{/each}
				</div>
			{/if}

			<div class="ws-map-row">
				<span class="ws-map-label">items.xml</span>
				<input
					class="ws-input"
					readonly
					value={workspace.selectedXmlPath || server?.itemsXml.path || ""}
					placeholder="Auto-detected or pick manually…"
					title={workspace.selectedXmlPath || server?.itemsXml.path || ""}
				/>
				<button type="button" class="ws-btn ws-btn-ghost" onclick={() => workspace.pickXmlOverride()}>
					Browse…
				</button>
				{#if workspace.selectedXmlPath}
					<button type="button" class="ws-btn ws-btn-ghost" onclick={() => workspace.setXmlOverride("")}>
						Auto
					</button>
				{/if}
			</div>

			{#if workspace.attachedOtbPath && workspace.effectiveOtbPath && workspace.attachedOtbPath.toLowerCase() !== workspace.effectiveOtbPath.toLowerCase()}
				<div class="ws-alert ws-alert-warn">
					This client currently has a different <code>items.otb</code> attached ({workspace.attachedOtbPath}).
					You will be asked to confirm before switching it on Open Editor.
				</div>
			{/if}
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
		gap: 12px;
	}
	.ws-section-head {
		display: flex;
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
	.ws-res-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(175px, 1fr));
		gap: 10px;
	}
	.ws-res-chip {
		background: var(--ui-bg);
		border: 1px solid var(--ui-border);
		border-radius: 10px;
		padding: 10px 12px;
		display: flex;
		flex-direction: column;
		gap: 5px;
		min-width: 0;
	}
	.ws-res-top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 6px;
	}
	.ws-res-name {
		font-size: 12.5px;
		font-weight: 600;
		color: var(--ui-text);
	}
	.ws-res-detail {
		font-size: 11.5px;
		color: var(--ui-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ws-mapping {
		border-top: 1px solid var(--ui-border);
		padding-top: 12px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.ws-map-row {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.ws-map-label {
		width: 72px;
		flex: none;
		font-size: 12px;
		font-weight: 600;
		color: var(--ui-muted);
	}
	.ws-input {
		flex: 1;
		min-width: 0;
		height: 34px;
		padding: 0 10px;
		border-radius: 8px;
		border: 1px solid var(--ui-border);
		background: var(--ui-bg);
		color: var(--ui-text);
		font: inherit;
		font-size: 12px;
	}
	.ws-cand-list {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px;
		padding-left: 80px;
	}
	.ws-cand-title {
		font-size: 11px;
		color: var(--ui-warning);
	}
	.ws-cand-pill {
		max-width: 280px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		padding: 3px 8px;
		border-radius: 99px;
		border: 1px solid var(--ui-border);
		background: var(--ui-surface-raised);
		color: var(--ui-muted);
		font-size: 11px;
		cursor: pointer;
	}
	.ws-cand-pill.active {
		border-color: var(--ui-primary);
		color: var(--ui-text);
	}
</style>
