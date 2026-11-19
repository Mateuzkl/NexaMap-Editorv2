import { beforeEach, describe, expect, it, vi } from "vitest";
import { Format, ResourceState, WorkspaceService, type WorkspaceInspection } from "../../lib/api";
import { deriveWorkspaceName, workspace } from "./workspace.svelte";

function makeInspection(overrides: Partial<WorkspaceInspection> = {}): WorkspaceInspection {
	return {
		generation: 1,
		client: {
			root: "/client",
			state: ResourceState.ResourceFound,
			files: {
				format: Format.FormatDat,
				datPath: "/client/Tibia.dat",
				sprPath: "/client/Tibia.spr",
				datSignature: 0x4c220594,
				sprSignature: 0x4c220594,
				detected: { value: 860, name: "8.60", datSignature: 0x4c220594, sprSignature: 0x4c220594 },
				features: {
					extended: false,
					transparency: false,
					improvedAnimations: false,
					frameGroups: false,
					spriteSize: 32,
				},
				hasOtfi: false,
				otbPath: "",
			},
			candidates: [],
			ambiguous: false,
			error: "",
		},
		server: {
			root: "/server",
			state: ResourceState.ResourceFound,
			profile: "TFS",
			profileNote: "Detected config.lua, items.otb and TFS data layout",
			serverDir: "/server/data",
			itemsOtb: { path: "/server/data/items/items.otb", state: ResourceState.ResourceFound, details: "v3.20" },
			itemsXml: { path: "/server/data/items/items.xml", state: ResourceState.ResourceFound, details: "Valid" },
			maps: [
				{
					name: "world.otbm",
					path: "/server/data/world/world.otbm",
					format: "otbm",
					size: 1024,
					relDir: "data/world",
				},
			],
			monsters: { path: "/server/data/monster", state: ResourceState.ResourceFound, details: "12 files" },
			npcs: { path: "/server/data/npc", state: ResourceState.ResourceFound, details: "3 files" },
			materials: { path: "", state: ResourceState.ResourceMissing },
			error: "",
		},
		...overrides,
	};
}

describe("WorkspaceStore", () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		workspace.clientRoot = "";
		workspace.serverRoot = "";
		workspace.selectedClientPath = "";
		workspace.selectedOtbPath = "";
		workspace.selectedXmlPath = "";
		workspace.preferredItemIds = "auto";
		workspace.inspection = null;
		workspace.scanning = false;
		workspace.scanError = "";
		workspace.opening = false;
	});

	it("derives readable workspace names", () => {
		expect(deriveWorkspaceName("/clients/860", "/servers/my-tfs", "8.60")).toBe("my-tfs (8.60)");
		expect(deriveWorkspaceName("/clients/860", "", "8.60")).toBe("860 (8.60)");
		expect(deriveWorkspaceName("", "")).toBe("Untitled Workspace");
	});

	it("allows opening editor with valid client even when server is not configured", async () => {
		const clientOnly = makeInspection({
			server: {
				...makeInspection().server,
				root: "",
				state: ResourceState.ResourceNotConfigured,
				itemsOtb: { path: "", state: ResourceState.ResourceNotConfigured },
				itemsXml: { path: "", state: ResourceState.ResourceNotConfigured },
				maps: [],
			},
		});
		vi.spyOn(WorkspaceService, "InspectWorkspace").mockResolvedValue(clientOnly as never);

		workspace.clientRoot = "/client";
		await workspace.inspect();

		expect(workspace.canOpenEditor).toBe(true);
		expect(workspace.openBlockedReason).toBe("");
	});

	it("blocks opening when client is invalid or ambiguous", async () => {
		const invalid = makeInspection({
			client: {
				root: "/broken",
				state: ResourceState.ResourceInvalid,
				error: "Tibia.spr: file does not exist",
			},
		});
		vi.spyOn(WorkspaceService, "InspectWorkspace").mockResolvedValue(invalid as never);

		workspace.clientRoot = "/broken";
		await workspace.inspect();

		expect(workspace.canOpenEditor).toBe(false);
		expect(workspace.openBlockedReason).toContain("Tibia.spr");

		const ambiguous = makeInspection({
			client: {
				root: "/multi",
				state: ResourceState.ResourceFound,
				ambiguous: true,
				candidates: [
					{ path: "/multi/A.dat", kind: "dat", label: "A.dat" },
					{ path: "/multi/B.dat", kind: "dat", label: "B.dat" },
				],
			},
		});
		vi.spyOn(WorkspaceService, "InspectWorkspace").mockResolvedValue(ambiguous as never);
		await workspace.inspect();

		expect(workspace.canOpenEditor).toBe(false);
		expect(workspace.openBlockedReason).toContain("Choose which client file");
	});

	it("blocks ServerID mode when items.otb is missing or invalid", async () => {
		const noOtb = makeInspection({
			server: {
				...makeInspection().server,
				itemsOtb: { path: "", state: ResourceState.ResourceMissing },
			},
		});
		vi.spyOn(WorkspaceService, "InspectWorkspace").mockResolvedValue(noOtb as never);

		workspace.clientRoot = "/client";
		workspace.serverRoot = "/server";
		await workspace.inspect();
		expect(workspace.canOpenEditor).toBe(true);

		workspace.preferredItemIds = "server";
		expect(workspace.canOpenEditor).toBe(false);
		expect(workspace.openBlockedReason).toContain("ServerID mode requires a valid items.otb");
	});

	it("ignores stale out-of-order inspection responses", async () => {
		let resolveFirst!: (val: WorkspaceInspection) => void;
		let resolveSecond!: (val: WorkspaceInspection) => void;
		const first = new Promise<WorkspaceInspection>((r) => (resolveFirst = r));
		const second = new Promise<WorkspaceInspection>((r) => (resolveSecond = r));

		const spy = vi.spyOn(WorkspaceService, "InspectWorkspace");
		spy.mockReturnValueOnce(first as never).mockReturnValueOnce(second as never);

		workspace.clientRoot = "/first";
		const p1 = workspace.inspect();
		workspace.clientRoot = "/second";
		const p2 = workspace.inspect();

		const secondResult = makeInspection({ generation: 2, client: { ...makeInspection().client, root: "/second" } });
		const firstResult = makeInspection({ generation: 1, client: { ...makeInspection().client, root: "/first" } });

		resolveSecond(secondResult);
		await p2;
		expect(workspace.inspection?.client.root).toBe("/second");

		resolveFirst(firstResult);
		await p1;
		// Stale response must NOT overwrite newer workspace selection.
		expect(workspace.inspection?.client.root).toBe("/second");
	});
});
