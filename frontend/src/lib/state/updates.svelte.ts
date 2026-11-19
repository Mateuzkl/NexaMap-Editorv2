// Self-update through the Wails updater (app.Updater with the GitHub
// provider, see main.go). Progress comes from the standard updater events.
import { Events, Updater } from "@wailsio/runtime";
import { UpdateService, errorMessage, type UpdateInfo } from "../api";
import { app, toast } from "./app.svelte";

export const REPO_URL = "https://github.com/Mateuzkl/NexaMap-Editorv2";

export type UpdateStage = "idle" | "downloading" | "verifying" | "installing" | "restarting";

export const updates = $state<{
	version: string;
	info: UpdateInfo | null;
	checking: boolean;
	stage: UpdateStage;
	progress: { written: number; total: number } | null;
}>({ version: "", info: null, checking: false, stage: "idle", progress: null });

let stopUpdateEvents: (() => void) | null = null;

export async function initUpdates(autoCheck: boolean): Promise<() => void> {
	stopUpdateEvents?.();
	try {
		updates.version = await UpdateService.Version();
	} catch {
		return () => {}; // not running inside Wails
	}
	const offs = [
		Events.On(Updater.Events.DownloadStarted, () => (updates.stage = "downloading")),
		Events.On(
			Updater.Events.DownloadProgress,
			(event: { data: { written: number; total: number } }) => (updates.progress = event.data),
		),
		Events.On(Updater.Events.Verifying, () => (updates.stage = "verifying")),
		Events.On(Updater.Events.Installing, () => (updates.stage = "installing")),
		Events.On(Updater.Events.UpdateReady, () => (updates.stage = "restarting")),
	];
	let checkTimer: ReturnType<typeof setTimeout> | undefined;
	if (updates.version !== "dev" && autoCheck) {
		checkTimer = setTimeout(() => void checkForUpdates(false), 3000);
	}
	const stop = () => {
		clearTimeout(checkTimer);
		for (const off of offs) off();
		if (stopUpdateEvents === stop) stopUpdateEvents = null;
	};
	stopUpdateEvents = stop;
	return stop;
}

export async function checkForUpdates(manual: boolean): Promise<void> {
	if (updates.checking) return;
	updates.checking = true;
	try {
		const info = await UpdateService.Check();
		updates.info = info;
		if (info.available) {
			if (!app.dialog) app.dialog = "update";
		} else if (manual) {
			toast(`NexaMap Editor ${updates.version} is up to date.`, "success");
		}
	} catch (event) {
		if (manual) toast(`Update check failed: ${errorMessage(event)}`, "error");
	} finally {
		updates.checking = false;
	}
}

export async function installUpdate(): Promise<void> {
	updates.stage = "downloading";
	updates.progress = null;
	try {
		await UpdateService.Install();
	} catch (event) {
		updates.stage = "idle";
		toast(`Update failed: ${errorMessage(event)}`, "error");
	}
}
