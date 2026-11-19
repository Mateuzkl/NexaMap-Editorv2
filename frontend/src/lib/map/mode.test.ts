import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MapService, type ItemTable } from "./data/api";
import { ItemIndex, parseUint32ItemId } from "./data/items";
import { loadMapData, map } from "./state.svelte";

type View = Awaited<ReturnType<typeof MapService.ViewData>>;
type Status = Awaited<ReturnType<typeof MapService.Status>>;

function view(mode: "otb" | "client"): View {
	return {
		data: { ready: true, mode, client: "test", config: {} },
		items: {
			ids: [mode === "otb" ? 1000 : 100],
			clients: [100],
			flags: [0],
			tops: [0],
			minimap: [0],
			names: ["Ground"],
			kinds: [0],
			speeds: [150],
			mode,
			maxId: mode === "otb" ? 1000 : 100,
			flagbits: {},
		} satisfies ItemTable,
		palette: { tilesets: [], brushes: [], creatures: [], doorTypes: [] },
	} as unknown as View;
}

beforeEach(() => {
	map.activeId = 1;
	map.data = null;
	map.items = null;
	map.palette = null;
	map.brush = null;
	map.pasting = false;
	vi.spyOn(MapService, "Status").mockResolvedValue({
		data: { ready: true, mode: "otb" },
		maps: [
			{ id: 1, itemMode: "otb" },
			{ id: 2, itemMode: "client" },
		],
		recent: [],
		keys: {},
		settings: {},
	} as unknown as Status);
});
afterEach(() => vi.restoreAllMocks());

describe("map tab item modes", () => {
	it("uses the selected map's client IDs instead of the shared server defaults", async () => {
		map.activeId = 2;
		const data = vi.spyOn(MapService, "ViewData").mockResolvedValue(view("client"));
		await loadMapData(true);
		expect(data).toHaveBeenCalledWith(2, true);
		expect(map.data?.mode).toBe("client");
		expect(map.items?.client[100]).toBe(100);
		expect(map.items?.has(1000)).toBe(false);
	});

	it("resets server-ID brush and paste state after conversion", async () => {
		const data = vi.spyOn(MapService, "ViewData").mockResolvedValue(view("otb"));
		await loadMapData(true);
		map.brush = { kind: "raw", item: 1000, label: "Ground" };
		map.pasting = true;
		data.mockResolvedValue(view("client"));
		await loadMapData();
		expect(map.items?.mode).toBe("client");
		expect(map.brush).toBe(null);
		expect(map.pasting).toBe(false);
	});

	it("ignores an older tab response that arrives after a new tab loads", async () => {
		let finish!: (result: View) => void;
		const pending = new Promise<View>((resolve) => (finish = resolve));
		const data = vi
			.spyOn(MapService, "ViewData")
			.mockImplementation(
				(id) =>
					(id === 1 ? pending : Promise.resolve(view("client"))) as ReturnType<typeof MapService.ViewData>,
			);
		const oldLoad = loadMapData(true);
		await vi.waitFor(() => expect(data).toHaveBeenCalledWith(1, true));
		map.activeId = 2;
		await loadMapData(true);
		finish(view("otb"));
		await oldLoad;
		expect(map.activeId).toBe(2);
		expect(map.data?.mode).toBe("client");
		expect(map.items?.mode).toBe("client");
	});

	it("supports 32-bit item IDs and client IDs without 16-bit truncation", () => {
		expect(parseUint32ItemId("0")).toMatchObject({ valid: true, value: 0 });
		expect(parseUint32ItemId("65536")).toMatchObject({ valid: true, value: 65536 });
		expect(parseUint32ItemId("4294967295")).toMatchObject({ valid: true, value: 4294967295 });
		expect(parseUint32ItemId("4294967296").valid).toBe(false);
		expect(parseUint32ItemId("-1").valid).toBe(false);
		expect(parseUint32ItemId("abc").valid).toBe(false);

		const index = new ItemIndex({
			ids: [100, 70000, 2_000_000_000],
			clients: [100, 80000, 3_000_000_000],
			flags: [1, 2, 1],
			tops: [0, 1, 2],
			minimap: [12, 34, 56],
			names: ["Grass", "Extended Sword", "Billion Crown"],
			kinds: [0, 0, 0],
			speeds: [100, 0, 250],
			mode: "otb",
			maxId: 2_000_000_000,
			flagbits: { ground: 1, pickupable: 2 },
		});
		expect(index.clientId(70000)).toBe(80000);
		expect(index.client[70000]).toBe(80000);
		expect(index.clientId(2_000_000_000)).toBe(3_000_000_000);
		expect(index.has(2_000_000_000)).toBe(true);
		expect(index.name(2_000_000_000)).toBe("Billion Crown");
		expect(index.topOf(2_000_000_000)).toBe(2);
		expect(index.is(2_000_000_000, "ground")).toBe(true);
		expect(index.search("2000000000", 10)).toEqual([2_000_000_000]);
	});
});
