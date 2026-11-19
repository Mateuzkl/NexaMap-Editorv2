<script lang="ts">
	import { commands } from "../../lib/commands";
	import { debugConsole } from "../../lib/debug/console.svelte";
	import { map } from "../../lib/map/state.svelte";
	import { mapCommands } from "../../lib/map/view/commands";
	import { app, openSettings, toast } from "../../lib/state/app.svelte";
	import { currentTheme, prefs, savePrefs, setUITheme } from "../../lib/state/prefs.svelte";
	import { UI_THEMES, type UITheme } from "../../lib/theme/theme.svelte";
	import { workspace } from "./workspace.svelte";

	const collapsed = $derived(!!prefs.settings.sidebarCollapsed);
	const theme = $derived(currentTheme());

	function toggleSidebar() {
		void savePrefs({ sidebarCollapsed: !collapsed });
	}

	async function goObjects() {
		if (app.open) {
			app.showWorkspace = false;
			await mapCommands.setMode("objects");
			return;
		}
		if (workspace.canOpenEditor) {
			await workspace.openEditor("objects");
			return;
		}
		commands.open();
	}

	async function goNewMap() {
		if (app.open) {
			app.showWorkspace = false;
			await mapCommands.setMode("map");
			await mapCommands.newMap();
			return;
		}
		if (workspace.canOpenEditor) {
			const ok = await workspace.openEditor("map");
			if (ok) await mapCommands.newMap();
			return;
		}
		toast("Select a valid client folder first to create a map.");
	}

	async function goOpenMap() {
		if (app.open) {
			app.showWorkspace = false;
			await mapCommands.setMode("map");
			await mapCommands.open();
			return;
		}
		if (workspace.canOpenEditor) {
			const ok = await workspace.openEditor("map");
			if (ok) await mapCommands.open();
			return;
		}
		toast("Select a valid client folder first to open a map.");
	}

	function goMultiplayer() {
		workspace.activeSection = "multiplayer";
	}

	function goConverters() {
		workspace.activeSection = "converters";
	}

	function pickTheme(next: UITheme) {
		void setUITheme(next);
	}
</script>

