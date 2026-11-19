<script lang="ts">
	import { onDestroy, onMount, tick } from "svelte";
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

	type ViewMode = "split" | "result" | "source";

	let suggestions = $state<HuntSuggestion[]>([]);
	let search = $state("");
	let detecting = $state(false);
	let analyzing = $state(false);
	let generating = $state(false);
	let placing = $state(false);

	let selectedName = $state("");
	let x1 = $state(Math.max(0, Math.round(map.cam.x) - 36));
	let y1 = $state(Math.max(0, Math.round(map.cam.y) - 36));
	let x2 = $state(Math.min(65535, Math.round(map.cam.x) + 36));
	let y2 = $state(Math.min(65535, Math.round(map.cam.y) + 36));
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
	let viewMode = $state<ViewMode>("split");
	let zoom = $state(1);

	let srcNatW = $state(0);
	let srcNatH = $state(0);
	let resNatW = $state(0);
	let resNatH = $state(0);

	let srcCanvasEl = $state<HTMLDivElement | null>(null);
	let resCanvasEl = $state<HTMLDivElement | null>(null);
	let srcImgEl = $state<HTMLImageElement | null>(null);

	let destX = $state(Math.max(100, Math.round(map.cam.x) + 120));
	let destY = $state(Math.max(100, Math.round(map.cam.y)));
	let destZ = $state(map.cam.floor);
	let replaceTiles = $state(true);

	// Interactive pan & Shift+drag visual crop state
	let panning = $state(false);
	let cropBox = $state<{ x: number; y: number; w: number; h: number; tiles: string } | null>(null);
	let detachDrag: (() => void) | null = null;

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

	function clampZoom(v: number): number {
		return Math.min(4, Math.max(0.15, Number(v.toFixed(3))));
	}

	async function fitPreview(prefer: "source" | "result" = "result") {
		await tick();
		const target =
			prefer === "result" && resNatW > 0 && resCanvasEl
				? { el: resCanvasEl, w: resNatW, h: resNatH }
				: srcNatW > 0 && srcCanvasEl
					? { el: srcCanvasEl, w: srcNatW, h: srcNatH }
					: resNatW > 0 && resCanvasEl
						? { el: resCanvasEl, w: resNatW, h: resNatH }
						: null;
		if (!target) return;
		const availW = Math.max(160, target.el.clientWidth - 20);
		const availH = Math.max(160, target.el.clientHeight - 20);
		const fit = Math.min(availW / target.w, availH / target.h);
		// Cap initial auto-fit at 2.5x so tiny hunts look crisp without blowing up excessively.
		zoom = clampZoom(Math.min(2.5, fit));
	}

	function handleWheel(event: WheelEvent) {
		event.preventDefault();
		const el = event.currentTarget as HTMLDivElement;
		const rect = el.getBoundingClientRect();
		const mx = event.clientX - rect.left + el.scrollLeft;
		const my = event.clientY - rect.top + el.scrollTop;
		const factor = event.deltaY < 0 ? 1.18 : 1 / 1.18;
		const prev = zoom;
		const next = clampZoom(prev * factor);
		if (next === prev) return;
		zoom = next;
		const ratio = next / prev;
		requestAnimationFrame(() => {
			el.scrollLeft = mx * ratio - (event.clientX - rect.left);
			el.scrollTop = my * ratio - (event.clientY - rect.top);
		});
	}

	function clearDragListeners() {
		if (detachDrag) {
			detachDrag();
			detachDrag = null;
		}
		panning = false;
		cropBox = null;
	}

	function startCanvasMouseDown(event: MouseEvent, kind: "source" | "result") {
		if (event.button !== 0) return;
		const el = event.currentTarget as HTMLDivElement;

		// Shift + drag on Original preview visually crops (x1, y1 -> x2, y2) and re-analyzes.
		if (kind === "source" && event.shiftKey && analysisView && srcImgEl) {
			event.preventDefault();
			clearDragListeners();
			const img = srcImgEl;
			const imgRect = img.getBoundingClientRect();
			const canvasRect = el.getBoundingClientRect();
			const reg = analysisView.region;
			const cols = Math.max(1, analysisView.summary.width);
			const rows = Math.max(1, analysisView.summary.height);

			const clampPxX = (clientX: number) => Math.min(imgRect.right, Math.max(imgRect.left, clientX));
			const clampPxY = (clientY: number) => Math.min(imgRect.bottom, Math.max(imgRect.top, clientY));

			const startClientX = clampPxX(event.clientX);
			const startClientY = clampPxY(event.clientY);

			const computeBox = (curX: number, curY: number) => {
				const lx = Math.min(startClientX, curX);
				const rx = Math.max(startClientX, curX);
				const ty = Math.min(startClientY, curY);
				const by = Math.max(startClientY, curY);
				const tx1 = Math.max(0, Math.min(cols - 1, Math.floor(((lx - imgRect.left) / imgRect.width) * cols)));
				const tx2 = Math.max(tx1, Math.min(cols - 1, Math.floor(((rx - imgRect.left) / imgRect.width) * cols)));
				const ty1 = Math.max(0, Math.min(rows - 1, Math.floor(((ty - imgRect.top) / imgRect.height) * rows)));
				const ty2 = Math.max(ty1, Math.min(rows - 1, Math.floor(((by - imgRect.top) / imgRect.height) * rows)));
				return {
					x: lx - canvasRect.left + el.scrollLeft,
					y: ty - canvasRect.top + el.scrollTop,
					w: Math.max(2, rx - lx),
					h: Math.max(2, by - ty),
					tx1,
					ty1,
					tx2,
					ty2,
					tiles: `${tx2 - tx1 + 1}×${ty2 - ty1 + 1} tiles`,
				};
			};

			const onMove = (ev: MouseEvent) => {
				const box = computeBox(clampPxX(ev.clientX), clampPxY(ev.clientY));
				cropBox = { x: box.x, y: box.y, w: box.w, h: box.h, tiles: box.tiles };
			};

			const onUp = (ev: MouseEvent) => {
				const box = computeBox(clampPxX(ev.clientX), clampPxY(ev.clientY));
				clearDragListeners();
				if (box.tx2 - box.tx1 >= 4 && box.ty2 - box.ty1 >= 4) {
					x1 = reg.x1 + box.tx1;
					y1 = reg.y1 + box.ty1;
					x2 = reg.x1 + box.tx2;
					y2 = reg.y1 + box.ty2;
					void analyzeCurrent();
				}
			};

			window.addEventListener("mousemove", onMove);
			window.addEventListener("mouseup", onUp);
			detachDrag = () => {
				window.removeEventListener("mousemove", onMove);
				window.removeEventListener("mouseup", onUp);
			};
			return;
		}

		// Standard left-drag pans the preview smoothly.
		event.preventDefault();
		clearDragListeners();
		panning = true;
		const startX = event.clientX;
		const startY = event.clientY;
		const startLeft = el.scrollLeft;
		const startTop = el.scrollTop;

		const onMove = (ev: MouseEvent) => {
			el.scrollLeft = startLeft - (ev.clientX - startX);
			el.scrollTop = startTop - (ev.clientY - startY);
		};
		const onUp = () => clearDragListeners();

		window.addEventListener("mousemove", onMove);
		window.addEventListener("mouseup", onUp);
		detachDrag = () => {
			window.removeEventListener("mousemove", onMove);
			window.removeEventListener("mouseup", onUp);
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

	async function useAreaAround(cx: number, cy: number, cz: number, label: string) {
		const half = 36;
		x1 = Math.max(0, Math.round(cx) - half);
		y1 = Math.max(0, Math.round(cy) - half);
		x2 = Math.min(65535, Math.round(cx) + half);
		y2 = Math.min(65535, Math.round(cy) + half);
		z = cz;
		selectedName = label;
		destX = Math.min(65000, x2 + 24);
		destY = y1;
		destZ = cz;
		await analyzeCurrent();
	}

	function setDestToCamera() {
		destX = Math.max(0, Math.min(65535, Math.round(map.cam.x)));
		destY = Math.max(0, Math.min(65535, Math.round(map.cam.y)));
		destZ = map.cam.floor;
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
			resNatW = 0;
			resNatH = 0;
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

	function onSourceImageLoad(event: Event) {
		const img = event.currentTarget as HTMLImageElement;
		srcNatW = img.naturalWidth;
		srcNatH = img.naturalHeight;
		if (!generateView) void fitPreview("source");
	}

	function onResultImageLoad(event: Event) {
		const img = event.currentTarget as HTMLImageElement;
		resNatW = img.naturalWidth;
		resNatH = img.naturalHeight;
		void fitPreview("result");
	}

	function close() {
		map.dialog = null;
	}

	onMount(() => {
		const anchor = map.huntForgeAnchor;
		map.huntForgeAnchor = null;

		void (async () => {
			await detectHunts();

			if (anchor) {
				const hit = suggestions.find(
					(s) =>
						s.region.z1 === anchor.z &&
						anchor.x >= s.region.x1 &&
						anchor.x <= s.region.x2 &&
						anchor.y >= s.region.y1 &&
						anchor.y <= s.region.y2,
				);
				if (hit) {
					await pickSuggestion(hit);
					return;
				}
				if (map.active?.selection && map.activeId !== null) {
					try {
						const reg = await HuntForgeService.SelectionRegion(map.activeId);
						if (
							anchor.z >= reg.z1 &&
							anchor.z <= reg.z2 &&
							anchor.x >= reg.x1 &&
							anchor.x <= reg.x2 &&
							anchor.y >= reg.y1 &&
							anchor.y <= reg.y2
						) {
							await useMapSelection();
							return;
						}
					} catch {
						/* fall back to area centered at anchor */
					}
				}
				await useAreaAround(anchor.x, anchor.y, anchor.z, `Área (${anchor.x},${anchor.y},z${anchor.z})`);
				return;
			}

			if (map.active?.selection) {
				await useMapSelection();
			} else if (suggestions.length > 0) {
				await pickSuggestion(suggestions[0]);
			}
		})();
	});

	onDestroy(() => {
		clearDragListeners();
		void HuntForgeService.Clear().catch(() => {});
	});
</script>

<Dialog title="Hunt Forge — Estúdio Procedural de Hunts" full={true} onclose={close}>
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
					class="t-btn primary grow"
					disabled={!map.active?.selection || analyzing}
					title={map.active?.selection
						? "Analisar a área atualmente selecionada no mapa"
						: "Selecione uma área no mapa para usar este atalho"}
					onclick={useMapSelection}
				>
					Usar seleção
				</button>
				<button
					class="t-btn"
					disabled={analyzing}
					title="Analisar uma área 72×72 centrada onde a câmera do mapa está agora"
					onclick={() =>
						useAreaAround(
							map.cam.x,
							map.cam.y,
							map.cam.floor,
							`Câmera (${Math.round(map.cam.x)},${Math.round(map.cam.y)},z${map.cam.floor})`,
						)}
				>
					Área da câmera
				</button>
			</div>

			<div class="hf-list t-panel">
				{#if detecting}
					<div class="hf-empty">Detectando hunts no mapa…</div>
				{:else if filtered.length === 0}
					<div class="hf-empty">
						{suggestions.length === 0
							? "Nenhuma hunt com ≥ 8 monstros encontrada. Clique com o botão direito no mapa, use a seleção ou a área manual abaixo."
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

				<div class="hf-seg" role="group" aria-label="Modo de visualização">
					<button
						class="t-btn"
						class:primary={viewMode === "split"}
						onclick={() => {
							viewMode = "split";
							void fitPreview(generateView ? "result" : "source");
						}}
					>
						Lado a lado
					</button>
					<button
						class="t-btn"
						class:primary={viewMode === "result"}
						onclick={() => {
							viewMode = "result";
							void fitPreview("result");
						}}
					>
						Só Resultado
					</button>
					<button
						class="t-btn"
						class:primary={viewMode === "source"}
						onclick={() => {
							viewMode = "source";
							void fitPreview("source");
						}}
					>
						Só Original
					</button>
				</div>

				<button class="t-btn" onclick={() => (zoom = clampZoom(zoom / 1.25))} title="Diminuir zoom">−</button>
				<span class="hf-zoom">{Math.round(zoom * 100)}%</span>
				<button class="t-btn" onclick={() => (zoom = clampZoom(zoom * 1.25))} title="Aumentar zoom">+</button>
				<button class="t-btn" onclick={() => void fitPreview(viewMode === "source" ? "source" : "result")}>
					Ajustar
				</button>
				<button class="t-btn" onclick={() => (zoom = 1)}>100%</button>
				<button class="t-btn" class:primary={showMonsters} onclick={toggleMonsters}>Monstros</button>
			</div>

			<div class="hf-views" class:single={viewMode !== "split"}>
				{#if viewMode !== "result"}
					<div class="hf-pane t-panel">
						<div class="hf-pane-head">
							<span>Original</span>
							<span class="hf-hint">Arraste: mover · Roda: zoom · Shift+arrastar: recortar</span>
						</div>
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<div
							class="hf-canvas"
							class:panning
							bind:this={srcCanvasEl}
							onwheel={handleWheel}
							onmousedown={(e) => startCanvasMouseDown(e, "source")}
						>
							{#if analyzing}
								<div class="hf-empty">Analisando e renderizando sprites…</div>
							{:else if analysisView?.preview}
								<div class="hf-stage">
									<img
										bind:this={srcImgEl}
										src={analysisView.preview}
										alt="Original hunt"
										draggable="false"
										onload={onSourceImageLoad}
										style={srcNatW > 0
											? `width:${Math.max(1, Math.round(srcNatW * zoom))}px;height:${Math.max(1, Math.round(srcNatH * zoom))}px;`
											: undefined}
									/>
								</div>
								{#if cropBox}
									<div
										class="hf-crop"
										style="left:{cropBox.x}px;top:{cropBox.y}px;width:{cropBox.w}px;height:{cropBox.h}px;"
									>
										<span>{cropBox.tiles}</span>
									</div>
								{/if}
							{:else}
								<div class="hf-empty">
									Escolha uma hunt à esquerda, selecione uma área no mapa ou clique com o botão
									direito no mapa.
								</div>
							{/if}
						</div>
					</div>
				{/if}

				{#if viewMode !== "source"}
					<div class="hf-pane t-panel">
						<div class="hf-pane-head">
							<span>Resultado</span>
							<span class="hf-hint">Arraste: mover · Roda: zoom</span>
						</div>
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<div
							class="hf-canvas"
							class:panning
							bind:this={resCanvasEl}
							onwheel={handleWheel}
							onmousedown={(e) => startCanvasMouseDown(e, "result")}
						>
							{#if generating}
								<div class="hf-empty">Gerando hunt e renderizando sprites…</div>
							{:else if generateView?.preview}
								<div class="hf-stage">
									<img
										src={generateView.preview}
										alt="Generated hunt"
										draggable="false"
										onload={onResultImageLoad}
										style={resNatW > 0
											? `width:${Math.max(1, Math.round(resNatW * zoom))}px;height:${Math.max(1, Math.round(resNatH * zoom))}px;`
											: undefined}
									/>
								</div>
							{:else}
								<div class="hf-empty">
									Ajuste o modo à direita e clique em <b>Gerar Hunt</b> para visualizar aqui.
								</div>
							{/if}
						</div>
					</div>
				{/if}
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
				<div class="hf-box-head">
					<h3>Inserir ou Exportar</h3>
					<button
						class="t-btn"
						type="button"
						onclick={setDestToCamera}
						title="Definir destino X, Y, Z para a posição atual da câmera no mapa"
					>
						Usar câmera
					</button>
				</div>
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
			Hunt Forge integrado ao NexaMap Editor · funciona com mapas ServerID, ClientID e 32-bit · botão direito no
			mapa abre direto na área
		</span>
		<button class="t-btn" onclick={close}>Fechar</button>
	{/snippet}
</Dialog>

<style>
	.hf-layout {
		display: grid;
		grid-template-columns: 260px 1fr 315px;
		gap: 10px;
		flex: 1;
		min-height: 0;
		height: 100%;
		padding-top: 4px;
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
	.hf-bar,
	.hf-quick {
		display: flex;
		gap: 6px;
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
	.hf-box-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 6px;
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
		flex-wrap: wrap;
		gap: 6px;
	}
	.hf-seg {
		display: inline-flex;
		gap: 3px;
	}
	.hf-vtitle {
		flex: 1;
		min-width: 140px;
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
		min-width: 40px;
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
	.hf-views.single {
		grid-template-columns: 1fr;
	}
	.hf-pane {
		display: flex;
		flex-direction: column;
		min-height: 0;
		min-width: 0;
		overflow: hidden;
	}
	.hf-pane-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		padding: 6px 10px;
		font-size: 11px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		border-bottom: 1px solid var(--border, #2c3139);
		opacity: 0.85;
	}
	.hf-hint {
		font-weight: 400;
		text-transform: none;
		letter-spacing: normal;
		font-size: 10.5px;
		opacity: 0.65;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.hf-canvas {
		flex: 1;
		min-height: 0;
		overflow: auto;
		background: #0b0e14;
		position: relative;
		padding: 8px;
		cursor: grab;
		user-select: none;
	}
	.hf-canvas.panning {
		cursor: grabbing;
	}
	.hf-stage {
		min-width: 100%;
		min-height: 100%;
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}
	.hf-canvas img {
		display: block;
		image-rendering: pixelated;
		pointer-events: none;
	}
	.hf-crop {
		position: absolute;
		border: 2px dashed #a855f7;
		background: rgba(168, 85, 247, 0.2);
		pointer-events: none;
		box-sizing: border-box;
	}
	.hf-crop span {
		position: absolute;
		top: -20px;
		left: 0;
		font-size: 10.5px;
		font-weight: 600;
		padding: 1px 6px;
		border-radius: 4px;
		background: #7c4df2;
		color: #fff;
		white-space: nowrap;
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
		padding: 6px 6px;
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
