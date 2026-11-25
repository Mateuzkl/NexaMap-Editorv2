import { describe, expect, it } from "vitest";
import {
	DEFAULT_SELECTION,
	isBrushActive,
	map,
	parseSelectionOptions,
	resolveSelectionArgs,
	selectBrush,
	setCam,
	setSelectionMode,
	toggleCompensateSelection,
	warningCount,
} from "./state.svelte";

describe("warningCount", () => {
	it("counts listed warnings and the ones summed up", () => {
		expect(warningCount(["tile 1", "tile 2", "40 more not listed: unknown item"])).toBe(42);
		expect(warningCount(undefined)).toBe(0);
	});
});

describe("setCam", () => {
	it("changes only the given camera values", () => {
		setCam(77, { x: 5 });
		expect(map.cams[77]).toEqual({ x: 5, y: 0, zoom: 1, floor: 7 });
		setCam(77, { zoom: 2 });
		expect(map.cams[77]).toEqual({ x: 5, y: 0, zoom: 2, floor: 7 });
	});
});

describe("selectBrush", () => {
	it("switches to the brush tool and remembers recent brushes once", () => {
		const grass = { kind: "ground", name: "grass", label: "Grass" };
		selectBrush(grass);
		selectBrush({ kind: "ground", name: "sand", label: "Sand" });
		selectBrush({ ...grass });
		expect(map.tool).toBe("brush");
		expect(isBrushActive(grass)).toBe(true);
		expect(map.recentBrushes.filter((brush) => brush.name === "grass")).toHaveLength(1);
		expect(map.recentBrushes[0].name).toBe("grass");
	});

	it("does not remember raw brushes without an item", () => {
		const before = map.recentBrushes.length;
		selectBrush({ kind: "raw", label: "nothing" });
		expect(map.recentBrushes).toHaveLength(before);
	});
});

describe("selection mode preferences", () => {
	it("defaults to current floor with compensate enabled and sanitizes invalid stored values", () => {
		expect(parseSelectionOptions(null)).toEqual(DEFAULT_SELECTION);
		expect(parseSelectionOptions({ mode: "bogus", compensate: "yes" })).toEqual({
			mode: "current",
			compensate: true,
		});
		expect(parseSelectionOptions({ mode: "visible", compensate: false })).toEqual({
			mode: "visible",
			compensate: false,
		});
	});

	it("propagates floors and compensate to SelectRect independently of Show lower floors", () => {
		const prevLowerFloors = map.view.lowerFloors;
		map.selection = { ...DEFAULT_SELECTION };
		expect(resolveSelectionArgs()).toEqual({ floors: "current", compensate: true });

		setSelectionMode("visible");
		toggleCompensateSelection();
		expect(resolveSelectionArgs()).toEqual({ floors: "visible", compensate: false });

		// Toggling view.lowerFloors must never overwrite or suppress the selection mode preference.
		map.view.lowerFloors = false;
		expect(resolveSelectionArgs()).toEqual({ floors: "visible", compensate: false });

		setSelectionMode("lower");
		toggleCompensateSelection();
		expect(resolveSelectionArgs()).toEqual({ floors: "lower", compensate: true });

		map.view.lowerFloors = prevLowerFloors;
		map.selection = { ...DEFAULT_SELECTION };
	});
});
