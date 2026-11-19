<script lang="ts">
	import { onDestroy, onMount } from "svelte";
	import { DialogService, errorMessage } from "../../../lib/api";
	import {
		HuntForgeService,
		HuntMode,
		HuntShapeStyle,
		type HuntAnalysisView,
		type HuntGenerateView,
		type HuntRegion,
		type HuntSuggestion,
	} from "../../../lib/map/data/api";
	import { applyChange, map } from "../../../lib/map/state.svelte";
	import { goTo } from "../../../lib/map/view/commands";
	import { toast } from "../../../lib/state/app.svelte";
	import Dialog from "../../../lib/ui/Dialog.svelte";

	const MODE_HELP: Record<HuntMode, string> = {
		[HuntMode.$zero]: "",
		[HuntMode.ModeClone]: "Copia exatamente os tiles e monstros da área selecionada.",
		[HuntMode.ModeExpand]:
			"Mantém a hunt original intacta no centro e cresce novos corredores e salas ao redor (8 direções).",
		[HuntMode.ModeRandom]:
			"Cria uma hunt totalmente nova usando as mesmas paredes, pisos, bordas, salas e densidade de monstros.",
		[HuntMode.ModeMix]:
			"Original no centro com braços planejados (Estrela, Cruz, X, Octógono, Arena, Ramificado, Assimétrico ou Orgânico).",
	};

	const STYLES: { value: HuntShapeStyle; label: string }[] = [
		{ value: HuntShapeStyle.StyleAuto, label: "Automático (sortear pela seed)" },
		{ value: HuntShapeStyle.StyleStar, label: "Estrela 8 pontas" },
		{ value: HuntShapeStyle.StyleCross, label: "Cruz expandida" },
		{ value: HuntShapeStyle.StyleDiagonal, label: "X diagonal" },
		{ value: HuntShapeStyle.StyleOctagon, label: "Octógono (anel)" },
		{ value: HuntShapeStyle.StyleArena, label: "Arena com braços" },
		{ value: HuntShapeStyle.StyleBranches, label: "Ramificado (Y)" },
		{ value: HuntShapeStyle.StyleAsymmetric, label: "Assimétrico" },
		{ value: HuntShapeStyle.StyleOrganic, label: "Orgânico" },
	];

	let suggestions = $state<HuntSuggestion[]>([]);
	let search = $state("");
	let detecting = $state(false);
	let analyzing = $state(false);
	let generating = $state(false);
	let placing = $state(false);

	let selectedName = $state("");
	let x1 = $state(Math.max(0, Math.round(map.cam.x) - 32));
	let y1 = $state(Math.max(0, Math.round(map.cam.y) - 32));
	let x2 = $state(Math.round(map.cam.x) + 32);
	let y2 = $state(Math.round(map.cam.y) + 32);
	let z = $state(map.cam.floor);

	let analysisView = $state<HuntAnalysisView | null>(null);
	let generateView = $state<HuntGenerateView | null>(null);

	let mode = $state<HuntMode>(HuntMode.ModeExpand);
	let areaPercent = $state(175);
	let densityPercent = $state(100);
	let style = $state<HuntShapeStyle>(HuntShapeStyle.StyleAuto);
	let loopChance = $state(-1);
	let seed = $state(Math.floor(Math.random() * 99999) + 1);
	let balanced = $state(true);
	let newSeedEachRun = $state(true);

	let showMonsters = $state(true);
	let zoom = $state(1);

	let destX = $state(Math.max(100, Math.round(map.cam.x) + 120));
	let destY = $state(Math.max(100, Math.round(map.cam.y)));
	let destZ = $state(map.cam.floor);
	let replaceTiles = $state(true);

	const filtered = $derived.by(() => {
		const q = search.trim().toLowerCase();
		if (!q) return suggestions;
		return suggestions.filter(
			(s) =>
				s.name.toLowerCase().includes(q) || (s.species ?? []).some((sp) => sp.name.toLowerCase().includes(q)),
		);
	});

	function currentRegion(): HuntRegion {
		return {
			x1: Number(x1) || 0,
			y1: Number(y1) || 0,
			x2: Number(x2) || 0,
			y2: Number(y2) || 0,
			z1: Number(z) || 0,
			z2: Number(z) || 0,
		};
	}

	async function detectHunts() {
		if (map.activeId === null) return;
		detecting = true;
		try {
			suggestions = (await HuntForgeService.Detect(map.activeId)) ?? [];
		} catch (err) {
			toast(errorMessage(err), "error");
		} finally {
			detecting = false;
		}
	}

	async function useMapSelection() {
		if (map.activeId === null) return;
		try {
			const reg = await HuntForgeService.SelectionRegion(map.activeId);
			x1 = reg.x1;
			y1 = reg.y1;
			x2 = reg.x2;
			y2 = reg.y2;
			z = reg.z1;
			selectedName = `Seleção (${reg.x1},${reg.y1},z${reg.z1})`;
			destX = Math.min(65000, reg.x2 + 24);
			destY = reg.y1;
			destZ = reg.z1;
			await analyzeCurrent();
		} catch (err) {
			toast(errorMessage(err), "error");
		}
	}

	async function pickSuggestion(s: HuntSuggestion) {
		selectedName = s.name;
		x1 = s.region.x1;
		y1 = s.region.y1;
		x2 = s.region.x2;
		y2 = s.region.y2;
		z = s.region.z1;
		destX = Math.min(65000, s.region.x2 + 24);
		destY = s.region.y1;
		destZ = s.region.z1;
		await analyzeCurrent();
	}

	async function analyzeCurrent() {
		if (map.activeId === null) return;
		analyzing = true;
		try {
			const view = await HuntForgeService.Analyze(
				map.activeId,
				currentRegion(),
				selectedName || `hunt_${x1}_${y1}_z${z}`,
				showMonsters,
			);
			analysisView = view;
			generateView = null;
		} catch (err) {
			toast(errorMessage(err), "error");
		} finally {
			analyzing = false;
		}
	}

	async function generateHunt() {
		if (map.activeId === null) return;
		if (newSeedEachRun) seed = Math.floor(Math.random() * 999999) + 1;
		generating = true;
		try {
			const view = await HuntForgeService.Generate(
				map.activeId,
				currentRegion(),
				selectedName || `hunt_${x1}_${y1}_z${z}`,
				{
					mode,
					areaFactor: areaPercent / 100,
					seed: Number(seed) || 1,
					spawnDensity: densityPercent / 100,
					loopChance: Number(loopChance),
					name: selectedName,
					balanced,
					style,
				},
				showMonsters,
			);
			generateView = view;
		} catch (err) {
			toast(errorMessage(err), "error");
		} finally {
			generating = false;
		}
	}

	async function toggleMonsters() {
		showMonsters = !showMonsters;
		if (map.activeId === null) return;
		try {
			if (analysisView) {
				const src = await HuntForgeService.RenderPreview(map.activeId, "source", showMonsters);
				if (src) analysisView = { ...analysisView, preview: src };
			}
			if (generateView) {
				const res = await HuntForgeService.RenderPreview(map.activeId, "result", showMonsters);
				if (res) generateView = { ...generateView, preview: res };
			}
		} catch {
			/* ignore preview refresh error */
		}
	}

	async function placeInMap() {
		if (map.activeId === null || !generateView) return;
		placing = true;
		try {
			const tx = Number(destX) || 0;
			const ty = Number(destY) || 0;
			const tz = Number(destZ) || 0;
			const change = await HuntForgeService.PlaceInMap(map.activeId, tx, ty, tz, replaceTiles);
			applyChange(map.activeId, change);
			goTo({
				x: tx + Math.floor(generateView.result.width / 2),
				y: ty + Math.floor(generateView.result.height / 2),
				z: tz,
			});
			toast(
				`Hunt inserida em (${tx}, ${ty}, ${tz}) — ${change.count.toLocaleString()} tiles alterados. Ctrl+Z desfaz tudo.`,
				"success",
			);
			close();
		} catch (err) {
			toast(errorMessage(err), "error");
		} finally {
			placing = false;
		}
	}

	async function exportOTBM() {
		if (map.activeId === null || !generateView) return;
		const suggested = `${(selectedName || "hunt").replace(/[^a-zA-Z0-9_-]+/g, "_")}_${mode}.otbm`;
		const path = await DialogService.SaveFile("huntForgeExport", "Exportar hunt (.otbm + spawn.xml)", suggested, [
			{ name: "OTBM map (*.otbm)", pattern: "*.otbm" },
		]);
		if (!path) return;
		placing = true;
		try {
			const saved = await HuntForgeService.ExportOTBM(
				map.activeId,
				path,
				Number(destX) || 1000,
				Number(destY) || 1000,
				Number(destZ) || 7,
			);
			toast(`Hunt exportada com sucesso: ${saved}`, "success");
		} catch (err) {
			toast(errorMessage(err), "error");
		} finally {
			placing = false;
		}
	}

	function close() {
		map.dialog = null;
	}

	onMount(() => {
		void (async () => {
			await detectHunts();
			if (map.active?.selection) {
				await useMapSelection();
			} else if (suggestions.length > 0) {
				await pickSuggestion(suggestions[0]);
			}
		})();
	});

	onDestroy(() => {
		void HuntForgeService.Clear().catch(() => {});
	});
