<script lang="ts">
	import { onDestroy, type Snippet } from "svelte";
	import { backdrop, pop } from "./motion";

	let {
		title,
		width = 420,
		full = false,
		onclose,
		children,
		footer,
	}: {
		title: string;
		width?: number;
		full?: boolean;
		onclose: () => void;
		children: Snippet;
		footer?: Snippet;
	} = $props();

	let box: HTMLDivElement;
	let dx = $state(0);
	let dy = $state(0);
	let dragging = $state(false);

	let dragState: {
		startX: number;
		startY: number;
		originDx: number;
		originDy: number;
		minDx: number;
		maxDx: number;
		minDy: number;
		maxDy: number;
	} | null = null;

	function onPointerMove(event: PointerEvent) {
		if (!dragState) return;
		const nextX = dragState.originDx + (event.clientX - dragState.startX);
		const nextY = dragState.originDy + (event.clientY - dragState.startY);
		dx = Math.max(dragState.minDx, Math.min(dragState.maxDx, nextX));
		dy = Math.max(dragState.minDy, Math.min(dragState.maxDy, nextY));
	}

	function stopDrag() {
		if (!dragState) return;
		dragState = null;
		dragging = false;
		window.removeEventListener("pointermove", onPointerMove);
		window.removeEventListener("pointerup", stopDrag);
		window.removeEventListener("pointercancel", stopDrag);
	}

	function startDrag(event: PointerEvent) {
		if (event.button !== 0 || !box) return;
		const target = event.target as HTMLElement | null;
		if (target?.closest("button, input, select, a")) return;
		event.preventDefault();
		const rect = box.getBoundingClientRect();
		const vw = window.innerWidth;
		const vh = window.innerHeight;
		const keepVisibleX = Math.min(120, Math.max(60, rect.width * 0.25));
		dragState = {
			startX: event.clientX,
			startY: event.clientY,
			originDx: dx,
			originDy: dy,
			minDx: dx + (keepVisibleX - rect.right),
			maxDx: dx + (vw - keepVisibleX - rect.left),
			minDy: dy + (8 - rect.top),
			maxDy: dy + (vh - 44 - rect.top),
		};
		dragging = true;
		window.addEventListener("pointermove", onPointerMove);
		window.addEventListener("pointerup", stopDrag);
		window.addEventListener("pointercancel", stopDrag);
	}

	onDestroy(stopDrag);

	$effect(() => {
		// A disabled control cannot take the focus: without one that can, the
		// window itself does, so keys (Escape) reach it and not what is behind.
		const first = box.querySelector<HTMLElement>(
			"input:not(:disabled), select:not(:disabled), button.primary:not(:disabled), button:not(:disabled)",
		);
		(first ?? box).focus();
	});

	function onkeydown(event: KeyboardEvent) {
		if (event.key === "Escape") {
			event.stopPropagation();
			onclose();
		}
	}

	const dialogStyle = $derived.by(() => {
		const parts: string[] = [];
		if (!full) parts.push(`width:${width}px`);
		if (dx !== 0 || dy !== 0) parts.push(`transform:translate3d(${dx}px, ${dy}px, 0)`);
		return parts.length ? parts.join(";") : undefined;
	});
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="overlay"
	transition:backdrop|global
	{onkeydown}
	onmousedown={(event) => event.target === event.currentTarget && !dragging && onclose()}
>
	<div
		class="t-window dialog"
		class:full
		class:dragging
		transition:pop|global
		style={dialogStyle}
		bind:this={box}
		tabindex="-1"
		role="dialog"
		aria-modal="true"
		aria-label={title}
	>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="t-window-title drag-handle" title="Drag to move window" onpointerdown={startDrag}>
			{title}
		</div>
		<div class="content">
			{@render children()}
		</div>
		{#if footer}
			<div class="t-sep"></div>
			<div class="footer">{@render footer()}</div>
		{/if}
	</div>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		z-index: 50;
		display: flex;
		align-items: center;
		justify-content: center;
		background: rgba(0, 0, 0, 0.45);
	}
	.dialog {
		max-height: calc(100vh - 40px);
		padding: 10px 12px 10px;
		box-shadow: 0 10px 40px rgba(0, 0, 0, 0.6);
	}
	.dialog.full {
		width: calc(100vw - 24px);
		height: calc(100vh - 24px);
		max-width: none;
		max-height: none;
		display: flex;
		flex-direction: column;
	}
	.drag-handle {
		cursor: grab;
		user-select: none;
		touch-action: none;
	}
	.dialog.dragging .drag-handle {
		cursor: grabbing;
	}
	.content {
		overflow: auto;
		min-height: 0;
	}
	.dialog.full .content {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.footer {
		display: flex;
		justify-content: flex-end;
		gap: 6px;
	}
</style>
