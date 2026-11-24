<script lang="ts">
	import type { MenuEntry, MenuItem } from "../menu.svelte";
	import MenuEntries from "./MenuEntries.svelte";

	let { items, onpick }: { items: MenuEntry[]; onpick: (item: MenuItem) => void } = $props();
	const checkable = $derived(items.some((entry) => entry !== "-" && !!entry.checked));
	let openSub = $state<number | null>(null);
	let flipLeft = $state(false);

	function showSub(index: number, target: HTMLElement | null) {
		openSub = index;
		if (target) {
			const rect = target.getBoundingClientRect();
			flipLeft = rect.right + 210 > window.innerWidth;
		}
	}

	function marker(entry: MenuItem): string {
		if (!entry.checked) return "";
		const on = !!entry.checked();
		if (entry.radio) return on ? "●" : "○";
		return on ? "✓" : "";
	}
</script>

{#each items as entry, index}
	{#if entry === "-"}
		<div class="t-menu-sep"></div>
	{:else if entry.children?.length}
		{@const disabled = !!entry.disabled?.()}
		<div
			class="sub-wrap"
			role="none"
			onmouseenter={(event) => !disabled && showSub(index, event.currentTarget)}
			onmouseleave={() => openSub === index && (openSub = null)}
		>
			<button
				class="t-menu-entry has-sub"
				class:sub-open={openSub === index}
				role="menuitem"
				aria-haspopup="menu"
				aria-expanded={openSub === index}
				{disabled}
				onclick={(event) => {
					event.stopPropagation();
					if (disabled) return;
					if (openSub === index) openSub = null;
					else showSub(index, event.currentTarget.parentElement);
				}}
			>
				<span
					>{#if checkable}<span class="mark"></span>{/if}{entry.label}</span
				>
				<span class="arrow" aria-hidden="true">▸</span>
			</button>
			{#if openSub === index && !disabled}
				<div class="sub t-menu" class:left={flipLeft} role="menu">
					<MenuEntries items={entry.children} {onpick} />
				</div>
			{/if}
		</div>
	{:else}
		{@const isChecked = !!entry.checked?.()}
		<button
			class="t-menu-entry"
			class:active={entry.checked && isChecked}
			role={entry.checked ? (entry.radio ? "menuitemradio" : "menuitemcheckbox") : "menuitem"}
			aria-checked={entry.checked ? isChecked : undefined}
			disabled={entry.disabled?.()}
			onmouseenter={() => (openSub = null)}
			onclick={() => onpick(entry)}
		>
			<span
				>{#if checkable}<span class="mark" class:on={isChecked} class:radio={entry.radio}>{marker(entry)}</span
					>{/if}{entry.label}</span
			>{#if entry.keys}<kbd>{entry.keys}</kbd>{/if}
		</button>
	{/if}
{/each}

<style>
	.mark {
		display: inline-block;
		width: 15px;
		text-align: center;
		margin-right: 2px;
	}
	.mark.radio {
		opacity: 0.45;
		font-size: 0.85em;
	}
	.mark.on {
		opacity: 1;
		color: var(--ui-primary, var(--gold));
	}
	.t-menu-entry.active {
		color: var(--text-bright, #fff);
		font-weight: 600;
	}
	.sub-wrap {
		position: relative;
		display: flex;
		flex-direction: column;
	}
	.has-sub {
		width: 100%;
	}
	.has-sub.sub-open {
		background: var(--select, #ffffff36);
	}
	.arrow {
		opacity: 0.75;
		margin-left: 8px;
	}
	.sub {
		position: absolute;
		top: -4px;
		left: calc(100% - 2px);
		min-width: 190px;
		z-index: 30;
	}
	.sub.left {
		left: auto;
		right: calc(100% - 2px);
	}
</style>
