<script lang="ts">
	import { onDestroy, onMount } from "svelte";
	import { DialogService, errorMessage } from "../../../lib/api";
	import {
		MapService,
		type AutoLearnedBorder,
		type LearnedBorderReport,
		type LearnedBorderSlot,
		type LearnedBorderTransition,
		type WholeMapBorderReport,
	} from "../../../lib/map/data/api";
	import { map, reloadPalette } from "../../../lib/map/state.svelte";
	import { toast } from "../../../lib/state/app.svelte";
	import Dialog from "../../../lib/ui/Dialog.svelte";
	import ItemIcon from "../ItemIcon.svelte";
	import ItemPicker from "../ItemPicker.svelte";

	const EDGE_LABELS: Record<string, string> = {
		cnw: "Corner NW",
		n: "North",
		cne: "Corner NE",
		w: "West",
		e: "East",
		csw: "Corner SW",
		s: "South",
		cse: "Corner SE",
		dnw: "Diag NW",
		dne: "Diag NE",
		dsw: "Diag SW",
		dse: "Diag SE",
	};

	const GRID_3X3: (string | null)[] = ["cnw", "n", "cne", "w", null, "e", "csw", "s", "cse"];
	const DIAGS = ["dnw", "dne", "dsw", "dse"];
	const PREVIEW_EDGES = ["n", "e", "s", "w", "cnw", "cne", "csw", "cse", "dnw", "dne", "dsw", "dse"];

	const hasSelection = $derived((map.active?.selection ?? 0) > 0);
	let mode = $state<"selection" | "map">(
		map.learnBorderMode === "map" || !(map.active?.selection ?? 0) ? "map" : "selection",
	);

	let loading = $state(false);
	let saving = $state(false);
	let error = $state("");
	let report = $state<LearnedBorderReport | null>(null);
	let transitionIdx = $state(0);

	let edges = $state<Record<string, number>>({});
	let selectedEdge = $state<string>("n");
	let brushName = $state("");
	let align = $state<"outer" | "inner">("outer");
	let toTarget = $state("all");
	let zOrder = $state(3500);
	let borderIdPreview = $state(1000);

	// Whole-map auto-learn state
	let mapLoading = $state(false);
	let mapError = $state("");
	let mapReport = $state<WholeMapBorderReport | null>(null);
	let onlyMissing = $state(true);
	let minEdges = $state(4);
	let mapBorders = $state<AutoLearnedBorder[]>([]);
	let selectedMapIdx = $state<Set<number>>(new Set());

	let mounted = true;
	let reqSeq = 0;
	onDestroy(() => {
		mounted = false;
	});

	const families = $derived(report?.families ?? []);
	const transitions = $derived(report?.transitions ?? []);
	const transition = $derived<LearnedBorderTransition | null>(transitions[transitionIdx] ?? null);
	const sourceFamily = $derived(transition ? (families.find((f) => f.index === transition.familyA) ?? null) : null);
	const groundIDs = $derived<number[]>(sourceFamily?.itemIds ?? []);
	const groundLook = $derived<number>(sourceFamily?.representativeId ?? groundIDs[0] ?? 0);
	const activeSlot = $derived<LearnedBorderSlot | null>(
		transition?.slots?.find((s) => s.edge === selectedEdge) ?? null,
	);
	const assignedCount = $derived(Object.values(edges).filter((id) => id > 0).length);

	const visibleMapBorders = $derived(
		mapBorders.map((b, idx) => ({ b, idx })).filter(({ b }) => b.assignedCount >= minEdges),
	);

	const chosenMapBorders = $derived(visibleMapBorders.filter(({ idx }) => selectedMapIdx.has(idx)).map(({ b }) => b));

	function applyTransition(idx: number) {
		transitionIdx = idx;
		const tr = (report?.transitions ?? [])[idx];
		if (!tr) return;
		const nextEdges: Record<string, number> = {};
		for (const slot of tr.slots ?? []) {
			if (slot.itemId > 0) nextEdges[slot.edge] = slot.itemId;
		}
		edges = nextEdges;
		align = tr.align === "inner" ? "inner" : "outer";
		toTarget = align === "inner" ? "none" : "all";
		const fam = (report?.families ?? []).find((f) => f.index === tr.familyA);
		brushName = fam ? fam.name : "learned_ground";
	}

	async function analyzeSelection() {
		if (map.activeId === null) return;
		const seq = ++reqSeq;
		loading = true;
		error = "";
		try {
			const res = await MapService.LearnBorder(map.activeId);
			if (!mounted || seq !== reqSeq) return;
			report = res;
			if (res?.transitions?.length) {
				applyTransition(0);
			} else if (res?.families?.length) {
				brushName = res.families[0].name;
				edges = {};
			}
		} catch (err) {
			if (!mounted || seq !== reqSeq) return;
			error = errorMessage(err);
		} finally {
			if (mounted && seq === reqSeq) loading = false;
		}
	}

	async function scanWholeMap() {
		if (map.activeId === null) return;
		const seq = ++reqSeq;
		mapLoading = true;
		mapError = "";
		try {
			const res = await MapService.LearnBordersFromMap(map.activeId, onlyMissing);
			if (!mounted || seq !== reqSeq) return;
			mapReport = res;
			const list = (res?.borders ?? []).map((b) => ({ ...b }));
			mapBorders = list;
			const nextSel = new Set<number>();
			list.forEach((b, i) => {
				if (b.assignedCount >= minEdges) nextSel.add(i);
			});
			selectedMapIdx = nextSel;
		} catch (err) {
			if (!mounted || seq !== reqSeq) return;
			mapError = errorMessage(err);
		} finally {
			if (mounted && seq === reqSeq) mapLoading = false;
		}
	}

	function switchMode(next: "selection" | "map") {
		if (next === "selection" && !hasSelection) return;
		mode = next;
		if (next === "selection" && !report) void analyzeSelection();
		if (next === "map" && !mapReport) void scanWholeMap();
	}

	function setEdge(edge: string, id: number) {
		const next = { ...edges };
		if (id > 0) next[edge] = id;
		else delete next[edge];
		edges = next;
	}

	function toggleMapBorder(idx: number) {
		const next = new Set(selectedMapIdx);
		if (next.has(idx)) next.delete(idx);
		else next.add(idx);
		selectedMapIdx = next;
	}

	function selectAllVisible(on: boolean) {
		const next = new Set(selectedMapIdx);
		for (const { idx } of visibleMapBorders) {
			if (on) next.add(idx);
			else next.delete(idx);
		}
		selectedMapIdx = next;
	}

	async function saveToEditor() {
		if (assignedCount === 0) {
			toast("Assign at least one border edge item first.", "error");
			return;
		}
		saving = true;
		try {
			const id = await MapService.SaveLearnedBorder(
				brushName.trim(),
				groundIDs,
				edges,
				align,
				toTarget.trim(),
				Number(zOrder) || 3500,
			);
			borderIdPreview = id;
			await reloadPalette();
			toast(
				`Border #${id} saved${brushName.trim() ? ` to brush "${brushName.trim()}" (Learned Borders)` : ""}. Ready with Auto-Border (A)!`,
				"success",
			);
		} catch (err) {
			toast(errorMessage(err), "error");
		} finally {
			if (mounted) saving = false;
		}
	}

	async function exportXML() {
		if (assignedCount === 0) {
			toast("Assign at least one border edge item first.", "error");
			return;
		}
		const base = (brushName.trim() || "learned_border").replace(/[^a-zA-Z0-9_-]+/g, "_");
		const path = await DialogService.SaveFile("exportBorderXml", "Export RME Border XML", `${base}.xml`, [
			{ name: "RME XML (*.xml)", pattern: "*.xml" },
		]);
		if (!path) return;
		saving = true;
		try {
			const saved = await MapService.ExportLearnedBorderXML(
				path,
				borderIdPreview,
				brushName.trim(),
				groundIDs,
				edges,
				align,
				toTarget.trim(),
				Number(zOrder) || 3500,
			);
			toast(`Border XML exported: ${saved}`, "success");
		} catch (err) {
			toast(errorMessage(err), "error");
		} finally {
			if (mounted) saving = false;
		}
	}

	async function saveAllMapBorders() {
		if (!chosenMapBorders.length) return;
		saving = true;
		try {
			const count = await MapService.SaveLearnedBordersBatch(chosenMapBorders);
			await reloadPalette();
			toast(
				`Saved ${count} auto-learned border${count === 1 ? "" : "s"} in "Auto-Learned Borders". Auto-Border (A) is active now!`,
				"success",
			);
		} catch (err) {
			toast(errorMessage(err), "error");
		} finally {
			if (mounted) saving = false;
		}
	}

	async function exportAllMapBordersXML() {
		if (!chosenMapBorders.length) return;
		const mapBase = (map.active?.name ?? "map").replace(/\.otbm$/i, "").replace(/[^a-zA-Z0-9_-]+/g, "_");
		const path = await DialogService.SaveFile(
			"exportAllBordersXml",
			"Export Auto-Learned Borders XML",
			`${mapBase}_auto_borders.xml`,
			[{ name: "RME XML (*.xml)", pattern: "*.xml" }],
		);
		if (!path) return;
		saving = true;
		try {
			const saved = await MapService.ExportLearnedBordersBatchXML(path, chosenMapBorders, 10000);
			toast(`Exported ${chosenMapBorders.length} borders to ${saved}`, "success");
		} catch (err) {
			toast(errorMessage(err), "error");
		} finally {
			if (mounted) saving = false;
		}
	}

	function close() {
		map.dialog = null;
	}

	onMount(() => {
		if (mode === "map") void scanWholeMap();
		else void analyzeSelection();
	});
