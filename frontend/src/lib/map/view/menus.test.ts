import { describe, expect, it } from "vitest";
import type { TileInfo, TileItem } from "../data/api";
import { TileFlags, type TileData } from "../data/chunks";
import { map, toggleFavoriteBrush } from "../state.svelte";
import { mapContextMenu, tilePicks } from "./menus";

const item = (id: number, name: string, brush = "", brushKind = "", topOrder = 0) =>
	({ id, name, brush, brushKind, topOrder }) as TileItem;
const tile = (ground: TileItem | null, ...items: TileItem[]) => ({ ground, items }) as TileInfo;

describe("tilePicks", () => {
	it("picks the brush of an item instead of the raw item", () => {
		const picks = tilePicks(tile(item(103, "earth tile", "earth", "ground")));
		expect(picks.click).toMatchObject({ kind: "ground", name: "earth" });
		expect(picks.brushes.map((brush) => brush.name)).toEqual(["earth"]);
		expect(picks.raw).toMatchObject({ kind: "raw", item: 103 });
	});

	it("looks under brushless borders for the ground brush", () => {
		const picks = tilePicks(
			tile(item(103, "earth tile", "earth", "ground"), item(4500, "grass border", "", "", 1)),
		);
		expect(picks.click).toMatchObject({ kind: "ground", name: "earth" });
		expect(picks.raw).toMatchObject({ kind: "raw", item: 4500 });
	});

	it("picks a brushless loose item as is", () => {
		const picks = tilePicks(tile(item(103, "earth tile", "earth", "ground"), item(3031, "gold coin")));
		expect(picks.click).toMatchObject({ kind: "raw", item: 3031 });
		expect(picks.brushes.map((brush) => brush.name)).toEqual(["earth"]);
	});

	it("lists each brush once, topmost first, and paints decoration as a wall", () => {
		const picks = tilePicks(
			tile(
				item(103, "earth tile", "earth", "ground"),
				item(1, "fence", "fence", "decoration", 2),
				item(2, "fence", "fence", "decoration", 2),
			),
		);
		expect(picks.brushes.map((brush) => [brush.kind, brush.name])).toEqual([
			["wall", "fence"],
			["ground", "earth"],
		]);
	});

	it("picks nothing on an empty tile", () => {
		expect(tilePicks(null)).toEqual({ brushes: [], raw: null, click: null });
	});
});

describe("mapContextMenu", () => {
	it("builds right-click menu cleanly, opens Quick Replace dialog, and toggles Favorites", () => {
		map.favorites = [];
		map.dialog = null;
		map.quickReplaceAnchorItem = 0;

		const tileData: TileData = {
			flags: TileFlags.ground,
			house: 0,
			ids: new Uint32Array([103, 4500]),
			subs: new Uint16Array([0, 0]),
			itemFlags: new Uint8Array([0, 0]),
			creatures: null,
			spawn: null,
			spawnNpc: null,
			zones: null,
			text: null,
		};
		const picks = tilePicks(tile(item(103, "earth tile", "earth", "ground"), item(4500, "grass border")));
		const entries = mapContextMenu({ x: 100, y: 200, z: 7 }, tileData, picks);
		const labels = entries
			.filter((entry): entry is Exclude<typeof entry, "-"> => entry !== "-")
			.map((entry) => entry.label);

		expect(labels).toContain("Quick Replace Items…");
		expect(labels).toContain("Learn Border from Selection…");
		expect(labels).toContain("Save Terrain as Brush…");
		expect(labels).toContain("Save Terrain as .otbm…");
		expect(labels.some((label) => label.startsWith("★ Add to Favorites"))).toBe(true);

		const qr = entries.find((entry) => entry !== "-" && entry.label === "Quick Replace Items…");
		expect(qr && qr !== "-").toBe(true);
		if (qr && qr !== "-") qr.action();
		expect(map.dialog).toBe("quickReplace");
		expect(map.quickReplaceAnchorItem).toBe(4500);

		// Simulate corrupted non-array state to ensure Favorites helpers never throw
		(map as unknown as { favorites: unknown }).favorites = {};
		expect(() => mapContextMenu({ x: 100, y: 200, z: 7 }, tileData, picks)).not.toThrow();
		expect(toggleFavoriteBrush(picks.click!)).toBe(true);
		expect(Array.isArray(map.favorites)).toBe(true);
		expect(map.favorites).toHaveLength(1);
		map.favorites = [];
		map.dialog = null;
	});
});
