import { Events } from "@wailsio/runtime";
import { DebugService, errorMessage, type DebugLogEntry, type RuntimeStats } from "../api";
import { toast } from "../state/app.svelte";

const MAX_CLIENT_LOGS = 1000;

export type LogFilter = "all" | "info" | "warn" | "error";

class DebugConsoleStore {
	open = $state(false);
	entries = $state<DebugLogEntry[]>([]);
	stats = $state<RuntimeStats | null>(null);
	filter = $state<LogFilter>("all");
	query = $state("");
	autoScroll = $state(true);
	unreadErrors = $state(0);

	private pollTimer: ReturnType<typeof setInterval> | null = null;
	private localSeq = -1;

	readonly filtered = $derived.by(() => {
		const q = this.query.trim().toLowerCase();
		return this.entries.filter((entry) => {
			if (this.filter !== "all" && entry.level !== this.filter) return false;
			if (!q) return true;
			return (
				entry.message.toLowerCase().includes(q) ||
				entry.source.toLowerCase().includes(q) ||
				entry.level.toLowerCase().includes(q)
			);
		});
	});

	toggle(): void {
		this.setOpen(!this.open);
	}

	setOpen(next: boolean): void {
		this.open = next;
		if (next) {
			this.unreadErrors = 0;
			void this.refreshStats();
			this.startPolling();
		} else {
			this.stopPolling();
		}
	}

	push(entry: DebugLogEntry): void {
		if (this.entries.length >= MAX_CLIENT_LOGS) {
			this.entries = [...this.entries.slice(100), entry];
		} else {
			this.entries = [...this.entries, entry];
		}
		if (!this.open && entry.level === "error") {
			this.unreadErrors++;
		}
	}

	logFrontend(level: "info" | "warn" | "error", message: string): void {
		const trimmed = message.trim();
		if (!trimmed) return;
		DebugService.Log(level, "frontend", trimmed).catch(() => {
			// Fallback when running outside Wails.
			this.push({
				id: this.localSeq--,
				time: new Date().toTimeString().slice(0, 8),
				level,
				source: "frontend",
				message: trimmed,
			});
		});
	}

	async refreshLogs(): Promise<void> {
		try {
			const list = await DebugService.Logs();
			this.entries = (list ?? []).slice(-MAX_CLIENT_LOGS);
		} catch {
			/* outside Wails */
		}
	}

	async refreshStats(): Promise<void> {
		try {
			this.stats = await DebugService.Stats();
		} catch {
			/* outside Wails */
		}
	}

	async forceGC(): Promise<void> {
		try {
			this.stats = await DebugService.ForceGC();
		} catch (err) {
			toast(errorMessage(err), "error");
		}
	}

	async toggleNativeConsole(): Promise<void> {
		const target = !(this.stats?.nativeConsole ?? false);
		try {
			const opened = await DebugService.ToggleNativeConsole(target);
			await this.refreshStats();
			toast(opened ? "Windows debug console opened." : "Windows debug console closed.", "info");
		} catch (err) {
			toast(errorMessage(err), "error");
		}
	}

	async clear(): Promise<void> {
		this.entries = [];
		this.unreadErrors = 0;
		try {
			await DebugService.Clear();
		} catch {
			/* outside Wails */
		}
	}

	async copyAll(): Promise<void> {
		const text = this.filtered
			.map((e) => `[${e.time}] [${e.level.toUpperCase()}] [${e.source}] ${e.message}`)
			.join("\n");
		try {
			await navigator.clipboard?.writeText(text);
			toast("Debug logs copied to clipboard.", "success");
		} catch (err) {
			toast(errorMessage(err), "error");
		}
	}

	private startPolling(): void {
		this.stopPolling();
		this.pollTimer = setInterval(() => {
			if (this.open) void this.refreshStats();
		}, 2000);
	}

	private stopPolling(): void {
		if (this.pollTimer !== null) {
			clearInterval(this.pollTimer);
			this.pollTimer = null;
		}
	}

	dispose(): void {
		this.stopPolling();
	}
}

export const debugConsole = new DebugConsoleStore();

function formatConsoleArgs(args: unknown[]): string {
	return args
		.map((arg) => {
			if (arg instanceof Error) return arg.stack || arg.message;
			if (typeof arg === "string") return arg;
			try {
				return JSON.stringify(arg);
			} catch {
				return String(arg);
			}
		})
		.join(" ");
}

/** Connects live Go backend logs + frontend error hooks; returns a full cleanup callback. */
export function initDebugConsole(): () => void {
	void debugConsole.refreshLogs();
	void debugConsole.refreshStats();

	const offLog = Events.On("debug:log", (event: { data: DebugLogEntry | null }) => {
		if (event.data) debugConsole.push(event.data);
	});

	const onError = (event: ErrorEvent) => {
		debugConsole.logFrontend("error", `${event.message} (${event.filename}:${event.lineno})`);
	};
	const onRejection = (event: PromiseRejectionEvent) => {
		debugConsole.logFrontend("error", `Unhandled rejection: ${errorMessage(event.reason)}`);
	};
	window.addEventListener("error", onError);
	window.addEventListener("unhandledrejection", onRejection);

	const origWarn = console.warn;
	const origError = console.error;
	console.warn = (...args: unknown[]) => {
		origWarn.apply(console, args);
		debugConsole.logFrontend("warn", formatConsoleArgs(args));
	};
	console.error = (...args: unknown[]) => {
		origError.apply(console, args);
		debugConsole.logFrontend("error", formatConsoleArgs(args));
	};

	return () => {
		offLog();
		window.removeEventListener("error", onError);
		window.removeEventListener("unhandledrejection", onRejection);
		console.warn = origWarn;
		console.error = origError;
		debugConsole.dispose();
	};
}
