<script lang="ts">
	import { onDestroy, onMount } from "svelte";
	import { errorMessage } from "../../../lib/api";
	import { MapService, type QuickReplaceCandidate, type QuickReplaceChoice } from "../../../lib/map/data/api";
	import { edit, map } from "../../../lib/map/state.svelte";
	import { toast } from "../../../lib/state/app.svelte";
	import Dialog from "../../../lib/ui/Dialog.svelte";
	import ItemIcon from "../ItemIcon.svelte";
	import ItemPicker from "../ItemPicker.svelte";

	interface ReplaceRule {
		fromId: number;
		fromName: string;
		toId: number;
		toName: string;
		remove: boolean;
	}

	const CATEGORIES = ["All", "Floor", "Border", "Wall", "Door", "Carpet", "Table", "Doodad", "Item"] as const;

	const selectedTiles = $derived(map.active?.selection ?? 0);
	let inSelection = $state((map.active?.selection ?? 0) > 0);
	let category = $state<string>("All");
	let query = $state("");

	let loading = $state(true);
	let applying = $state(false);
	let error = $state("");
	let candidates = $state<QuickReplaceCandidate[]>([]);

	let selectedId = $state<number>(0);
	let targetId = $state<number>(0);
	let removeMode = $state(false);
	let autoBorder = $state(false);
	let queue = $state<ReplaceRule[]>([]);

	let sameCategory = $state(true);
	let choiceQuery = $state("");
	let choicesLoading = $state(false);
	let choices = $state<QuickReplaceChoice[]>([]);

	let mounted = true;
	let scanSeq = 0;
	let choiceSeq = 0;
	let choiceTimer: ReturnType<typeof setTimeout> | null = null;

	onDestroy(() => {
		mounted = false;
		if (choiceTimer) clearTimeout(choiceTimer);
	});

	const categoryCounts = $derived.by(() => {
		const counts: Record<string, number> = { All: candidates.length };
		for (const c of candidates) {
			counts[c.category] = (counts[c.category] ?? 0) + 1;
		}
		return counts;
	});

	const filtered = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return candidates.filter((c) => {
			if (category !== "All" && c.category !== category) return false;
			if (!q) return true;
			return String(c.id).includes(q) || c.name.toLowerCase().includes(q) || c.category.toLowerCase().includes(q);
		});
	});

	const selected = $derived<QuickReplaceCandidate | null>(
		candidates.find((c) => c.id === selectedId) ?? filtered[0] ?? null,
	);

	const queueByFrom = $derived.by(() => {
		const m = new Map<number, ReplaceRule>();
		for (const r of queue) m.set(r.fromId, r);
		return m;
	});

	const activeBrushItemId = $derived<number>(map.brush?.item || map.brush?.look || 0);

	const canApplySingle = $derived(
		!!selected && !applying && (removeMode || (targetId > 0 && targetId !== selected.id)),
	);

	function itemName(id: number): string {
		if (id <= 0) return "";
		return map.items?.name(id) || `Item #${id}`;
	}

	async function loadChoices(source: QuickReplaceCandidate | null) {
		const id = map.activeId;
		if (id === null || !source) {
			choices = [];
			return;
		}
		const seq = ++choiceSeq;
		choicesLoading = true;
		try {
			const res = await MapService.QuickReplaceChoices(
				id,
				source.id,
				source.category,
				sameCategory,
				choiceQuery,
				180,
			);
			if (!mounted || seq !== choiceSeq) return;
			choices = res ?? [];
		} catch {
			if (!mounted || seq !== choiceSeq) return;
			choices = [];
		} finally {
			if (mounted && seq === choiceSeq) choicesLoading = false;
		}
	}

	function scheduleChoices(immediate = false) {
		if (choiceTimer) {
			clearTimeout(choiceTimer);
			choiceTimer = null;
		}
		if (immediate) {
			void loadChoices(selected);
			return;
		}
		choiceTimer = setTimeout(() => {
			choiceTimer = null;
			void loadChoices(selected);
		}, 120);
	}

	function selectCandidate(c: QuickReplaceCandidate) {
		selectedId = c.id;
		const existing = queueByFrom.get(c.id);
		if (existing) {
			removeMode = existing.remove;
			targetId = existing.remove ? 0 : existing.toId;
		} else {
			removeMode = false;
			targetId = 0;
		}
		void loadChoices(c);
	}

	async function scan(anchorId = 0) {
		const id = map.activeId;
		if (id === null) return;
		const seq = ++scanSeq;
		loading = true;
		error = "";
		try {
			const scope = inSelection && (map.active?.selection ?? 0) > 0;
			if (scope !== inSelection) inSelection = scope;
			const raw = await MapService.QuickReplaceCandidates(id, scope);
			if (!mounted || seq !== scanSeq) return;
			const list = raw ?? [];
			candidates = list;

			const preferred = anchorId > 0 ? list.find((c) => c.id === anchorId) : null;
			const current = list.find((c) => c.id === selectedId);
			const pick = preferred ?? current ?? list[0] ?? null;
			if (pick) {
				selectCandidate(pick);
			} else {
				selectedId = 0;
				targetId = 0;
				removeMode = false;
				choices = [];
			}
		} catch (err) {
			if (!mounted || seq !== scanSeq) return;
			error = errorMessage(err);
		} finally {
			if (mounted && seq === scanSeq) loading = false;
		}
	}

	function setScope(nextInSelection: boolean) {
		if (nextInSelection && selectedTiles === 0) return;
		if (inSelection === nextInSelection) return;
		inSelection = nextInSelection;
		void scan();
	}

	function pickChoice(choice: QuickReplaceChoice) {
		removeMode = false;
		targetId = choice.id;
	}

	function addOrUpdateQueue() {
		if (!selected || !canApplySingle) return;
		const rule: ReplaceRule = {
			fromId: selected.id,
			fromName: selected.name || itemName(selected.id),
			toId: removeMode ? 0 : targetId,
			toName: removeMode ? "Remove" : itemName(targetId),
			remove: removeMode,
		};
		const idx = queue.findIndex((r) => r.fromId === rule.fromId);
		if (idx >= 0) {
			queue = queue.with(idx, rule);
		} else {
			queue = [...queue, rule];
		}

		const nextUnqueued = filtered.find((c) => c.id !== selected.id && !queue.some((r) => r.fromId === c.id));
		if (nextUnqueued) {
			selectCandidate(nextUnqueued);
		}
	}

	function removeRule(fromId: number) {
		queue = queue.filter((r) => r.fromId !== fromId);
	}

	async function runReplace(fromIds: number[], toIds: number[], label: string) {
		const id = map.activeId;
		if (id === null || applying || !fromIds.length) return;
		applying = true;
		try {
			const scope = inSelection && (map.active?.selection ?? 0) > 0;
			const change = await edit((mapId) => MapService.Replace(mapId, fromIds, toIds, scope));
			if (!mounted) return;
			if (!change) return;

			if (autoBorder && change.count > 0) {
				await edit((mapId) => MapService.Tool(mapId, scope ? "borderizeSelection" : "borderizeMap", scope));
				if (!mounted) return;
			}

			toast(
				change.count > 0
					? `${label}: ${change.count.toLocaleString()} item${change.count === 1 ? "" : "s"} updated. (Ctrl+Z undoes)`
					: `${label}: no matching items found.`,
				change.count > 0 ? "success" : "info",
			);

			const replacedSet = new Set(fromIds);
			queue = queue.filter((r) => !replacedSet.has(r.fromId));
			await scan();
		} finally {
			if (mounted) applying = false;
		}
	}

	function replaceSingleNow() {
		if (!selected || !canApplySingle) return;
		const to = removeMode ? 0 : targetId;
		void runReplace([selected.id], [to], removeMode ? `Removed ${selected.name}` : `Replaced ${selected.name}`);
	}

	function applyBatchQueue() {
		if (!queue.length || applying) return;
		const fromIds = queue.map((r) => r.fromId);
		const toIds = queue.map((r) => (r.remove ? 0 : r.toId));
		void runReplace(fromIds, toIds, `Batch replace (${queue.length} rule${queue.length === 1 ? "" : "s"})`);
	}

	function scoreTone(score: number): string {
		if (score >= 80) return "hi";
		if (score >= 60) return "mid";
		return "low";
	}

	function close() {
		map.quickReplaceAnchorItem = 0;
		map.dialog = null;
	}

	onMount(() => {
		const anchor = map.quickReplaceAnchorItem;
		map.quickReplaceAnchorItem = 0;
		void scan(anchor);
	});
