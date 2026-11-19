import { Format, SettingsService, errorMessage, type Recent, type Settings } from "../api";
import { applyTheme, normalizeTheme, type UITheme } from "../theme/theme.svelte";
import { toast } from "./app.svelte";
import { dirname } from "../util/path";

export const IMAGE_FORMATS = [
	{ value: "png", label: "PNG" },
	{ value: "bmp", label: "BMP" },
	{ value: "jpg", label: "JPG" },
];

export const prefs = $state<{ settings: Settings }>({
	settings: {
		checkUpdates: true,
		reopenLast: false,
		sheetBackground: "magenta",
		exportFormat: "png",
		objectFormat: "otobj",
		listColumns: 7,
		recent: [],
		marketAuthor: "",
		marketAdminToken: "",
		marketShared: {},
		lastDirs: {},
		itemsOtb: {},
		mcpServer: false,
		mcpPort: 47800,
		uiTheme: "modern-dark",
		sidebarCollapsed: false,
		workspaces: [],
		activeWorkspaceId: "",
	},
});

export function currentTheme(): UITheme {
	return normalizeTheme(prefs.settings.uiTheme);
}

/** Objects per row in the object list (matches internal/settings). */
export const LIST_COLUMNS = { min: 6, max: 12 };

export const LIST_CELL = { width: 36, gap: 2 };

/** Width of the object list panel that fits `columns` cells next to the
 * scrollbar (36px is the panel frame, padding and scrollbar). */
export function listPanelWidth(columns: number): number {
	return columns * (LIST_CELL.width + LIST_CELL.gap) - LIST_CELL.gap + 36;
}

export async function loadPrefs(): Promise<void> {
	try {
		const settings = await SettingsService.Get();
		const theme = normalizeTheme(settings.uiTheme);
		prefs.settings = {
			...settings,
			uiTheme: theme,
			recent: settings.recent ?? [],
			workspaces: settings.workspaces ?? [],
		};
		applyTheme(theme);
	} catch {
		/* not running inside Wails: keep defaults */
		applyTheme(currentTheme());
	}
}

export async function savePrefs(change: Partial<Omit<Settings, "recent" | "workspaces">>): Promise<void> {
	const next = { ...prefs.settings, ...change };
	if (change.uiTheme !== undefined) {
		next.uiTheme = normalizeTheme(change.uiTheme);
		applyTheme(next.uiTheme as UITheme);
	}
	prefs.settings = next;
	try {
		await SettingsService.Update(next);
	} catch (err) {
		toast(errorMessage(err), "error");
	}
}

export async function setUITheme(theme: UITheme): Promise<void> {
	const normalized = normalizeTheme(theme);
	applyTheme(normalized);
	await savePrefs({ uiTheme: normalized });
}

export async function removeRecent(recent: Recent | null): Promise<void> {
	try {
		await SettingsService.RemoveRecent(recent?.datPath ?? "");
	} finally {
		await loadPrefs();
	}
}

export function shortPath(path: string, max: number): string {
	return path.length > max ? "…" + path.slice(path.length - max + 1) : path;
}

export function recentDir(recent: Recent): string {
	if (recent.format === Format.FormatAssets) return recent.datPath;
	return dirname(recent.datPath);
}
