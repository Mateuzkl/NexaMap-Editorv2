<script lang="ts">
	import {
		ConverterService,
		DialogService,
		errorMessage,
		type CustomServerAnalysis,
		type CustomServerReport,
	} from "../../lib/api";
	import { toast } from "../../lib/state/app.svelte";
	import { workspace } from "./workspace.svelte";

	let sourceRoot = $state(workspace.serverRoot || "");
	let destinationRoot = $state("");
	let includeMaps = $state(true);
	let includeXml = $state(true);
	let includeOtb = $state(true);
	let allowDuplicateClientIds = $state(false);

	let analyzing = $state(false);
	let converting = $state(false);
	let analysis = $state.raw<CustomServerAnalysis | null>(null);
	let report = $state.raw<CustomServerReport | null>(null);

	async function pickSource() {
		const dir = await DialogService.PickDirectory("customConverterSrc", "Select source server folder");
		if (!dir) return;
		sourceRoot = dir;
		analysis = null;
		report = null;
		await analyze();
	}

	async function pickDest() {
		const dir = await DialogService.PickDirectory("customConverterDst", "Select output folder (new / empty)");
		if (!dir) return;
		destinationRoot = dir;
	}

	async function analyze() {
		if (!sourceRoot.trim()) {
			toast("Select a source server folder first.", "error");
			return;
		}
		analyzing = true;
		report = null;
		try {
			analysis = await ConverterService.AnalyzeCustomServer(
				sourceRoot,
				{ maps: includeMaps, itemsXml: includeXml, itemsOtb: includeOtb },
				allowDuplicateClientIds,
			);
		} catch (err) {
			toast(errorMessage(err), "error");
		} finally {
			analyzing = false;
		}
	}

	async function convert() {
		if (!sourceRoot.trim() || !destinationRoot.trim()) {
			toast("Select both a source server folder and a destination folder.", "error");
			return;
		}
		converting = true;
		report = null;
		try {
			const res = await ConverterService.ConvertCustomServer({
				sourceRoot,
				destinationRoot,
				scope: { maps: includeMaps, itemsXml: includeXml, itemsOtb: includeOtb },
				allowDuplicateClientIds,
			});
			report = res;
			if (res.success) {
				toast(`Custom server converted in ${(res.durationMs / 1000).toFixed(2)}s.`, "success");
			} else if (res.error) {
				toast(res.error, "error");
			}
		} catch (err) {
			toast(errorMessage(err), "error");
		} finally {
			converting = false;
		}
	}
</script>