</script>

<Dialog title="Hunt Forge — Gerador Procedural de Hunts" width={1140} onclose={close}>
	<div class="hf-layout">
		<aside class="hf-left">
			<div class="hf-bar">
				<input class="t-input grow" placeholder="Procurar monstro ou hunt…" bind:value={search} />
				<button class="t-btn" disabled={detecting} onclick={detectHunts} title="Re-escanear monstros do mapa">
					{detecting ? "…" : "↻"}
				</button>
			</div>

			<div class="hf-quick">
				<button
					class="t-btn primary full"
					disabled={!map.active?.selection || analyzing}
					title={map.active?.selection
						? "Analisar a área atualmente selecionada no mapa"
						: "Selecione uma área no mapa para usar este atalho"}
					onclick={useMapSelection}
				>
					Usar seleção do mapa
				</button>
			</div>

			<div class="hf-list t-panel">
				{#if detecting}
					<div class="hf-empty">Detectando hunts no mapa…</div>
				{:else if filtered.length === 0}
					<div class="hf-empty">
						{suggestions.length === 0
							? "Nenhuma hunt com ≥ 8 monstros encontrada. Use a área manual abaixo ou a seleção do mapa."
							: "Nenhum resultado para o filtro."}
					</div>
				{:else}
					{#each filtered as s (s.name + s.region.x1 + "_" + s.region.y1 + "_" + s.region.z1)}
						<button
							class="hf-item"
							class:on={selectedName === s.name &&
								x1 === s.region.x1 &&
								y1 === s.region.y1 &&
								z === s.region.z1}
							onclick={() => pickSuggestion(s)}
						>
							<b>{s.name}</b>
							<small>
								{s.creatures} monstros · z{s.region.z1} · {s.region.x2 - s.region.x1 + 1}×{s.region.y2 -
									s.region.y1 +
									1}
							</small>
						</button>
					{/each}
				{/if}
			</div>

			<div class="hf-manual">
				<div class="hf-sub">Área manual (x1, y1 → x2, y2, z)</div>
				<div class="hf-grid5">
					<label>x1<input class="t-input" type="number" bind:value={x1} /></label>
					<label>y1<input class="t-input" type="number" bind:value={y1} /></label>
					<label>x2<input class="t-input" type="number" bind:value={x2} /></label>
					<label>y2<input class="t-input" type="number" bind:value={y2} /></label>
					<label>z<input class="t-input" type="number" min="0" max="15" bind:value={z} /></label>
				</div>
				<button class="t-btn full" disabled={analyzing} onclick={analyzeCurrent}>
					{analyzing ? "Analisando…" : "Analisar esta área"}
				</button>
			</div>
		</aside>

		<section class="hf-center">
			<div class="hf-vbar">
				<span class="hf-vtitle">
					{selectedName || "Selecione ou analise uma hunt"}
					{#if analysisView}
						<small>
							· Orig {analysisView.summary.width}×{analysisView.summary.height}
							{#if generateView}
								→ Res {generateView.result.width}×{generateView.result.height}
							{/if}
						</small>
					{/if}
				</span>
				<button class="t-btn" onclick={() => (zoom = Math.max(0.5, +(zoom - 0.25).toFixed(2)))}>−</button>
				<span class="hf-zoom">{Math.round(zoom * 100)}%</span>
				<button class="t-btn" onclick={() => (zoom = Math.min(3, +(zoom + 0.25).toFixed(2)))}>+</button>
				<button class="t-btn" onclick={() => (zoom = 1)}>100%</button>
				<button class="t-btn" class:primary={showMonsters} onclick={toggleMonsters}>Monstros</button>
			</div>

			<div class="hf-views">
				<div class="hf-pane t-panel">
					<div class="hf-pane-head">Original</div>
					<div class="hf-canvas">
						{#if analyzing}
							<div class="hf-empty">Analisando e renderizando…</div>
						{:else if analysisView?.preview}
							<img
								src={analysisView.preview}
								alt="Original hunt"
								style="transform: scale({zoom}); transform-origin: top left;"
							/>
						{:else}
							<div class="hf-empty">Escolha uma hunt à esquerda ou selecione no mapa.</div>
						{/if}
					</div>
				</div>

				<div class="hf-pane t-panel">
					<div class="hf-pane-head">Resultado</div>
					<div class="hf-canvas">
						{#if generating}
							<div class="hf-empty">Gerando hunt…</div>
						{:else if generateView?.preview}
							<img
								src={generateView.preview}
								alt="Generated hunt"
								style="transform: scale({zoom}); transform-origin: top left;"
							/>
						{:else}
							<div class="hf-empty">Clique em <b>Gerar Hunt</b> à direita para visualizar.</div>
						{/if}
					</div>
				</div>
			</div>
		</section>

		<aside class="hf-right">
			<div class="hf-box">
				<h3>Análise</h3>
				{#if analysisView}
					<div class="hf-kv">
						<span>Área andável</span><span>{analysisView.summary.walkArea.toLocaleString()} tiles</span>
						<span>Corredor / Salas</span><span
							>{analysisView.summary.corridorWidth} tiles · {analysisView.summary.rooms} salas</span
						>
						<span>Trechos / Voltas</span><span
							>{analysisView.summary.runs} trechos · {analysisView.summary.loopsPerRoom.toFixed(
								2,
							)}/sala</span
						>
						<span>Criaturas</span><span>{analysisView.summary.creatures}</span>
					</div>
					{#if analysisView.summary.species?.length}
						<div class="hf-pills">
							{#each analysisView.summary.species.slice(0, 6) as sp}
								<span>{sp.name} ({sp.count})</span>
							{/each}
						</div>
					{/if}
				{:else}
					<div class="hf-note">Nenhuma hunt analisada ainda.</div>
				{/if}
			</div>

			<div class="hf-box">
				<h3>Gerar</h3>
				<div class="hf-modes">
					<button class:on={mode === HuntMode.ModeClone} onclick={() => (mode = HuntMode.ModeClone)}
						>Clonar</button
					>
					<button class:on={mode === HuntMode.ModeExpand} onclick={() => (mode = HuntMode.ModeExpand)}
						>Expandir</button
					>
					<button class:on={mode === HuntMode.ModeRandom} onclick={() => (mode = HuntMode.ModeRandom)}
						>Nova no estilo</button
					>
					<button class:on={mode === HuntMode.ModeMix} onclick={() => (mode = HuntMode.ModeMix)}
						>Expandir + Estilo</button
					>
				</div>
				<div class="hf-note">{MODE_HELP[mode]}</div>

				{#if mode !== HuntMode.ModeClone}
					<label class="hf-slider">
						<span>Tamanho: <b>{areaPercent}%</b> da área original</span>
						<input type="range" min="100" max="300" step="5" bind:value={areaPercent} />
					</label>
				{/if}

				<label class="hf-slider">
					<span>Monstros: <b>{densityPercent}%</b> da densidade</span>
					<input type="range" min="0" max="200" step="10" bind:value={densityPercent} />
				</label>

				{#if mode === HuntMode.ModeMix}
					<label class="hf-field">
						<span>Estilo do desenho</span>
						<select class="t-input" bind:value={style}>
							{#each STYLES as opt}
								<option value={opt.value}>{opt.label}</option>
							{/each}
						</select>
					</label>
				{/if}

				<div class="hf-row2">
					<label class="hf-field">
						<span>Voltas</span>
						<select class="t-input" bind:value={loopChance}>
							<option value={-1}>Igual ao original</option>
							<option value={0.1}>Poucas</option>
							<option value={0.4}>Médias</option>
							<option value={0.8}>Muitas</option>
						</select>
					</label>
					<label class="hf-field">
						<span>Seed</span>
						<input class="t-input" type="number" bind:value={seed} />
					</label>
				</div>

				{#if mode === HuntMode.ModeExpand || mode === HuntMode.ModeRandom}
					<label class="hf-check">
						<input class="t-check" type="checkbox" bind:checked={balanced} />
						Crescer equilibrado nos 8 lados
					</label>
				{/if}
				<label class="hf-check">
					<input class="t-check" type="checkbox" bind:checked={newSeedEachRun} />
					Nova seed a cada geração
				</label>

				<button class="t-btn primary full" disabled={!analysisView || generating} onclick={generateHunt}>
					{generating ? "Gerando…" : "Gerar Hunt"}
				</button>

				{#if generateView}
					<div class="hf-report">{generateView.result.report}</div>
					{#if generateView.result.species?.length}
						<div class="hf-pills">
							{#each generateView.result.species.slice(0, 6) as sp}
								<span>{sp.name} ({sp.count})</span>
							{/each}
						</div>
					{/if}
				{/if}
			</div>

			<div class="hf-box">
				<h3>Inserir ou Exportar</h3>
				<div class="hf-grid3">
					<label>X<input class="t-input" type="number" bind:value={destX} /></label>
					<label>Y<input class="t-input" type="number" bind:value={destY} /></label>
					<label>Z<input class="t-input" type="number" min="0" max="15" bind:value={destZ} /></label>
				</div>
				<label class="hf-check">
					<input class="t-check" type="checkbox" bind:checked={replaceTiles} />
					Substituir tiles existentes no destino
				</label>
				<button class="t-btn primary full" disabled={!generateView || placing} onclick={placeInMap}>
					{placing ? "Aplicando…" : "Colocar direto no mapa (Ctrl+Z)"}
				</button>
				<button class="t-btn full" disabled={!generateView || placing} onclick={exportOTBM}>
					Exportar .otbm + -spawn.xml…
				</button>
			</div>
		</aside>
	</div>

	{#snippet footer()}
		<span class="t-label grow">
			Hunt Forge integrado ao NexaMap Editor · funciona com mapas ServerID, ClientID e 32-bit
		</span>
		<button class="t-btn" onclick={close}>Fechar</button>
	{/snippet}
</Dialog>

<style>
	.hf-layout {
		display: grid;
		grid-template-columns: 245px 1fr 295px;
		gap: 10px;
		height: min(74vh, 660px);
		min-height: 480px;
	}
	.hf-left,
	.hf-center,
	.hf-right {
		display: flex;
		flex-direction: column;
		gap: 8px;
		min-height: 0;
		min-width: 0;
	}
	.hf-right {
		overflow-y: auto;
		padding-right: 2px;
	}
	.hf-bar {
		display: flex;
		gap: 6px;
	}
	.hf-quick {
		display: flex;
	}
	.full {
		width: 100%;
	}
	.hf-list {
		flex: 1;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		padding: 4px;
		gap: 2px;
	}
	.hf-item {
		text-align: left;
		background: transparent;
		border: 1px solid transparent;
		border-radius: 6px;
		padding: 6px 8px;
		cursor: pointer;
		color: inherit;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.hf-item:hover {
		background: color-mix(in srgb, var(--accent, #7c4df2) 14%, transparent);
	}
	.hf-item.on {
		background: color-mix(in srgb, var(--accent, #7c4df2) 22%, transparent);
		border-color: var(--accent, #7c4df2);
	}
	.hf-item b {
		font-size: 12px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.hf-item small {
		font-size: 11px;
		opacity: 0.72;
	}
	.hf-manual,
	.hf-box {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 8px;
		border: 1px solid var(--border, #2c3139);
		border-radius: 8px;
	}
	.hf-box h3 {
		margin: 0;
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		opacity: 0.72;
	}
	.hf-sub,
	.hf-note {
		font-size: 11px;
		opacity: 0.75;
		line-height: 1.4;
	}
	.hf-grid5 {
		display: grid;
		grid-template-columns: repeat(5, 1fr);
		gap: 4px;
	}
	.hf-grid3 {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 5px;
	}
	.hf-row2 {
		display: grid;
		grid-template-columns: 1fr 90px;
		gap: 6px;
	}
	.hf-grid5 label,
	.hf-grid3 label,
	.hf-field {
		display: flex;
		flex-direction: column;
		gap: 2px;
		font-size: 11px;
	}
	.hf-grid5 input,
	.hf-grid3 input {
		width: 100%;
		min-width: 0;
	}
	.hf-vbar {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.hf-vtitle {
		flex: 1;
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.hf-vtitle small {
		font-weight: 400;
		opacity: 0.75;
	}
	.hf-zoom {
		font-size: 11px;
		min-width: 38px;
		text-align: center;
		font-variant-numeric: tabular-nums;
	}
	.hf-views {
		flex: 1;
		min-height: 0;
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 8px;
	}
	.hf-pane {
		display: flex;
		flex-direction: column;
		min-height: 0;
		min-width: 0;
		overflow: hidden;
	}
	.hf-pane-head {
		padding: 5px 8px;
		font-size: 11px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		border-bottom: 1px solid var(--border, #2c3139);
		opacity: 0.8;
	}
	.hf-canvas {
		flex: 1;
		min-height: 0;
		overflow: auto;
		background: #0b0e14;
		position: relative;
		padding: 6px;
	}
	.hf-canvas img {
		display: block;
		image-rendering: pixelated;
	}
	.hf-empty {
		height: 100%;
		display: grid;
		place-items: center;
		text-align: center;
		padding: 14px;
		font-size: 12px;
		opacity: 0.65;
	}
	.hf-kv {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 3px 8px;
		font-size: 11.5px;
	}
	.hf-kv span:nth-child(odd) {
		opacity: 0.7;
	}
	.hf-pills {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}
	.hf-pills span {
		font-size: 10.5px;
		padding: 1px 7px;
		border-radius: 99px;
		border: 1px solid var(--border, #2c3139);
		background: color-mix(in srgb, var(--accent, #7c4df2) 10%, transparent);
	}
	.hf-modes {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 4px;
	}
	.hf-modes button {
		padding: 5px 6px;
		font-size: 11.5px;
		border-radius: 6px;
		border: 1px solid var(--border, #2c3139);
		background: transparent;
		color: inherit;
		cursor: pointer;
	}
	.hf-modes button.on {
		background: var(--accent, #7c4df2);
		border-color: var(--accent, #7c4df2);
		color: #fff;
		font-weight: 600;
	}
	.hf-slider {
		display: flex;
		flex-direction: column;
		gap: 2px;
		font-size: 11.5px;
	}
	.hf-slider input {
		width: 100%;
	}
	.hf-check {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 11.5px;
		cursor: pointer;
	}
	.hf-report {
		font-size: 11px;
		line-height: 1.45;
		padding: 6px 8px;
		border-radius: 6px;
		border: 1px solid var(--border, #2c3139);
		background: rgba(0, 0, 0, 0.2);
	}
</style>