</script>

<Dialog title="Learn Borders (Selection & Whole Map)" width={860} onclose={close}>
	<div class="lb-tabs">
		<button
			class="lb-tab"
			class:on={mode === "selection"}
			disabled={!hasSelection}
			onclick={() => switchMode("selection")}
		>
			Selection (Single Border Editor)
		</button>
		<button class="lb-tab" class:on={mode === "map"} onclick={() => switchMode("map")}>
			Whole Map (Auto-Detect All Missing)
		</button>
	</div>

	{#if mode === "selection"}
		{#if loading}
			<div class="lb-empty">Analyzing selected ground &amp; border tiles…</div>
		{:else if error}
			<div class="lb-empty lb-err">{error}</div>
		{:else if report}
			<div class="lb-wrap">
				<div class="lb-top">
					<div class="lb-meta">
						Floor <b>z{report.floor}</b> · <b>{report.selectedTiles}</b> selected tiles ·
						<b>{families.length}</b> ground {families.length === 1 ? "family" : "families"}
					</div>
					<button class="t-btn" onclick={analyzeSelection}>Re-scan selection</button>
				</div>

				{#if transitions.length > 0}
					<label class="lb-field">
						<span>Detected ground transition</span>
						<select
							class="t-input"
							value={transitionIdx}
							onchange={(e) => applyTransition(Number(e.currentTarget.value))}
						>
							{#each transitions as tr, i}
								<option value={i}>
									{tr.label} — {tr.assignedCount}/12 edges · {Math.round(tr.overallConfidence * 100)}%
									conf · {Math.round(tr.matchRate * 100)}% match
								</option>
							{/each}
						</select>
					</label>
				{:else}
					<div class="lb-note">
						No automatic border transitions were detected in this selection (select a ground island
						surrounded by border tiles and neighboring ground/void). You can still assign edges manually
						below.
					</div>
				{/if}

				<div class="lb-main">
					<div class="lb-grids">
						<div class="lb-sub">Outer / Cardinal &amp; Corner Edges (click any slot to edit)</div>
						<div class="lb-grid3">
							{#each GRID_3X3 as edge}
								{#if edge === null}
									<div class="lb-center" title="Source ground ({groundIDs.join(', ')})">
										{#if groundLook}<ItemIcon id={groundLook} size={32} crisp />{/if}
										<span>Ground</span>
									</div>
								{:else}
									{@const itemId = edges[edge] ?? 0}
									{@const slot = transition?.slots?.find((s) => s.edge === edge)}
									<button
										class="lb-slot"
										class:on={selectedEdge === edge}
										class:filled={itemId > 0}
										class:warn={slot?.ambiguous}
										onclick={() => (selectedEdge = edge)}
										title="{EDGE_LABELS[edge]} ({edge}){itemId ? ` · ID ${itemId}` : ' · empty'}"
									>
										{#if itemId > 0}
											<ItemIcon id={itemId} size={32} crisp />
										{:else}
											<span class="lb-dash">—</span>
										{/if}
										<small>{edge.toUpperCase()}</small>
										{#if itemId > 0}<span class="lb-id">{itemId}</span>{/if}
									</button>
								{/if}
							{/each}
						</div>

						<div class="lb-sub">Diagonal Inner Corners (DNW / DNE / DSW / DSE)</div>
						<div class="lb-grid4">
							{#each DIAGS as edge}
								{@const itemId = edges[edge] ?? 0}
								{@const slot = transition?.slots?.find((s) => s.edge === edge)}
								<button
									class="lb-slot"
									class:on={selectedEdge === edge}
									class:filled={itemId > 0}
									class:warn={slot?.ambiguous}
									onclick={() => (selectedEdge = edge)}
									title="{EDGE_LABELS[edge]} ({edge}){itemId ? ` · ID ${itemId}` : ' · empty'}"
								>
									{#if itemId > 0}
										<ItemIcon id={itemId} size={32} crisp />
									{:else}
										<span class="lb-dash">—</span>
									{/if}
									<small>{edge.toUpperCase()}</small>
									{#if itemId > 0}<span class="lb-id">{itemId}</span>{/if}
								</button>
							{/each}
						</div>
					</div>

					<div class="lb-side">
						<div class="lb-box">
							<h4>Slot: {EDGE_LABELS[selectedEdge]} ({selectedEdge})</h4>
							<div class="lb-row">
								<ItemPicker
									value={edges[selectedEdge] ?? 0}
									placeholder="Pick or type item ID…"
									onchange={(id) => setEdge(selectedEdge, id)}
								/>
								<button
									class="t-btn"
									disabled={!edges[selectedEdge]}
									onclick={() => setEdge(selectedEdge, 0)}>Clear</button
								>
							</div>
							{#if activeSlot?.alternatives?.length}
								<div class="lb-sub">Detected candidates</div>
								<div class="lb-cands">
									{#each activeSlot.alternatives.slice(0, 6) as cand}
										<button
											class="lb-cand"
											class:on={edges[selectedEdge] === cand.itemId}
											onclick={() => setEdge(selectedEdge, cand.itemId)}
										>
											<ItemIcon id={cand.itemId} size={22} />
											<span class="grow"
												>{map.items?.name(cand.itemId) || `Item ${cand.itemId}`}</span
											>
											<small>{cand.itemId} · {Math.round(cand.confidence * 100)}%</small>
										</button>
									{/each}
								</div>
							{/if}
						</div>

						<div class="lb-box">
							<h4>Save as Ground Brush + Auto-Border</h4>
							<label class="lb-field">
								<span>Brush name</span>
								<input class="t-input" bind:value={brushName} placeholder="e.g. custom_lava_ground" />
							</label>
							<div class="lb-row2">
								<label class="lb-field">
									<span>Align</span>
									<select class="t-input" bind:value={align}>
										<option value="outer">Outer</option>
										<option value="inner">Inner</option>
									</select>
								</label>
								<label class="lb-field">
									<span>Target ("to")</span>
									<input class="t-input" bind:value={toTarget} placeholder="all / none" />
								</label>
								<label class="lb-field">
									<span>Z-Order</span>
									<input class="t-input" type="number" bind:value={zOrder} />
								</label>
							</div>
							<div class="lb-note">
								Assigned edges: <b>{assignedCount}/12</b> · Ground IDs:
								<b>{groundIDs.length ? groundIDs.join(", ") : "none"}</b>
							</div>
						</div>
					</div>
				</div>
			</div>
		{/if}
	{:else}
		<div class="lb-wrap">
			<div class="lb-top">
				<label class="lb-check">
					<input type="checkbox" bind:checked={onlyMissing} onchange={scanWholeMap} disabled={mapLoading} />
					<span>Only unconfigured / missing borders</span>
				</label>
				<label class="lb-inline">
					<span>Min edges:</span>
					<select class="t-input lb-minsel" bind:value={minEdges}>
						<option value={2}>2+</option>
						<option value={4}>4+</option>
						<option value={8}>8+</option>
						<option value={12}>12 (complete)</option>
					</select>
				</label>
				<span class="grow"></span>
				<button class="t-btn" disabled={mapLoading || saving} onclick={scanWholeMap}>
					{mapLoading ? "Scanning map…" : "Re-scan map"}
				</button>
			</div>

			{#if mapLoading}
				<div class="lb-empty">Scanning every floor of the map and clustering custom grounds &amp; borders…</div>
			{:else if mapError}
				<div class="lb-empty lb-err">{mapError}</div>
			{:else if mapReport}
				<div class="lb-top">
					<div class="lb-meta">
						Scanned <b>{mapReport.scannedTiles.toLocaleString()}</b> tiles ·
						<b>{mapReport.distinctGrounds}</b> ground IDs (<b>{mapReport.missingGrounds}</b> unconfigured) ·
						<b>{visibleMapBorders.length}</b> auto-learned border sets
					</div>
					<div class="lb-row">
						<button class="t-btn" onclick={() => selectAllVisible(true)}>Select all</button>
						<button class="t-btn" onclick={() => selectAllVisible(false)}>None</button>
					</div>
				</div>

				{#if visibleMapBorders.length === 0}
					<div class="lb-empty">
						No border sets with {minEdges}+ edges detected. Try lowering <b>Min edges</b> or unchecking
						<b>Only unconfigured</b>.
					</div>
				{:else}
					<div class="lb-map-list">
						{#each visibleMapBorders as { b, idx } (idx)}
							<div class="lb-map-row" class:on={selectedMapIdx.has(idx)}>
								<input
									type="checkbox"
									checked={selectedMapIdx.has(idx)}
									onchange={() => toggleMapBorder(idx)}
								/>
								<ItemIcon id={b.groundIds?.[0] ?? 0} size={30} crisp />
								<div class="lb-map-info">
									<div class="lb-map-head">
										<input
											class="t-input lb-name-in"
											value={b.name}
											oninput={(e) => (mapBorders[idx].name = e.currentTarget.value)}
										/>
										<span class="lb-badge" class:full={b.assignedCount >= 8}>
											{b.assignedCount}/12 edges
										</span>
										<span class="lb-badge">{b.align}</span>
										<span class="lb-badge">{Math.round(b.confidence * 100)}%</span>
										<small>{b.groundIds?.length ?? 0} ground ID(s) · {b.contacts} hits</small>
									</div>
									<div class="lb-strip">
										{#each PREVIEW_EDGES as edge}
											{@const eid = b.edges?.[edge] ?? 0}
											{#if eid > 0}
												<span class="lb-mini" title="{edge.toUpperCase()}: #{eid}">
													<ItemIcon id={eid} size={20} crisp />
												</span>
											{/if}
										{/each}
									</div>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			{/if}
		</div>
	{/if}

	{#snippet footer()}
		{#if mode === "selection"}
			<button class="t-btn" disabled={saving || assignedCount === 0} onclick={exportXML}>Export RME XML…</button>
			<span class="grow"></span>
			<button class="t-btn primary" disabled={saving || assignedCount === 0} onclick={saveToEditor}>
				{saving ? "Saving…" : "Save to NexaMap (Active now)"}
			</button>
		{:else}
			<button class="t-btn" disabled={saving || chosenMapBorders.length === 0} onclick={exportAllMapBordersXML}>
				Export {chosenMapBorders.length} as RME XML…
			</button>
			<span class="grow"></span>
			<button
				class="t-btn primary"
				disabled={saving || chosenMapBorders.length === 0}
				onclick={saveAllMapBorders}
			>
				{saving ? "Applying…" : `Apply ${chosenMapBorders.length} to NexaMap (Active now)`}
			</button>
		{/if}
		<button class="t-btn" onclick={close}>Close</button>
	{/snippet}
</Dialog>

<style>
	.lb-tabs {
		display: flex;
		gap: 6px;
		margin-bottom: 10px;
	}
	.lb-tab {
		flex: 1;
		padding: 6px 10px;
		border-radius: 6px;
		border: 1px solid var(--border, #2c3139);
		background: rgba(255, 255, 255, 0.03);
		color: inherit;
		font: inherit;
		font-size: 12px;
		cursor: pointer;
	}
	.lb-tab.on {
		border-color: var(--accent, #7c4df2);
		background: color-mix(in srgb, var(--accent, #7c4df2) 24%, transparent);
		color: #fff;
		font-weight: 500;
	}
	.lb-tab:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	.lb-wrap {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.lb-top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		flex-wrap: wrap;
	}
	.lb-meta,
	.lb-note {
		font-size: 12px;
		opacity: 0.82;
		line-height: 1.4;
	}
	.lb-check,
	.lb-inline {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		cursor: pointer;
	}
	.lb-minsel {
		width: 112px;
	}
	.lb-main {
		display: grid;
		grid-template-columns: 280px 1fr;
		gap: 12px;
	}
	.lb-grids,
	.lb-side {
		display: flex;
		flex-direction: column;
		gap: 8px;
		min-width: 0;
	}
	.lb-sub {
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		opacity: 0.72;
	}
	.lb-grid3 {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 5px;
	}
	.lb-grid4 {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 5px;
	}
	.lb-slot,
	.lb-center {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 2px;
		min-height: 66px;
		padding: 4px;
		border-radius: 6px;
		border: 1px solid var(--border, #2c3139);
		background: rgba(0, 0, 0, 0.25);
		color: inherit;
	}
	.lb-slot {
		cursor: pointer;
	}
	.lb-slot:hover {
		border-color: var(--accent, #7c4df2);
	}
	.lb-slot.on {
		border-color: var(--accent, #7c4df2);
		background: color-mix(in srgb, var(--accent, #7c4df2) 22%, transparent);
	}
	.lb-slot.warn {
		border-color: #eab308;
	}
	.lb-slot small,
	.lb-center span {
		font-size: 10px;
		opacity: 0.75;
	}
	.lb-id {
		font-size: 10px;
		color: var(--gold, #e9b46a);
	}
	.lb-dash {
		font-size: 16px;
		opacity: 0.35;
	}
	.lb-box {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 8px;
		border: 1px solid var(--border, #2c3139);
		border-radius: 8px;
	}
	.lb-box h4 {
		margin: 0;
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		opacity: 0.8;
	}
	.lb-field {
		display: flex;
		flex-direction: column;
		gap: 3px;
		font-size: 11.5px;
	}
	.lb-row {
		display: flex;
		gap: 6px;
		align-items: center;
	}
	.lb-row2 {
		display: grid;
		grid-template-columns: 92px 1fr 80px;
		gap: 6px;
	}
	.lb-cands {
		display: flex;
		flex-direction: column;
		gap: 3px;
		max-height: 150px;
		overflow-y: auto;
	}
	.lb-cand {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 4px 6px;
		border-radius: 5px;
		border: 1px solid transparent;
		background: rgba(255, 255, 255, 0.03);
		color: inherit;
		cursor: pointer;
		text-align: left;
		font-size: 11.5px;
	}
	.lb-cand.on {
		border-color: var(--accent, #7c4df2);
		background: color-mix(in srgb, var(--accent, #7c4df2) 18%, transparent);
	}
	.lb-cand small {
		opacity: 0.7;
		font-size: 10.5px;
	}
	.lb-map-list {
		display: flex;
		flex-direction: column;
		gap: 5px;
		max-height: 410px;
		overflow-y: auto;
		padding: 4px;
		border: 1px solid var(--border, #2c3139);
		border-radius: 8px;
		background: rgba(0, 0, 0, 0.22);
	}
	.lb-map-row {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 6px 8px;
		border-radius: 6px;
		border: 1px solid transparent;
		background: rgba(255, 255, 255, 0.025);
	}
	.lb-map-row.on {
		border-color: color-mix(in srgb, var(--accent, #7c4df2) 55%, transparent);
		background: color-mix(in srgb, var(--accent, #7c4df2) 10%, transparent);
	}
	.lb-map-info {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.lb-map-head {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
	}
	.lb-name-in {
		width: 210px;
		font-size: 12px;
		padding: 2px 6px;
	}
	.lb-badge {
		font-size: 10.5px;
		padding: 1px 6px;
		border-radius: 4px;
		background: rgba(255, 255, 255, 0.08);
	}
	.lb-badge.full {
		background: rgba(34, 197, 94, 0.22);
		color: #86efac;
	}
	.lb-map-head small {
		font-size: 10.5px;
		opacity: 0.7;
	}
	.lb-strip {
		display: flex;
		gap: 3px;
		flex-wrap: wrap;
	}
	.lb-mini {
		display: inline-grid;
		place-items: center;
		width: 24px;
		height: 24px;
		border-radius: 4px;
		background: rgba(0, 0, 0, 0.35);
		border: 1px solid rgba(255, 255, 255, 0.07);
	}
	.lb-empty {
		padding: 28px 14px;
		text-align: center;
		opacity: 0.8;
	}
	.lb-err {
		color: #f87171;
	}
</style>
