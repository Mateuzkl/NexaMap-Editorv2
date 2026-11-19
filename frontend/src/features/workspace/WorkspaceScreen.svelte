<script lang="ts">
	import { onMount } from "svelte";
	import { commands } from "../../lib/commands";
	import { loadMapData, map } from "../../lib/map/state.svelte";
	import { mapCommands } from "../../lib/map/view/commands";
	import { app, openSettings, toast } from "../../lib/state/app.svelte";
	import { currentTheme, setUITheme } from "../../lib/state/prefs.svelte";
	import { nextTheme, themeLabel } from "../../lib/theme/theme.svelte";
	import ClientCard from "./ClientCard.svelte";
	import CustomConverterPanel from "./CustomConverterPanel.svelte";
	import DetectedMaps from "./DetectedMaps.svelte";
	import RecentWorkspaces from "./RecentWorkspaces.svelte";
	import ResourcesSummary from "./ResourcesSummary.svelte";
	import ServerCard from "./ServerCard.svelte";
	import WorkspaceOverview from "./WorkspaceOverview.svelte";
	import WorkspaceSidebar from "./WorkspaceSidebar.svelte";
	import { workspace } from "./workspace.svelte";

	let joinCode = $state("");

	onMount(() => {
		workspace.hydrateFromPrefs();
		void loadMapData();
	});

	const theme = $derived(currentTheme());

	function cycleTheme() {
		void setUITheme(nextTheme(theme));
	}

	async function openCollabDialog() {
		if (!app.open) {
			if (workspace.canOpenEditor) {
				const ok = await workspace.openEditor("map");
				if (!ok) return;
			} else {
				toast("Open or select a valid client first to use Multiplayer.");
				return;
			}
		} else {
			app.showWorkspace = false;
			await mapCommands.setMode("map");
		}
		map.dialog = "collab";
	}

	async function joinSession() {
		const code = joinCode.trim();
		if (!code) return;
		if (!app.open) {
			if (!workspace.canOpenEditor) {
				toast("Select a valid client folder first before joining a shared map.");
				return;
			}
			const ok = await workspace.openEditor("map");
			if (!ok) return;
		} else {
			app.showWorkspace = false;
		}
		await mapCommands.join(code);
	}

	async function openServerData() {
		if (!app.open) {
			toast("Open a client first to manage server data and RME brush imports.");
			return;
		}
		app.showWorkspace = false;
		await mapCommands.setMode("map");
		map.dialog = "data";
	}
</script>