<section class="cc-card" aria-label="Custom Server Converter">
	<header class="cc-head">
		<div>
			<h2 class="cc-title">Custom Server Converter (ServerID → ClientID)</h2>
			<p class="cc-sub">
				Converts a server's `.otbm`/`.otmap` maps, `items.xml`, and `items.otb` from Server IDs to Client IDs
				into a new output folder. Source files are never overwritten.
			</p>
		</div>
	</header>

	<div class="cc-paths">
		<div class="cc-row">
			<span class="cc-label">Source Server</span>
			<input
				class="cc-input"
				readonly
				value={sourceRoot}
				placeholder="Folder containing data/items/items.otb, items.xml, maps…"
				title={sourceRoot}
			/>
			<button type="button" class="ws-btn ws-btn-secondary ws-btn-sm" onclick={pickSource}>Browse…</button>
		</div>
		<div class="cc-row">
			<span class="cc-label">Output Folder</span>
			<input
				class="cc-input"
				readonly
				value={destinationRoot}
				placeholder="Choose a separate destination folder…"
				title={destinationRoot}
			/>
			<button type="button" class="ws-btn ws-btn-secondary ws-btn-sm" onclick={pickDest}>Browse…</button>
		</div>
	</div>

	<div class="cc-options">
		<label class="cc-check">
			<input class="t-check" type="checkbox" bind:checked={includeMaps} />
			<span>Maps (`.otbm` / `.otmap`)</span>
		</label>
		<label class="cc-check">
			<input class="t-check" type="checkbox" bind:checked={includeXml} />
			<span>`items.xml`</span>
		</label>
		<label class="cc-check">
			<input class="t-check" type="checkbox" bind:checked={includeOtb} />
			<span>`items.otb` (ServerID = ClientID)</span>
		</label>
		<label class="cc-check">
			<input class="t-check" type="checkbox" bind:checked={allowDuplicateClientIds} />
			<span>Allow duplicate ClientIDs</span>
		</label>
	</div>

	<div class="cc-actions">
		<button
			type="button"
			class="ws-btn ws-btn-secondary"
			disabled={analyzing || converting || !sourceRoot}
			onclick={analyze}
		>
			{analyzing ? "Analyzing…" : "Analyze Server"}
		</button>
		<button
			type="button"
			class="ws-btn ws-btn-primary"
			disabled={converting || analyzing || !sourceRoot || !destinationRoot}
			onclick={convert}
		>
			{converting ? "Converting…" : "Convert to ClientID"}
		</button>
	</div>

	{#if analysis}
		<div class="cc-report">
			{#if analysis.error}
				<div class="ws-alert ws-alert-error">{analysis.error}</div>
			{:else}
				<div class="cc-stats">
					<span
						><strong>Mapped items:</strong>
						{analysis.stats.mappedItems.toLocaleString()} / {analysis.stats.totalItems.toLocaleString()}</span
					>
					<span><strong>Unmapped:</strong> {analysis.stats.unmappedItems.toLocaleString()}</span>
					<span
						><strong>Duplicate ClientIDs:</strong>
						{analysis.stats.duplicateClientIds.toLocaleString()}</span
					>
					<span><strong>Maps found:</strong> {analysis.server.maps?.length ?? 0}</span>
				</div>
			{/if}
			{#each analysis.warnings ?? [] as w}
				<div class="ws-alert ws-alert-warn">{w}</div>
			{/each}
		</div>
	{/if}

	{#if report}
		<div class="cc-report">
			{#if report.error}
				<div class="ws-alert ws-alert-error">{report.error}</div>
			{:else if report.success}
				<div class="cc-done">
					<strong>Conversion completed</strong> in {(report.durationMs / 1000).toFixed(2)}s
				</div>
				{#if report.maps?.length}
					<div class="cc-list">
						{#each report.maps as m (m.source)}
							<div class="cc-item">
								<strong>{m.name}</strong>:
								{#if m.error}
									<span class="err">{m.error}</span>
								{:else}
									{m.tileCount.toLocaleString()} tiles · {m.changedItems.toLocaleString()} /
									{m.totalItems.toLocaleString()} items remapped
									{#if m.missingItems > 0}({m.missingItems} unmapped){/if}
								{/if}
							</div>
						{/each}
					</div>
				{/if}
				{#if report.itemsXml}
					<div class="cc-item">
						<strong>items.xml</strong>:
						{report.itemsXml.error ||
							`${report.itemsXml.updatedTags.toLocaleString()} / ${report.itemsXml.totalTags.toLocaleString()} tags updated`}
					</div>
				{/if}
				{#if report.itemsOtb}
					<div class="cc-item">
						<strong>items.otb</strong>:
						{report.itemsOtb.error ||
							`${report.itemsOtb.remapped.toLocaleString()} / ${report.itemsOtb.totalItems.toLocaleString()} items remapped`}
					</div>
				{/if}
			{/if}
		</div>
	{/if}
</section>

<style>
	.cc-card {
		background: var(--ui-surface);
		border: 1px solid var(--ui-border);
		border-radius: var(--ui-radius);
		padding: 18px 20px;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.cc-title {
		margin: 0;
		font-size: 15px;
		font-weight: 700;
		color: var(--ui-text);
	}
	.cc-sub {
		margin: 4px 0 0;
		font-size: 12.5px;
		color: var(--ui-muted);
		line-height: 1.45;
	}
	.cc-paths {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.cc-row {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.cc-label {
		width: 100px;
		flex: none;
		font-size: 12px;
		font-weight: 600;
		color: var(--ui-muted);
	}
	.cc-input {
		flex: 1;
		min-width: 0;
		height: 34px;
		padding: 0 10px;
		border-radius: 8px;
		border: 1px solid var(--ui-border);
		background: var(--ui-bg);
		color: var(--ui-text);
		font: inherit;
		font-size: 12.5px;
	}
	.cc-options {
		display: flex;
		flex-wrap: wrap;
		gap: 12px 18px;
	}
	.cc-check {
		display: inline-flex;
		align-items: center;
		gap: 7px;
		font-size: 12.5px;
		color: var(--ui-text);
		cursor: pointer;
	}
	.cc-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}
	.cc-report {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding-top: 10px;
		border-top: 1px solid var(--ui-border);
	}
	.cc-stats {
		display: flex;
		flex-wrap: wrap;
		gap: 12px 18px;
		font-size: 12.5px;
		color: var(--ui-text);
	}
	.cc-done {
		font-size: 13px;
		color: var(--ui-success);
	}
	.cc-list {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.cc-item {
		font-size: 12px;
		color: var(--ui-muted);
	}
	.cc-item strong {
		color: var(--ui-text);
	}
	.err {
		color: var(--ui-danger);
	}
</style>