<aside class="ws-sidebar" class:collapsed aria-label="Workspace navigation">
	<div class="ws-brand">
		<img class="ws-logo" src="/icon.png" alt="" width="24" height="24" draggable="false" />
		{#if !collapsed}
			<div class="ws-brand-text">
				<span class="ws-brand-name">NexaMap Editor</span>
				<span class="ws-brand-sub">Modern Workspace</span>
			</div>
		{/if}
		<button
			type="button"
			class="ws-collapse"
			title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
			aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
			onclick={toggleSidebar}
		>
			{collapsed ? "»" : "«"}
		</button>
	</div>

	<nav class="ws-nav">
		<button
			type="button"
			class="ws-nav-item"
			class:active={workspace.activeSection === "workspace"}
			title="Workspace"
			onclick={() => (workspace.activeSection = "workspace")}
		>
			<span class="ws-nav-icon" aria-hidden="true">▣</span>
			{#if !collapsed}<span class="ws-nav-label">Workspace</span>{/if}
		</button>

		<button type="button" class="ws-nav-item" title="Object Editor" onclick={goObjects}>
			<span class="ws-nav-icon" aria-hidden="true">◈</span>
			{#if !collapsed}<span class="ws-nav-label">Object Editor</span>{/if}
		</button>

		<button type="button" class="ws-nav-item" title="New Map" onclick={goNewMap}>
			<span class="ws-nav-icon" aria-hidden="true">▦</span>
			{#if !collapsed}<span class="ws-nav-label">New Map</span>{/if}
		</button>

		<button type="button" class="ws-nav-item" title="Open Map" onclick={goOpenMap}>
			<span class="ws-nav-icon" aria-hidden="true">▤</span>
			{#if !collapsed}<span class="ws-nav-label">Open Map</span>{/if}
		</button>

		<button
			type="button"
			class="ws-nav-item"
			class:active={workspace.activeSection === "multiplayer"}
			title="Multiplayer"
			onclick={goMultiplayer}
		>
			<span class="ws-nav-icon" aria-hidden="true">♧</span>
			{#if !collapsed}<span class="ws-nav-label">Multiplayer</span>{/if}
		</button>

		<button
			type="button"
			class="ws-nav-item"
			class:active={workspace.activeSection === "converters"}
			title="Converters & Tools"
			onclick={goConverters}
		>
			<span class="ws-nav-icon" aria-hidden="true">⇄</span>
			{#if !collapsed}<span class="ws-nav-label">Converters</span>{/if}
		</button>

		<div class="ws-nav-sep"></div>

		<button type="button" class="ws-nav-item" title="Market" onclick={() => commands.market()}>
			<span class="ws-nav-icon" aria-hidden="true">★</span>
			{#if !collapsed}<span class="ws-nav-label">Market</span>{/if}
		</button>

		<button
			type="button"
			class="ws-nav-item"
			class:active={debugConsole.open}
			title="Debug Console & Memory Monitor (F12)"
			onclick={() => debugConsole.toggle()}
		>
			<span class="ws-nav-icon" aria-hidden="true">⌘</span>
			{#if !collapsed}<span class="ws-nav-label">Debug Console</span>{/if}
		</button>

		<button type="button" class="ws-nav-item" title="Preferences" onclick={() => openSettings("appearance")}>
			<span class="ws-nav-icon" aria-hidden="true">⚙</span>
			{#if !collapsed}<span class="ws-nav-label">Preferences</span>{/if}
		</button>
	</nav>

	<div class="ws-side-footer">
		{#if !collapsed}
			<div class="ws-theme-label">Appearance</div>
			<div class="ws-theme-pills" role="group" aria-label="Appearance theme">
				{#each UI_THEMES as entry (entry.id)}
					<button
						type="button"
						class="ws-pill"
						class:active={theme === entry.id}
						title={entry.label}
						onclick={() => pickTheme(entry.id)}
					>
						{entry.shortLabel}
					</button>
				{/each}
			</div>
		{:else}
			<button
				type="button"
				class="ws-nav-item"
				title="Appearance settings"
				onclick={() => openSettings("appearance")}
			>
				<span class="ws-nav-icon" aria-hidden="true">◐</span>
			</button>
		{/if}
	</div>
</aside>

<style>
	.ws-sidebar {
		width: 232px;
		flex: none;
		display: flex;
		flex-direction: column;
		background: var(--ui-sidebar);
		border-right: 1px solid var(--ui-border);
		padding: 14px 12px;
		gap: 14px;
		transition: width 0.16s ease;
	}
	.ws-sidebar.collapsed {
		width: 64px;
		padding: 14px 8px;
		align-items: center;
	}
	.ws-brand {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 4px 4px 10px;
		border-bottom: 1px solid var(--ui-border);
		width: 100%;
	}
	.ws-sidebar.collapsed .ws-brand {
		flex-direction: column;
		gap: 6px;
		padding-bottom: 10px;
	}
	.ws-logo {
		flex: none;
		border-radius: 6px;
		object-fit: contain;
	}
	.ws-brand-text {
		display: flex;
		flex-direction: column;
		min-width: 0;
		flex: 1;
	}
	.ws-brand-name {
		font-size: 14px;
		font-weight: 700;
		color: var(--ui-text);
		letter-spacing: 0.02em;
	}
	.ws-brand-sub {
		font-size: 11px;
		color: var(--ui-muted);
	}
	.ws-collapse {
		flex: none;
		width: 26px;
		height: 26px;
		border-radius: 6px;
		border: 1px solid var(--ui-border);
		background: transparent;
		color: var(--ui-muted);
		cursor: pointer;
		display: grid;
		place-items: center;
		font-size: 13px;
	}
	.ws-collapse:hover {
		background: var(--ui-surface-raised);
		color: var(--ui-text);
	}
	.ws-nav {
		display: flex;
		flex-direction: column;
		gap: 4px;
		flex: 1;
		width: 100%;
	}
	.ws-nav-item {
		display: flex;
		align-items: center;
		gap: 10px;
		min-height: 36px;
		padding: 0 10px;
		border-radius: 8px;
		border: 1px solid transparent;
		background: transparent;
		color: var(--ui-muted);
		font: inherit;
		font-size: 13px;
		font-weight: 500;
		text-align: left;
		cursor: pointer;
		width: 100%;
		transition:
			background 0.12s ease,
			color 0.12s ease,
			border-color 0.12s ease;
	}
	.ws-sidebar.collapsed .ws-nav-item {
		justify-content: center;
		padding: 0;
	}
	.ws-nav-item:hover {
		background: var(--ui-surface-raised);
		color: var(--ui-text);
	}
	.ws-nav-item.active {
		background: color-mix(in srgb, var(--ui-primary) 18%, transparent);
		border-color: color-mix(in srgb, var(--ui-primary) 45%, transparent);
		color: var(--ui-text);
	}
	.ws-nav-icon {
		width: 18px;
		text-align: center;
		font-size: 14px;
		flex: none;
	}
	.ws-nav-label {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ws-nav-sep {
		height: 1px;
		background: var(--ui-border);
		margin: 6px 4px;
	}
	.ws-side-footer {
		width: 100%;
		padding-top: 10px;
		border-top: 1px solid var(--ui-border);
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.ws-theme-label {
		font-size: 11px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--ui-muted);
		padding-left: 2px;
	}
	.ws-theme-pills {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 4px;
		background: var(--ui-bg);
		padding: 3px;
		border-radius: 8px;
		border: 1px solid var(--ui-border);
	}
	.ws-pill {
		border: 0;
		background: transparent;
		color: var(--ui-muted);
		font: inherit;
		font-size: 11px;
		font-weight: 600;
		padding: 5px 4px;
		border-radius: 6px;
		cursor: pointer;
	}
	.ws-pill:hover {
		color: var(--ui-text);
	}
	.ws-pill.active {
		background: var(--ui-primary);
		color: #ffffff;
	}
</style>