<div class="ws-root">
	<WorkspaceSidebar />

	<div class="ws-main">
		<header class="ws-header">
			<div class="ws-head-copy">
				{#if workspace.activeSection === "workspace"}
					<h1 class="ws-title">Workspace</h1>
					<p class="ws-subtitle">
						Select your client and server folders. NexaMap Editor detects resources and maps automatically.
					</p>
				{:else if workspace.activeSection === "converters"}
					<h1 class="ws-title">Converters &amp; Tools</h1>
					<p class="ws-subtitle">
						Direct access to NexaMap Editor's built-in format conversion, comparison, and asset tools.
					</p>
				{:else}
					<h1 class="ws-title">Multiplayer</h1>
					<p class="ws-subtitle">
						Collaborate live on a shared map using NexaMap Editor's built-in collaborative editor.
					</p>
				{/if}
			</div>

			<div class="ws-head-actions">
				{#if app.open}
					<button type="button" class="ws-btn ws-btn-secondary" onclick={() => (app.showWorkspace = false)}>
						Back to Editor
					</button>
				{/if}
				<button type="button" class="ws-btn ws-btn-ghost" title="Switch interface theme" onclick={cycleTheme}>
					Theme: {themeLabel(theme)}
				</button>
				<button type="button" class="ws-btn ws-btn-ghost" onclick={() => openSettings("appearance")}>
					Settings
				</button>
			</div>
		</header>

		<div class="ws-scroll">
			<div class="ws-container">
				{#if workspace.activeSection === "workspace"}
					<div class="ws-two-col">
						<ClientCard />
						<ServerCard />
					</div>
					<ResourcesSummary />
					<WorkspaceOverview />
					<div class="ws-two-col">
						<DetectedMaps />
						<RecentWorkspaces />
					</div>
				{:else if workspace.activeSection === "converters"}
					<CustomConverterPanel />
					<div class="ws-tools-grid">
						<article class="ws-tool-card">
							<h3>Compile As / Convert Client</h3>
							<p>Convert between client versions and between DAT/SPR and Tibia 12+ assets.</p>
							<button
								type="button"
								class="ws-btn ws-btn-primary"
								disabled={!app.open}
								title={app.open ? "Open Compile As dialog" : "Open a client first"}
								onclick={() => commands.compileAs()}
							>
								Compile As…
							</button>
						</article>

						<article class="ws-tool-card">
							<h3>Compare &amp; Merge Clients</h3>
							<p>Diff two clients side-by-side by pixel data and copy objects or sprites safely.</p>
							<button
								type="button"
								class="ws-btn ws-btn-primary"
								disabled={!app.open}
								title={app.open ? "Open Compare dialog" : "Open a client first"}
								onclick={() => commands.compare()}
							>
								Compare Clients…
							</button>
						</article>

						<article class="ws-tool-card">
							<h3>Object Viewer (.otobj / .obd)</h3>
							<p>Inspect and preview OTOBJ and ObjectBuilder OBD files without opening a client.</p>
							<button type="button" class="ws-btn ws-btn-secondary" onclick={() => commands.viewObd()}>
								Open Object Viewer…
							</button>
						</article>

						<article class="ws-tool-card">
							<h3>Sprite Slicer</h3>
							<p>Slice sprite sheets or images into individual 32×32 client sprites.</p>
							<button
								type="button"
								class="ws-btn ws-btn-secondary"
								disabled={!app.open}
								title={app.open ? "Open Sprite Slicer" : "Open a client first"}
								onclick={() => (app.dialog = "slicer")}
							>
								Open Slicer…
							</button>
						</article>

						<article class="ws-tool-card">
							<h3>Optimize Sprites</h3>
							<p>Deduplicate identical sprites and remove empty or unused sprite slots.</p>
							<button
								type="button"
								class="ws-btn ws-btn-secondary"
								disabled={!app.open}
								title={app.open ? "Open Optimize Sprites dialog" : "Open a client first"}
								onclick={() => (app.dialog = "optimize")}
							>
								Optimize Sprites…
							</button>
						</article>

						<article class="ws-tool-card">
							<h3>Server Data &amp; RME Brushes</h3>
							<p>Configure `items.otb`, `items.xml`, and import Remere's Map Editor brush sets.</p>
							<button
								type="button"
								class="ws-btn ws-btn-secondary"
								disabled={!app.open}
								title={app.open ? "Open Server Data dialog" : "Open a client first"}
								onclick={openServerData}
							>
								Server Data…
							</button>
						</article>
					</div>
				{:else}
					<section class="ws-mp-card">
						<h3>Join a Shared Map</h3>
						<p class="ws-mp-desc">
							Paste an invite code (<code>ots:…</code>) or host address shared by another NexaMap Editor
							user. Requires a valid client selected in Workspace (or already open).
						</p>
						<div class="ws-mp-row">
							<input
								class="ws-mp-input"
								type="text"
								placeholder="ots:… or host:port"
								bind:value={joinCode}
								onkeydown={(e) => e.key === "Enter" && joinSession()}
							/>
							<button
								type="button"
								class="ws-btn ws-btn-primary"
								disabled={!joinCode.trim() || (!app.open && !workspace.canOpenEditor)}
								onclick={joinSession}
							>
								Join Map
							</button>
						</div>
						<div class="ws-mp-divider"></div>
						<h3>Host / Manage Live Session</h3>
						<p class="ws-mp-desc">
							Open the collaborative session dialog inside the Map Editor to share your active map or join
							with password and nickname options.
						</p>
						<div>
							<button
								type="button"
								class="ws-btn ws-btn-secondary"
								disabled={!app.open && !workspace.canOpenEditor}
								onclick={openCollabDialog}
							>
								Open Multiplayer Dialog…
							</button>
						</div>
					</section>
				{/if}
			</div>
		</div>
	</div>
</div>

<style>
	.ws-root {
		grid-column: 1 / -1;
		display: flex;
		min-height: 0;
		height: 100%;
		width: 100%;
		background: var(--ui-bg);
		color: var(--ui-text);
		overflow: hidden;
	}
	.ws-main {
		flex: 1;
		min-width: 0;
		min-height: 0;
		display: flex;
		flex-direction: column;
	}
	.ws-header {
		flex: none;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		padding: 18px 26px 14px;
		border-bottom: 1px solid var(--ui-border);
		background: var(--ui-bg);
	}
	.ws-head-copy {
		min-width: 0;
	}
	.ws-title {
		margin: 0;
		font-size: 20px;
		font-weight: 700;
		color: var(--ui-text);
		letter-spacing: -0.01em;
	}
	.ws-subtitle {
		margin: 4px 0 0;
		font-size: 13px;
		color: var(--ui-muted);
	}
	.ws-head-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
	}
	.ws-scroll {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		overflow-x: hidden;
		padding: 20px 26px 28px;
	}
	.ws-container {
		max-width: 1220px;
		margin: 0 auto;
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	.ws-two-col {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 16px;
	}
	@media (max-width: 1080px) {
		.ws-two-col {
			grid-template-columns: 1fr;
		}
	}
	.ws-tools-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
		gap: 14px;
	}
	.ws-tool-card {
		background: var(--ui-surface);
		border: 1px solid var(--ui-border);
		border-radius: var(--ui-radius);
		padding: 18px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.ws-tool-card h3 {
		margin: 0;
		font-size: 14px;
		font-weight: 700;
		color: var(--ui-text);
	}
	.ws-tool-card p {
		margin: 0;
		font-size: 12.5px;
		color: var(--ui-muted);
		line-height: 1.45;
		flex: 1;
	}
	.ws-mp-card {
		background: var(--ui-surface);
		border: 1px solid var(--ui-border);
		border-radius: var(--ui-radius);
		padding: 20px;
		display: flex;
		flex-direction: column;
		gap: 10px;
		max-width: 720px;
	}
	.ws-mp-card h3 {
		margin: 0;
		font-size: 14px;
		font-weight: 700;
		color: var(--ui-text);
	}
	.ws-mp-desc {
		margin: 0;
		font-size: 12.5px;
		color: var(--ui-muted);
		line-height: 1.45;
	}
	.ws-mp-row {
		display: flex;
		gap: 8px;
	}
	.ws-mp-input {
		flex: 1;
		min-width: 0;
		height: 36px;
		padding: 0 12px;
		border-radius: 8px;
		border: 1px solid var(--ui-border);
		background: var(--ui-bg);
		color: var(--ui-text);
		font: inherit;
		font-size: 13px;
	}
	.ws-mp-divider {
		height: 1px;
		background: var(--ui-border);
		margin: 6px 0;
	}

	/* Shared Modern Workspace button/status/alert utilities scoped to ws-root */
	.ws-root :global(.ws-btn) {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 6px;
		min-height: 36px;
		padding: 0 14px;
		border-radius: 8px;
		border: 1px solid transparent;
		font: inherit;
		font-size: 13px;
		font-weight: 600;
		cursor: pointer;
		transition:
			background 0.12s ease,
			border-color 0.12s ease,
			color 0.12s ease;
	}
	.ws-root :global(.ws-btn-sm) {
		min-height: 30px;
		padding: 0 10px;
		font-size: 12px;
	}
	.ws-root :global(.ws-btn-lg) {
		min-height: 38px;
		padding: 0 20px;
	}
	.ws-root :global(.ws-btn:disabled) {
		opacity: 0.45;
		cursor: not-allowed;
	}
	.ws-root :global(.ws-btn-primary) {
		background: var(--ui-primary);
		color: #ffffff;
	}
	.ws-root :global(.ws-btn-primary:hover:not(:disabled)) {
		background: var(--ui-primary-hover);
	}
	.ws-root :global(.ws-btn-secondary) {
		background: var(--ui-surface-raised);
		border-color: var(--ui-border);
		color: var(--ui-text);
	}
	.ws-root :global(.ws-btn-secondary:hover:not(:disabled)) {
		border-color: var(--ui-primary);
	}
	.ws-root :global(.ws-btn-ghost) {
		background: transparent;
		border-color: var(--ui-border);
		color: var(--ui-muted);
	}
	.ws-root :global(.ws-btn-ghost:hover:not(:disabled)) {
		background: var(--ui-surface-raised);
		color: var(--ui-text);
	}
	.ws-root :global(.ws-link-btn) {
		background: none;
		border: 0;
		padding: 0;
		color: var(--ui-link);
		font: inherit;
		font-size: 12px;
		font-weight: 600;
		cursor: pointer;
	}
	.ws-root :global(.ws-link-btn:hover) {
		text-decoration: underline;
	}
	.ws-root :global(.ws-status) {
		display: inline-flex;
		align-items: center;
		padding: 3px 9px;
		border-radius: 99px;
		font-size: 11px;
		font-weight: 700;
		flex: none;
	}
	.ws-root :global(.ws-status-ok) {
		background: color-mix(in srgb, var(--ui-success) 18%, transparent);
		color: var(--ui-success);
	}
	.ws-root :global(.ws-status-warn) {
		background: color-mix(in srgb, var(--ui-warning) 18%, transparent);
		color: var(--ui-warning);
	}
	.ws-root :global(.ws-status-error) {
		background: color-mix(in srgb, var(--ui-danger) 18%, transparent);
		color: var(--ui-danger);
	}
	.ws-root :global(.ws-status-info) {
		background: color-mix(in srgb, var(--ui-link) 18%, transparent);
		color: var(--ui-link);
	}
	.ws-root :global(.ws-status-neutral) {
		background: var(--ui-surface-raised);
		color: var(--ui-muted);
	}
	.ws-root :global(.ws-alert) {
		padding: 9px 12px;
		border-radius: 8px;
		font-size: 12px;
		line-height: 1.45;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.ws-root :global(.ws-alert-warn) {
		background: color-mix(in srgb, var(--ui-warning) 12%, transparent);
		border: 1px solid color-mix(in srgb, var(--ui-warning) 35%, transparent);
		color: var(--ui-text);
	}
	.ws-root :global(.ws-alert-error) {
		background: color-mix(in srgb, var(--ui-danger) 12%, transparent);
		border: 1px solid color-mix(in srgb, var(--ui-danger) 35%, transparent);
		color: var(--ui-text);
	}
</style>
