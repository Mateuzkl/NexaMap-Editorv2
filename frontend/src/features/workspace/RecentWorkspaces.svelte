<script lang="ts">
	import type { WorkspaceProfile } from "../../lib/api";
	import { commands } from "../../lib/commands";
	import { askText } from "../../lib/confirm.svelte";
	import { prefs, recentDir, shortPath } from "../../lib/state/prefs.svelte";
	import { workspace } from "./workspace.svelte";

	const profiles = $derived(prefs.settings.workspaces ?? []);
	const recentClients = $derived((prefs.settings.recent ?? []).slice(0, 5));

	async function rename(profile: WorkspaceProfile) {
		const next = await askText({
			title: "Rename workspace",
			label: "Workspace name",
			value: profile.name,
			ok: "Rename",
		});
		if (next) await workspace.renameProfile(profile.id, next);
	}

	function formatDate(iso: string): string {
		if (!iso) return "";
		const parsed = new Date(iso);
		if (Number.isNaN(parsed.getTime())) return "";
		return parsed.toLocaleDateString();
	}
</script>

<section class="ws-section" aria-label="Recent workspaces">
	<header class="ws-section-head">
		<h3 class="ws-section-title">RECENT WORKSPACES</h3>
		<span class="ws-sub">Removing from list never deletes files on disk</span>
	</header>

	{#if profiles.length > 0}
		<div class="ws-rec-list">
			{#each profiles as profile (profile.id)}
				<div class="ws-rec-item" class:active={workspace.activeProfileId === profile.id}>
					<button
						type="button"
						class="ws-rec-main"
						title={`Client: ${profile.clientRoot || "none"}\nServer: ${profile.serverRoot || "none"}`}
						onclick={() => workspace.applyProfile(profile)}
					>
						<div class="ws-rec-head">
							<span class="ws-rec-name">{profile.name}</span>
							{#if profile.lastUsedAt}
								<span class="ws-rec-date">{formatDate(profile.lastUsedAt)}</span>
							{/if}
						</div>
						<div class="ws-rec-paths">
							{#if profile.clientRoot}
								<span>Client: {shortPath(profile.clientRoot, 44)}</span>
							{/if}
							{#if profile.serverRoot}
								<span>· Server: {shortPath(profile.serverRoot, 44)}</span>
							{/if}
						</div>
					</button>
					<div class="ws-rec-actions">
						<button type="button" class="ws-btn ws-btn-ghost ws-btn-sm" onclick={() => rename(profile)}>
							Rename
						</button>
						<button
							type="button"
							class="ws-btn ws-btn-ghost ws-btn-sm ws-danger"
							title="Remove workspace from list (keeps all files on disk)"
							onclick={() => workspace.removeProfile(profile.id)}
						>
							Remove
						</button>
					</div>
				</div>
			{/each}
		</div>
	{:else if recentClients.length > 0}
		<p class="ws-empty">No saved workspaces yet. Recent clients you opened before:</p>
		<div class="ws-rec-list">
			{#each recentClients as client (client.datPath)}
				<div class="ws-rec-item">
					<button
						type="button"
						class="ws-rec-main"
						title={client.datPath}
						onclick={() => {
							workspace.clientRoot = recentDir(client);
							workspace.selectedClientPath = "";
							void workspace.inspect();
						}}
					>
						<div class="ws-rec-head">
							<span class="ws-rec-name">Client {client.version.name}</span>
						</div>
						<div class="ws-rec-paths">{shortPath(recentDir(client), 64)}</div>
					</button>
					<button
						type="button"
						class="ws-btn ws-btn-secondary ws-btn-sm"
						onclick={() => commands.openRecent(client)}
					>
						Open
					</button>
				</div>
			{/each}
		</div>
	{:else}
		<p class="ws-empty">Saved client + server pairs will appear here so you can reopen them in one click.</p>
	{/if}
</section>

<style>
	.ws-section {
		background: var(--ui-surface);
		border: 1px solid var(--ui-border);
		border-radius: var(--ui-radius);
		padding: 16px 18px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.ws-section-head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
	}
	.ws-section-title {
		margin: 0;
		font-size: 12px;
		font-weight: 700;
		letter-spacing: 0.08em;
		color: var(--ui-muted);
	}
	.ws-sub {
		font-size: 11px;
		color: var(--ui-muted);
	}
	.ws-empty {
		margin: 0;
		font-size: 12.5px;
		color: var(--ui-muted);
	}
	.ws-rec-list {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.ws-rec-item {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 10px;
		border-radius: 9px;
		background: var(--ui-bg);
		border: 1px solid var(--ui-border);
	}
	.ws-rec-item.active {
		border-color: var(--ui-primary);
	}
	.ws-rec-main {
		flex: 1;
		min-width: 0;
		background: none;
		border: 0;
		padding: 0;
		text-align: left;
		cursor: pointer;
		font: inherit;
		color: var(--ui-text);
		display: flex;
		flex-direction: column;
		gap: 3px;
	}
	.ws-rec-head {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.ws-rec-name {
		font-size: 13px;
		font-weight: 600;
		color: var(--ui-text);
	}
	.ws-rec-date {
		font-size: 11px;
		color: var(--ui-muted);
	}
	.ws-rec-paths {
		font-size: 11.5px;
		color: var(--ui-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ws-rec-actions {
		display: flex;
		gap: 4px;
		flex: none;
	}
	.ws-danger:hover {
		color: var(--ui-danger);
		border-color: var(--ui-danger);
	}
</style>
