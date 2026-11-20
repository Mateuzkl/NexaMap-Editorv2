<script lang="ts">
	import { onMount } from "svelte";
	import { DialogService, errorMessage } from "../../../lib/api";
	import {
		MapService,
		type LearnedBorderReport,
		type LearnedBorderSlot,
		type LearnedBorderTransition,
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

	let loading = $state(true);
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

	async function analyze() {
		if (map.activeId === null) return;
		loading = true;
		error = "";
		try {
			const res = await MapService.LearnBorder(map.activeId);
			report = res;
			if (res?.transitions?.length) {
				applyTransition(0);
			} else if (res?.families?.length) {
				brushName = res.families[0].name;
				edges = {};
			}
		} catch (err) {
			error = errorMessage(err);
		} finally {
			loading = false;
		}
	}

	function setEdge(edge: string, id: number) {
		const next = { ...edges };
		if (id > 0) next[edge] = id;
		else delete next[edge];
		edges = next;
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
			saving = false;
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
			saving = false;
		}
	}

	function close() {
		map.dialog = null;
	}

	onMount(() => {
		void analyze();
	});
</script>

<Dialog title="Learn Border from Selection" width={760} onclose={close}>
	{#if loading}
		<div class="lb-empty">Analyzing selected ground & border tiles…</div>
	{:else if error}
		<div class="lb-empty lb-err">{error}</div>
	{:else if report}
		<div class="lb-wrap">
			<div class="lb-top">
				<div class="lb-meta">
					Floor <b>z{report.floor}</b> · <b>{report.selectedTiles}</b> selected tiles ·
					<b>{families.length}</b> ground {families.length === 1 ? "family" : "families"}
				</div>
				<button class="t-btn" onclick={analyze}>Re-scan selection</button>
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
					No automatic border transitions were detected in this selection (select a ground island surrounded
					by border tiles and neighboring ground/void). You can still assign edges manually below.
				</div>
			{/if}

			<div class="lb-main">
				<div class="lb-grids">
					<div class="lb-sub">Outer / Cardinal & Corner Edges (click any slot to edit)</div>
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
										<span class="grow">{map.items?.name(cand.itemId) || `Item ${cand.itemId}`}</span
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

	{#snippet footer()}
		<button class="t-btn" disabled={saving || assignedCount === 0} onclick={exportXML}>Export RME XML…</button>
		<span class="grow"></span>
		<button class="t-btn primary" disabled={saving || assignedCount === 0} onclick={saveToEditor}>
			{saving ? "Saving…" : "Save to NexaMap (Active now)"}
		</button>
		<button class="t-btn" onclick={close}>Close</button>
	{/snippet}
</Dialog>

<style>
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
	}
	.lb-meta,
	.lb-note {
		font-size: 12px;
		opacity: 0.82;
		line-height: 1.4;
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
	.lb-empty {
		padding: 28px 14px;
		text-align: center;
		opacity: 0.8;
	}
	.lb-err {
		color: #f87171;
	}
</style>
