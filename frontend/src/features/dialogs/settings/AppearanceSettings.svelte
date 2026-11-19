<script lang="ts">
	import {
		UI_SCALE,
		currentTheme,
		currentUIScale,
		prefs,
		savePrefs,
		setUIScale,
		setUITheme,
	} from "../../../lib/state/prefs.svelte";
	import { isModernTheme, UI_THEMES, type UITheme } from "../../../lib/theme/theme.svelte";

	const active = $derived(currentTheme());
	const uiScale = $derived(currentUIScale());
	const modern = $derived(isModernTheme(active));

	function choose(theme: UITheme) {
		void setUITheme(theme);
	}
</script>

<h4>Interface Theme</h4>
<p class="t-label intro">
	Switch immediately between the original Classic Tibia look and the Modern Workspace. Your open client, maps, and
	settings stay intact.
</p>

<div class="themes" role="radiogroup" aria-label="Interface theme">
	{#each UI_THEMES as theme (theme.id)}
		{@const selected = active === theme.id}
		<button
			type="button"
			class="theme-card"
			class:selected
			role="radio"
			aria-checked={selected}
			onclick={() => choose(theme.id)}
		>
			<div class="preview preview-{theme.id}" aria-hidden="true">
				{#if theme.id === "classic"}
					<div class="p-bar"></div>
					<div class="p-classic-body">
						<div class="p-classic-win">
							<div class="p-classic-title"></div>
							<div class="p-classic-line"></div>
							<div class="p-classic-btn"></div>
						</div>
						<div class="p-classic-win">
							<div class="p-classic-title"></div>
							<div class="p-classic-line"></div>
							<div class="p-classic-btn"></div>
						</div>
					</div>
				{:else}
					<div class="p-mod-shell">
						<div class="p-mod-side">
							<div class="p-mod-dot"></div>
							<div class="p-mod-nav on"></div>
							<div class="p-mod-nav"></div>
							<div class="p-mod-nav"></div>
						</div>
						<div class="p-mod-main">
							<div class="p-mod-cards">
								<div class="p-mod-card"></div>
								<div class="p-mod-card"></div>
							</div>
							<div class="p-mod-cta"></div>
						</div>
					</div>
				{/if}
			</div>
			<div class="meta">
				<span class="title">
					{theme.label}
					{#if selected}<span class="badge">Active</span>{/if}
				</span>
				<span class="desc">{theme.description}</span>
			</div>
		</button>
	{/each}
</div>

<h4>Text &amp; UI Zoom</h4>
<div class="row zoom-row">
	<button
		type="button"
		class="t-btn"
		disabled={uiScale <= UI_SCALE.min}
		onclick={() => void setUIScale(uiScale - UI_SCALE.step)}>−</button
	>
	<input
		class="zoom-slider"
		type="range"
		min={UI_SCALE.min}
		max={UI_SCALE.max}
		step="5"
		value={uiScale}
		oninput={(event) => void setUIScale(Number(event.currentTarget.value))}
	/>
	<button
		type="button"
		class="t-btn"
		disabled={uiScale >= UI_SCALE.max}
		onclick={() => void setUIScale(uiScale + UI_SCALE.step)}>+</button
	>
	<span class="zoom-pct">{uiScale}%</span>
	<button type="button" class="t-btn" disabled={uiScale === 100} onclick={() => void setUIScale(100)}>Reset</button>
</div>
<p class="t-label intro">
	Adjust overall interface and font scale ({UI_SCALE.min}%–{UI_SCALE.max}%). Also available in the top bar.
</p>

{#if modern}
	<h4>Modern Workspace</h4>
	<label class="row">
		<input
			class="t-check"
			type="checkbox"
			checked={prefs.settings.sidebarCollapsed}
			onchange={(event) => savePrefs({ sidebarCollapsed: event.currentTarget.checked })}
		/>
		Start with the workspace sidebar collapsed
	</label>
{/if}

<style>
	.intro {
		margin: 2px 0 8px;
		line-height: 1.4;
	}
	.themes {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 8px;
		margin-bottom: 10px;
	}
	.theme-card {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 6px;
		background: rgba(0, 0, 0, 0.22);
		border: 1px solid var(--line-dark);
		border-radius: 6px;
		color: var(--text);
		text-align: left;
		cursor: pointer;
		font: inherit;
		transition:
			border-color 0.14s ease,
			background 0.14s ease;
	}
	.theme-card:hover {
		border-color: var(--line-bright);
		background: rgba(255, 255, 255, 0.05);
	}
	.theme-card.selected {
		border-color: var(--gold);
		box-shadow: 0 0 0 1px var(--gold);
	}
	.preview {
		height: 62px;
		border-radius: 4px;
		overflow: hidden;
		border: 1px solid rgba(0, 0, 0, 0.45);
	}
	.preview-classic {
		background: #232323;
		display: flex;
		flex-direction: column;
	}
	.p-bar {
		height: 9px;
		background: #3a3a3a;
		border-bottom: 1px solid #111;
	}
	.p-classic-body {
		flex: 1;
		display: flex;
		gap: 5px;
		padding: 6px;
	}
	.p-classic-win {
		flex: 1;
		background: #383838;
		border: 1px solid #5c5c5c;
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 3px;
	}
	.p-classic-title {
		height: 5px;
		background: #555;
	}
	.p-classic-line {
		height: 4px;
		width: 75%;
		background: #777;
	}
	.p-classic-btn {
		margin-top: auto;
		height: 8px;
		width: 55%;
		background: #4e6c45;
		border: 1px solid #222;
	}
	.preview-modern-dark {
		background: #081319;
	}
	.preview-modern-light {
		background: #f4f7fa;
	}
	.p-mod-shell {
		height: 100%;
		display: grid;
		grid-template-columns: 28px 1fr;
	}
	.preview-modern-dark .p-mod-side {
		background: #0b1c25;
		border-right: 1px solid #24414d;
	}
	.preview-modern-light .p-mod-side {
		background: #e9f0f5;
		border-right: 1px solid #ccd9e1;
	}
	.p-mod-side {
		padding: 5px 4px;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.p-mod-dot {
		width: 8px;
		height: 8px;
		border-radius: 3px;
		background: #7c4df2;
		margin-bottom: 2px;
	}
	.p-mod-nav {
		height: 4px;
		border-radius: 2px;
		background: rgba(145, 170, 182, 0.35);
	}
	.p-mod-nav.on {
		background: #7c4df2;
	}
	.p-mod-main {
		padding: 6px;
		display: flex;
		flex-direction: column;
		gap: 5px;
	}
	.p-mod-cards {
		flex: 1;
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 4px;
	}
	.preview-modern-dark .p-mod-card {
		background: #10232e;
		border: 1px solid #24414d;
		border-radius: 3px;
	}
	.preview-modern-light .p-mod-card {
		background: #ffffff;
		border: 1px solid #ccd9e1;
		border-radius: 3px;
	}
	.p-mod-cta {
		height: 8px;
		width: 44%;
		margin-left: auto;
		border-radius: 3px;
		background: #7c4df2;
	}
	.meta {
		display: flex;
		flex-direction: column;
		gap: 3px;
	}
	.title {
		display: flex;
		align-items: center;
		gap: 4px;
		color: var(--text-bright);
		font-weight: 600;
	}
	.badge {
		margin-left: auto;
		font-size: 9px;
		padding: 1px 5px;
		border-radius: 99px;
		background: rgba(124, 77, 242, 0.25);
		color: var(--gold);
	}
	.desc {
		color: var(--text-dim);
		font-size: 10px;
		line-height: 1.3;
	}
	.zoom-row {
		gap: 8px;
		margin: 4px 0;
	}
	.zoom-slider {
		flex: 1;
		accent-color: var(--ui-primary, var(--gold));
	}
	.zoom-pct {
		min-width: 40px;
		text-align: right;
		font-weight: 600;
		color: var(--text-bright);
	}
</style>
