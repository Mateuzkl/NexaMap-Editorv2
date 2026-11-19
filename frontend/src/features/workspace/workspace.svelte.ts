import {
	DialogService,
	Format,
	ProjectService,
	ResourceState,
	WorkspaceService,
	errorMessage,
	type WorkspaceInspection,
	type WorkspaceProfile,
} from "../../lib/api";
import { commands, openedNote } from "../../lib/commands";
import { OTB_FILTERS, confirmLeave } from "../../lib/commands/shared";
import { ask } from "../../lib/confirm.svelte";
import { MapService } from "../../lib/map/data/api";
import { loadMapData, map } from "../../lib/map/state.svelte";
import { mapCommands } from "../../lib/map/view/commands";
import { app, run, toast } from "../../lib/state/app.svelte";
import { loadPrefs, prefs } from "../../lib/state/prefs.svelte";
import { basename } from "../../lib/util/path";

export type ItemIdPreference = "auto" | "client" | "server";
export type WorkspaceSection = "workspace" | "converters" | "multiplayer";

export interface WorkspaceSnapshot {
	clientRoot: string;
	serverRoot: string;
	selectedClientPath: string;
	selectedOtbPath: string;
	selectedXmlPath: string;
	preferredItemIds: ItemIdPreference;
	activeProfileId: string;
}