</script>

<Dialog title="Quick Replace Items" width={1040} onclose={close}>
	<div class="qr-wrap">
		<div class="qr-topbar">
			<div class="qr-scope" role="group" aria-label="Replace scope">
				<button
					class="qr-pill"
					class:on={inSelection}
					disabled={selectedTiles === 0 || applying}
					onclick={() => setScope(true)}
				>
					Selection ({selectedTiles.toLocaleString()}
					{selectedTiles === 1 ? "tile" : "tiles"})
				</button>
				<button class="qr-pill" class:on={!inSelection} disabled={applying} onclick={() => setScope(false)}>
					Whole map
				</button>
			</div>
			<input
				class="t-input qr-search"
				type="search"
				bind:value={query}
				placeholder="Filter detected items by name or ID…"
			/>
			<button class="t-btn" disabled={loading || applying} onclick={() => scan()}>Re-scan</button>
		</div>

		<div class="qr-cats">
			{#each CATEGORIES as cat}
				{@const count = categoryCounts[cat] ?? 0}
				{#if cat === "All" || count > 0}
					<button class="qr-cat" class:on={category === cat} onclick={() => (category = cat)}>
						{cat}
						<span>{count}</span>
					</button>
				{/if}
			{/each}
		</div>

		{#if loading}
			<div class="qr-empty">Scanning placed items…</div>
		{:else if error}
			<div class="qr-empty qr-err">{error}</div>
		{:else if candidates.length === 0}
			<div class="qr-empty">
				No replaceable items found in {inSelection ? "the current selection" : "this map"}.
			</div>
		{:else}
			<div class="qr-main">
				<div class="qr-list-pane">
					<div class="qr-sub">
						Detected items ({filtered.length}{filtered.length !== candidates.length
							? ` of ${candidates.length}`
							: ""})
					</div>
					<div class="qr-list">
						{#each filtered as cand (cand.id + ":" + cand.category)}
							{@const queued = queueByFrom.get(cand.id)}
							<button
								class="qr-item"
								class:on={selected?.id === cand.id}
								class:queued={!!queued}
								onclick={() => selectCandidate(cand)}
							>
								<ItemIcon id={cand.id} size={28} />
								<div class="qr-item-info">
									<div class="qr-item-name" title={cand.name}>{cand.name}</div>
									<div class="qr-item-meta">
										<span>#{cand.id}</span>
										<span class="qr-tag">{cand.category}</span>
										{#if queued}
											<span class="qr-qtag" class:rm={queued.remove}>
												{queued.remove ? "✕ Remove" : `→ #${queued.toId}`}
											</span>
										{/if}
									</div>
								</div>
								<span class="qr-count">×{cand.count.toLocaleString()}</span>
							</button>
						{/each}
						{#if filtered.length === 0}
							<div class="qr-empty-small">No items match "{query}".</div>
						{/if}
					</div>
				</div>

				<div class="qr-right-pane">
					{#if selected}
						<div class="qr-box">
							<div class="qr-preview-row">
								<div class="qr-card">
									<ItemIcon id={selected.id} size={42} crisp />
									<div class="qr-card-title" title={selected.name}>{selected.name}</div>
									<div class="qr-card-sub">
										#{selected.id} · {selected.category} · ×{selected.count.toLocaleString()}
									</div>
								</div>
								<div class="qr-arrow">→</div>
								<div class="qr-card" class:rm={removeMode} class:empty={!removeMode && !targetId}>
									{#if removeMode}
										<div class="qr-rm-icon">✕</div>
										<div class="qr-card-title">Remove item</div>
										<div class="qr-card-sub">Deletes all occurrences</div>
									{:else if targetId > 0}
										<ItemIcon id={targetId} size={42} crisp />
										<div class="qr-card-title" title={itemName(targetId)}>{itemName(targetId)}</div>
										<div class="qr-card-sub">#{targetId}</div>
									{:else}
										<div class="qr-dash">?</div>
										<div class="qr-card-title">Pick target below</div>
										<div class="qr-card-sub">or mark to remove</div>
									{/if}
								</div>
							</div>

							<div class="qr-controls-row">
								<ItemPicker
									value={removeMode ? 0 : targetId}
									placeholder="Type ID or pick…"
									onchange={(id) => {
										removeMode = false;
										targetId = id;
									}}
								/>
								<button
									class="t-btn"
									class:danger-on={removeMode}
									onclick={() => {
										removeMode = !removeMode;
										if (removeMode) targetId = 0;
									}}
								>
									{removeMode ? "✕ Removing" : "Remove item"}
								</button>
								{#if activeBrushItemId > 0 && activeBrushItemId !== selected.id}
									<button
										class="t-btn"
										onclick={() => {
											removeMode = false;
											targetId = activeBrushItemId;
										}}
										title="Use currently selected brush/item (#{activeBrushItemId})"
									>
										Brush #{activeBrushItemId}
									</button>
								{/if}
								<button
									class="t-btn"
									disabled={!targetId && !removeMode}
									onclick={() => {
										targetId = 0;
										removeMode = false;
									}}
								>
									Clear
								</button>
								<span class="grow"></span>
								<button class="t-btn" disabled={!canApplySingle} onclick={addOrUpdateQueue}>
									{queueByFrom.has(selected.id) ? "Update queue" : "+ Add to queue"}
								</button>
								<button class="t-btn primary" disabled={!canApplySingle} onclick={replaceSingleNow}>
									{applying ? "Replacing…" : "Replace now"}
								</button>
							</div>
						</div>

						<div class="qr-box qr-choices-box">
							<div class="qr-choices-head">
								<h4>
									Matching {sameCategory ? selected.category : "All"} Suggestions ({choices.length})
								</h4>
								<label class="qr-check">
									<input
										type="checkbox"
										bind:checked={sameCategory}
										onchange={() => scheduleChoices(true)}
									/>
									<span>Same category ({selected.category})</span>
								</label>
								<input
									class="t-input qr-choice-search"
									type="search"
									bind:value={choiceQuery}
									oninput={() => scheduleChoices(false)}
									placeholder="Search {sameCategory ? selected.category : 'any'} item…"
								/>
							</div>

							{#if choicesLoading}
								<div class="qr-empty-small">Finding matching {selected.category} items…</div>
							{:else if choices.length === 0}
								<div class="qr-empty-small">
									No matching {sameCategory ? selected.category : ""} items found. Try unchecking
									<b>Same category</b>.
								</div>
							{:else}
								<div class="qr-grid">
									{#each choices as choice (choice.id)}
										<button
											class="qr-cell"
											class:on={!removeMode && targetId === choice.id}
											onclick={() => pickChoice(choice)}
											ondblclick={() => {
												pickChoice(choice);
												addOrUpdateQueue();
											}}
											title="{choice.name} (#{choice.id}) · {choice.category}{choice.brush
												? ` · ${choice.brush}`
												: ''} · {choice.score}% match (double-click to queue)"
										>
											<span class="qr-pct {scoreTone(choice.score)}">{choice.score}%</span>
											<ItemIcon id={choice.id} size={32} crisp />
											<span class="qr-cell-id">{choice.id}</span>
										</button>
									{/each}
								</div>
							{/if}
						</div>
					{/if}

					<div class="qr-box qr-queue-box">
						<div class="qr-queue-head">
							<h4>Batch Queue ({queue.length} {queue.length === 1 ? "rule" : "rules"})</h4>
							{#if queue.length > 0}
								<button class="qr-link" disabled={applying} onclick={() => (queue = [])}>
									Clear all
								</button>
							{/if}
						</div>

						{#if queue.length === 0}
							<div class="qr-queue-empty">
								Click a suggestion above and press <b>Replace now</b>, or double-click /
								<b>+ Add to queue</b> to swap multiple items in one undoable (Ctrl+Z) batch.
							</div>
						{:else}
							<div class="qr-queue-list">
								{#each queue as rule (rule.fromId)}
									<div class="qr-rule">
										<button
											class="qr-rule-main"
											onclick={() => {
												const found = candidates.find((c) => c.id === rule.fromId);
												if (found) selectCandidate(found);
											}}
											title="Click to edit rule"
										>
											<ItemIcon id={rule.fromId} size={22} />
											<span class="qr-rule-name">{rule.fromName}</span>
											<small>#{rule.fromId}</small>
											<span class="qr-rule-arrow">→</span>
											{#if rule.remove}
												<span class="qr-qtag rm">✕ Remove</span>
											{:else}
												<ItemIcon id={rule.toId} size={22} />
												<span class="qr-rule-name">{rule.toName}</span>
												<small>#{rule.toId}</small>
											{/if}
										</button>
										<button
											class="qr-rule-del"
											disabled={applying}
											onclick={() => removeRule(rule.fromId)}
											title="Remove rule"
										>
											✕
										</button>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				</div>
			</div>
		{/if}
	</div>

	{#snippet footer()}
		<label class="qr-check">
			<input type="checkbox" bind:checked={autoBorder} disabled={applying} />
			<span>Auto-border &amp; walls after replace</span>
		</label>
		<span class="grow"></span>
		<button class="t-btn primary" disabled={applying || queue.length === 0} onclick={applyBatchQueue}>
			{applying ? "Applying…" : `Apply Batch (${queue.length} ${queue.length === 1 ? "rule" : "rules"})`}
		</button>
		<button class="t-btn" disabled={applying} onclick={close}>Close</button>
	{/snippet}
</Dialog>

<style>
	.qr-wrap {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.qr-topbar {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
	}
	.qr-scope {
		display: inline-flex;
		border: 1px solid var(--border, #2c3139);
		border-radius: 6px;
		overflow: hidden;
	}
	.qr-pill {
		background: transparent;
		border: none;
		color: inherit;
		font: inherit;
		font-size: 12px;
		padding: 5px 10px;
		cursor: pointer;
		opacity: 0.78;
	}
	.qr-pill:hover:not(:disabled) {
		background: rgba(255, 255, 255, 0.06);
		opacity: 1;
	}
	.qr-pill.on {
		background: color-mix(in srgb, var(--accent, #7c4df2) 28%, transparent);
		color: #fff;
		opacity: 1;
	}
	.qr-pill:disabled {
		opacity: 0.35;
		cursor: not-allowed;
	}
	.qr-search {
		flex: 1;
		min-width: 180px;
	}
	.qr-cats {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}
	.qr-cat {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		padding: 3px 8px;
		border-radius: 5px;
		border: 1px solid var(--border, #2c3139);
		background: rgba(255, 255, 255, 0.02);
		color: inherit;
		font-size: 11.5px;
		cursor: pointer;
	}
	.qr-cat span {
		font-size: 10.5px;
		opacity: 0.65;
	}
	.qr-cat.on {
		border-color: var(--accent, #7c4df2);
		background: color-mix(in srgb, var(--accent, #7c4df2) 22%, transparent);
		color: #fff;
	}
	.qr-main {
		display: grid;
		grid-template-columns: 310px 1fr;
		gap: 10px;
		min-height: 480px;
	}
	.qr-list-pane,
	.qr-right-pane {
		display: flex;
		flex-direction: column;
		gap: 8px;
		min-width: 0;
	}
	.qr-sub {
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		opacity: 0.72;
	}
	.qr-list {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 3px;
		max-height: 510px;
		overflow-y: auto;
		padding: 4px;
		border: 1px solid var(--border, #2c3139);
		border-radius: 8px;
		background: rgba(0, 0, 0, 0.22);
	}
	.qr-item {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 5px 7px;
		border-radius: 6px;
		border: 1px solid transparent;
		background: rgba(255, 255, 255, 0.02);
		color: inherit;
		text-align: left;
		cursor: pointer;
	}
	.qr-item:hover {
		background: rgba(255, 255, 255, 0.06);
	}
	.qr-item.on {
		border-color: var(--accent, #7c4df2);
		background: color-mix(in srgb, var(--accent, #7c4df2) 20%, transparent);
	}
	.qr-item.queued:not(.on) {
		border-color: rgba(34, 197, 94, 0.35);
	}
	.qr-item-info {
		flex: 1;
		min-width: 0;
	}
	.qr-item-name {
		font-size: 12px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.qr-item-meta {
		display: flex;
		align-items: center;
		gap: 5px;
		font-size: 10.5px;
		opacity: 0.75;
		margin-top: 1px;
		flex-wrap: wrap;
	}
	.qr-tag {
		padding: 0 5px;
		border-radius: 4px;
		background: rgba(255, 255, 255, 0.08);
	}
	.qr-qtag {
		padding: 0 5px;
		border-radius: 4px;
		background: rgba(34, 197, 94, 0.22);
		color: #86efac;
		font-weight: 500;
	}
	.qr-qtag.rm {
		background: rgba(239, 68, 68, 0.22);
		color: #fca5a5;
	}
	.qr-count {
		font-size: 11.5px;
		color: var(--gold, #e9b46a);
		font-variant-numeric: tabular-nums;
		flex: none;
	}
	.qr-box {
		display: flex;
		flex-direction: column;
		gap: 7px;
		padding: 9px;
		border: 1px solid var(--border, #2c3139);
		border-radius: 8px;
		background: rgba(0, 0, 0, 0.16);
	}
	.qr-box h4 {
		margin: 0;
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		opacity: 0.8;
	}
	.qr-preview-row {
		display: grid;
		grid-template-columns: 1fr auto 1fr;
		align-items: center;
		gap: 8px;
	}
	.qr-card {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 3px;
		min-height: 84px;
		padding: 6px;
		border-radius: 7px;
		border: 1px solid var(--border, #2c3139);
		background: rgba(0, 0, 0, 0.25);
		text-align: center;
		min-width: 0;
	}
	.qr-card.rm {
		border-color: rgba(239, 68, 68, 0.55);
		background: rgba(239, 68, 68, 0.1);
	}
	.qr-card.empty {
		border-style: dashed;
		opacity: 0.75;
	}
	.qr-card-title {
		font-size: 11.5px;
		font-weight: 500;
		max-width: 100%;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.qr-card-sub {
		font-size: 10px;
		opacity: 0.7;
	}
	.qr-arrow {
		font-size: 18px;
		opacity: 0.65;
	}
	.qr-rm-icon {
		font-size: 24px;
		line-height: 42px;
		color: #f87171;
	}
	.qr-dash {
		font-size: 22px;
		line-height: 42px;
		opacity: 0.4;
	}
	.qr-controls-row {
		display: flex;
		gap: 5px;
		flex-wrap: wrap;
		align-items: center;
	}
	.danger-on {
		border-color: rgba(239, 68, 68, 0.7) !important;
		background: rgba(239, 68, 68, 0.2) !important;
		color: #fecaca !important;
	}
	.qr-choices-head {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
	}
	.qr-choice-search {
		flex: 1;
		min-width: 140px;
	}
	.qr-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(56px, 1fr));
		gap: 4px;
		max-height: 196px;
		overflow-y: auto;
		padding: 3px;
		border: 1px solid var(--border, #2c3139);
		border-radius: 6px;
		background: rgba(0, 0, 0, 0.22);
	}
	.qr-cell {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 2px;
		padding: 13px 2px 3px;
		border-radius: 5px;
		border: 1px solid transparent;
		background: rgba(255, 255, 255, 0.03);
		color: inherit;
		cursor: pointer;
	}
	.qr-cell:hover {
		border-color: rgba(255, 255, 255, 0.2);
		background: rgba(255, 255, 255, 0.08);
	}
	.qr-cell.on {
		border-color: var(--accent, #7c4df2);
		background: color-mix(in srgb, var(--accent, #7c4df2) 25%, transparent);
	}
	.qr-pct {
		position: absolute;
		top: 2px;
		right: 3px;
		font-size: 9px;
		font-weight: 600;
		padding: 0 3px;
		border-radius: 3px;
		line-height: 1.25;
	}
	.qr-pct.hi {
		background: rgba(34, 197, 94, 0.24);
		color: #86efac;
	}
	.qr-pct.mid {
		background: rgba(234, 179, 8, 0.24);
		color: #fde047;
	}
	.qr-pct.low {
		background: rgba(255, 255, 255, 0.09);
		color: rgba(255, 255, 255, 0.7);
	}
	.qr-cell-id {
		font-size: 9.5px;
		opacity: 0.72;
		font-variant-numeric: tabular-nums;
	}
	.qr-queue-box {
		min-height: 100px;
	}
	.qr-queue-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.qr-link {
		background: none;
		border: none;
		color: var(--accent, #a78bfa);
		font-size: 11.5px;
		cursor: pointer;
		padding: 0;
	}
	.qr-link:hover {
		text-decoration: underline;
	}
	.qr-queue-empty {
		font-size: 11.5px;
		opacity: 0.72;
		line-height: 1.4;
		padding: 4px 2px;
	}
	.qr-queue-list {
		display: flex;
		flex-direction: column;
		gap: 4px;
		max-height: 115px;
		overflow-y: auto;
	}
	.qr-rule {
		display: flex;
		align-items: center;
		gap: 4px;
		border: 1px solid var(--border, #2c3139);
		border-radius: 6px;
		background: rgba(255, 255, 255, 0.03);
		padding: 2px 4px;
	}
	.qr-rule-main {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: center;
		gap: 6px;
		background: none;
		border: none;
		color: inherit;
		font: inherit;
		font-size: 11.5px;
		cursor: pointer;
		padding: 3px 4px;
		text-align: left;
	}
	.qr-rule-name {
		max-width: 120px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.qr-rule-main small {
		opacity: 0.65;
		font-size: 10.5px;
	}
	.qr-rule-arrow {
		opacity: 0.65;
	}
	.qr-rule-del {
		background: none;
		border: none;
		color: inherit;
		opacity: 0.6;
		cursor: pointer;
		padding: 4px 6px;
	}
	.qr-rule-del:hover {
		opacity: 1;
		color: #f87171;
	}
	.qr-check {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-size: 11.5px;
		cursor: pointer;
	}
	.qr-empty {
		padding: 36px 14px;
		text-align: center;
		opacity: 0.8;
	}
	.qr-empty-small {
		padding: 16px 8px;
		text-align: center;
		font-size: 12px;
		opacity: 0.72;
	}
	.qr-err {
		color: #f87171;
	}
</style>
