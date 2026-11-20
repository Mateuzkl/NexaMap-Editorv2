<script lang="ts">
	import type { Snippet } from "svelte";
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
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="overlay"
	transition:backdrop|global
	{onkeydown}
	onmousedown={(event) => event.target === event.currentTarget && onclose()}
>
	<div
		class="t-window dialog"
		class:full
		transition:pop|global
		style={full ? undefined : `width:${width}px`}
		bind:this={box}
		tabindex="-1"
		role="dialog"
		aria-modal="true"
		aria-label={title}
	>
		<div class="t-window-title">{title}</div>
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