function makeWorkspaceId(): string {
	if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
		return crypto.randomUUID();
	}
	return `ws-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function samePath(a: string, b: string): boolean {
	if (!a || !b) return false;
	return a.replace(/\\/g, "/").toLowerCase() === b.replace(/\\/g, "/").toLowerCase();
}

export function deriveWorkspaceName(clientRoot: string, serverRoot: string, versionName?: string): string {
	const serverName = serverRoot ? basename(serverRoot.replace(/[\\/]+$/, "")) : "";
	const clientName = clientRoot ? basename(clientRoot.replace(/[\\/]+$/, "")) : "";
	if (serverName && versionName) return `${serverName} (${versionName})`;
	if (serverName) return serverName;
	if (clientName && versionName) return `${clientName} (${versionName})`;
	if (clientName) return clientName;
	return "Untitled Workspace";
}

class WorkspaceStore {
	clientRoot = $state("");
	serverRoot = $state("");
	selectedClientPath = $state("");
	selectedOtbPath = $state("");
	selectedXmlPath = $state("");
	preferredItemIds = $state<ItemIdPreference>("auto");
	activeProfileId = $state("");
	activeSection = $state<WorkspaceSection>("workspace");

	inspection = $state.raw<WorkspaceInspection | null>(null);
	scanning = $state(false);
	scanError = $state("");
	opening = $state(false);
	hydrated = $state(false);

	private requestSeq = 0;

	readonly clientFiles = $derived(this.inspection?.client.files ?? null);
	readonly clientState = $derived(this.inspection?.client.state ?? ResourceState.ResourceNotConfigured);
	readonly serverState = $derived(this.inspection?.server.state ?? ResourceState.ResourceNotConfigured);
	readonly isAmbiguousClient = $derived(!!this.inspection?.client.ambiguous);

	readonly detectedOtbPath = $derived(this.inspection?.server.itemsOtb.path ?? "");
	readonly effectiveOtbPath = $derived(this.selectedOtbPath || this.detectedOtbPath);
	readonly otbValid = $derived(this.inspection?.server.itemsOtb.state === ResourceState.ResourceFound);
	readonly attachedOtbPath = $derived(app.project?.info.itemsOtb?.path ?? this.clientFiles?.otbPath ?? "");

	readonly canOpenEditor = $derived.by(() => {
		if (this.scanning || this.opening) return false;
		if (this.isAmbiguousClient) return false;
		if (this.clientState !== ResourceState.ResourceFound || !this.clientFiles) return false;
		if (this.preferredItemIds === "server" && !this.otbValid) return false;
		return true;
	});

	readonly openBlockedReason = $derived.by(() => {
		if (this.scanning) return "Scanning workspace resources…";
		if (!this.clientRoot) return "Select a client folder to open the editor.";
		if (this.isAmbiguousClient) return "Choose which client file to use in the Client card.";
		if (this.clientState !== ResourceState.ResourceFound || !this.clientFiles) {
			return this.inspection?.client.error || "Select a valid client folder (.dat/.spr or Tibia 12+ assets).";
		}
		if (this.preferredItemIds === "server" && !this.otbValid) {
			return "ServerID mode requires a valid items.otb from the server folder.";
		}
		return "";
	});

	hydrateFromPrefs(): void {
		if (this.hydrated) return;
		this.hydrated = true;
		const list = prefs.settings.workspaces ?? [];
		if (!list.length) return;
		const active = list.find((w) => w.id === prefs.settings.activeWorkspaceId) ?? list[0];
		if (active) {
			void this.applyProfile(active, false);
		}
	}

	async inspect(): Promise<WorkspaceInspection | null> {
		const seq = ++this.requestSeq;
		if (!this.clientRoot && !this.serverRoot) {
			this.inspection = null;
			this.scanning = false;
			this.scanError = "";
			return null;
		}
		this.scanning = true;
		this.scanError = "";
		try {
			const result = await WorkspaceService.InspectWorkspace(
				this.clientRoot,
				this.serverRoot,
				this.selectedClientPath,
				this.selectedOtbPath,
				this.selectedXmlPath,
			);
			// Ignore out-of-order responses from earlier scans.
			if (seq !== this.requestSeq) return null;
			this.inspection = result;
			return result;
		} catch (err) {
			if (seq !== this.requestSeq) return null;
			this.scanError = errorMessage(err);
			return null;
		} finally {
			if (seq === this.requestSeq) this.scanning = false;
		}
	}

	cancelScan(): void {
		this.requestSeq++;
		this.scanning = false;
		void WorkspaceService.CancelScan().catch(() => {});
	}

	async pickClientFolder(): Promise<void> {
		const dir = await DialogService.PickDirectory("openClient", "Select client folder");
		if (!dir) return;
		this.clientRoot = dir;
		this.selectedClientPath = "";
		await this.inspect();
	}

	async selectClientCandidate(path: string): Promise<void> {
		this.selectedClientPath = path;
		await this.inspect();
	}

	clearClient(): void {
		this.clientRoot = "";
		this.selectedClientPath = "";
		void this.inspect();
	}

	async pickServerFolder(): Promise<void> {
		const dir = await DialogService.PickDirectory("mapData", "Select server folder");
		if (!dir) return;
		this.serverRoot = dir;
		this.selectedOtbPath = "";
		this.selectedXmlPath = "";
		await this.inspect();
	}

	clearServer(): void {
		this.serverRoot = "";
		this.selectedOtbPath = "";
		this.selectedXmlPath = "";
		void this.inspect();
	}

	async pickOtbOverride(): Promise<void> {
		const paths = await DialogService.OpenFiles("itemsOtb", "Select items.otb", OTB_FILTERS, false);
		if (!paths?.length) return;
		this.selectedOtbPath = paths[0];
		await this.inspect();
	}

	async pickXmlOverride(): Promise<void> {
		const paths = await DialogService.OpenFiles(
			"mapData",
			"Select items.xml",
			[{ name: "Items XML (*.xml)", pattern: "*.xml" }],
			false,
		);
		if (!paths?.length) return;
		this.selectedXmlPath = paths[0];
		await this.inspect();
	}

	async setOtbOverride(path: string): Promise<void> {
		this.selectedOtbPath = path;
		await this.inspect();
	}

	async setXmlOverride(path: string): Promise<void> {
		this.selectedXmlPath = path;
		await this.inspect();
	}

	async applyProfile(profile: WorkspaceProfile, persistActive = true): Promise<void> {
		this.activeProfileId = profile.id;
		this.clientRoot = profile.clientRoot;
		this.serverRoot = profile.serverRoot;
		this.selectedClientPath = profile.selectedClientPath ?? "";
		this.selectedOtbPath = profile.selectedOtbPath ?? "";
		this.selectedXmlPath = profile.selectedXmlPath ?? "";
		const pref = profile.preferredItemIds;
		this.preferredItemIds = pref === "client" || pref === "server" ? pref : "auto";
		if (persistActive) {
			void WorkspaceService.SetActiveWorkspace(profile.id).catch(() => {});
		}
		await this.inspect();
	}

	async saveCurrentProfile(customName?: string): Promise<WorkspaceProfile | null> {
		if (!this.clientRoot && !this.serverRoot) return null;
		const existing = (prefs.settings.workspaces ?? []).find((w) => w.id === this.activeProfileId);
		const id = existing?.id ?? makeWorkspaceId();
		const name =
			customName?.trim() ||
			existing?.name ||
			deriveWorkspaceName(this.clientRoot, this.serverRoot, this.clientFiles?.detected?.name);
		const profile: WorkspaceProfile = {
			id,
			name,
			clientRoot: this.clientRoot,
			serverRoot: this.serverRoot,
			selectedClientPath: this.selectedClientPath,
			selectedOtbPath: this.selectedOtbPath,
			selectedXmlPath: this.selectedXmlPath,
			preferredItemIds: this.preferredItemIds,
			lastUsedAt: new Date().toISOString(),
		};
		try {
			const updated = await WorkspaceService.SaveWorkspace(profile);
			this.activeProfileId = id;
			prefs.settings = {
				...prefs.settings,
				workspaces: updated ?? [],
				activeWorkspaceId: id,
			};
			return profile;
		} catch (err) {
			toast(errorMessage(err), "error");
			return null;
		}
	}

	async renameProfile(id: string, name: string): Promise<void> {
		const trimmed = name.trim();
		if (!trimmed) return;
		try {
			const updated = await WorkspaceService.RenameWorkspace(id, trimmed);
			prefs.settings = { ...prefs.settings, workspaces: updated ?? [] };
		} catch (err) {
			toast(errorMessage(err), "error");
		}
	}

	async removeProfile(id: string): Promise<void> {
		try {
			const updated = await WorkspaceService.RemoveWorkspace(id);
			prefs.settings = { ...prefs.settings, workspaces: updated ?? [] };
			if (this.activeProfileId === id) {
				this.activeProfileId = "";
			}
			await loadPrefs();
		} catch (err) {
			toast(errorMessage(err), "error");
		}
	}

	/** Opens the selected workspace in the Object Editor or Map Editor (optionally opening a specific map). */
	async openEditor(targetMode: "objects" | "map" = "objects", mapPath = ""): Promise<boolean> {
		const files = this.clientFiles;
		if (!files) {
			toast(this.openBlockedReason || "Select a valid client folder first.", "error");
			return false;
		}
		if (this.preferredItemIds === "server" && !this.otbValid) {
			toast("ServerID mode requires a valid items.otb from the server folder.", "error");
			return false;
		}
		// Unknown/custom signature: hand off to the existing OpenDialog so the user
		// picks the version and features explicitly instead of guessing.
		if (!files.detected && files.format !== Format.FormatAssets) {
			commands.open(files.datPath);
			return false;
		}

		const alreadySameClient =
			app.open &&
			samePath(app.project?.info.datPath ?? "", files.datPath) &&
			(files.format === Format.FormatAssets || samePath(app.project?.info.sprPath ?? "", files.sprPath));

		if (app.open && !alreadySameClient) {
			if (!(await confirmLeave("Switch workspace", "Switch"))) {
				return false;
			}
		}

		// Determine whether we should attach or detach the detected/selected items.otb.
		let otbToPass: string | null = null;
		const desiredOtb = this.preferredItemIds === "client" ? "" : this.otbValid ? this.effectiveOtbPath : "";
		const currentRememberedOtb =
			files.otbPath || (alreadySameClient ? (app.project?.info.itemsOtb?.path ?? "") : "");

		if (this.preferredItemIds === "client" && currentRememberedOtb) {
			otbToPass = "";
		} else if (desiredOtb) {
			if (currentRememberedOtb && !samePath(currentRememberedOtb, desiredOtb)) {
				const ok = await ask({
					title: "Change server items (items.otb)",
					message:
						"Changing server items changes the ID mapping used by maps and the OTB that client compilation can synchronize.",
					ok: "Use new items.otb",
				});
				if (ok) otbToPass = desiredOtb;
			} else if (!currentRememberedOtb) {
				otbToPass = desiredOtb;
			}
		}

		this.opening = true;
		try {
			if (!alreadySameClient) {
				const request = {
					format: files.format,
					datPath: files.datPath,
					sprPath: files.sprPath,
					version: files.detected,
					features: files.features,
					otbPath: otbToPass,
				};
				const state = await run("Loading client", () => ProjectService.Open(request));
				if (!state?.open) return false;
				openedNote(state);
				toast(
					`Loaded ${files.detected?.name ?? "client"}: ${state.info.counts.items - 99} items, ${state.info.counts.sprites} sprites.`,
					"success",
				);
			} else if (otbToPass !== null) {
				if (otbToPass === "") {
					await run("Detaching items.otb", () => ProjectService.DetachItemsOTB());
				} else {
					await run("Attaching items.otb", () => ProjectService.AttachItemsOTB(otbToPass!));
				}
			}

			// Apply server data (items.xml + serverDir) while preserving custom materialsPath.
			const server = this.inspection?.server;
			if (server && server.state === ResourceState.ResourceFound) {
				const xmlPath = server.itemsXml.state === ResourceState.ResourceFound ? server.itemsXml.path : "";
				const serverDir = server.serverDir || server.root;
				const existingMaterials = map.data?.config.materialsPath ?? "";
				if (xmlPath || serverDir) {
					try {
						await MapService.SetData({
							xmlPath,
							materialsPath: existingMaterials,
							serverDir,
						});
						await loadMapData(true);
					} catch (err) {
						toast(`Server data warning: ${errorMessage(err)}`, "info");
					}
				}
			}

			await this.saveCurrentProfile();
			app.showWorkspace = false;

			if (mapPath) {
				await mapCommands.setMode("map");
				await mapCommands.open(mapPath);
			} else if (targetMode === "map") {
				await mapCommands.setMode("map");
			} else {
				await mapCommands.setMode("objects");
			}
			return true;
		} finally {
			this.opening = false;
		}
	}
}

export const workspace = new WorkspaceStore();
