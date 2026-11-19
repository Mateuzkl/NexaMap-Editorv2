import { describe, expect, it } from "vitest";
import { applyTheme, isModernTheme, nextTheme, normalizeTheme, themeLabel } from "./theme.svelte";

describe("UI theme helpers", () => {
	it("normalizes valid and invalid theme values safely", () => {
		expect(normalizeTheme("classic")).toBe("classic");
		expect(normalizeTheme("modern-dark")).toBe("modern-dark");
		expect(normalizeTheme("modern-light")).toBe("modern-light");
		expect(normalizeTheme("")).toBe("modern-dark");
		expect(normalizeTheme("unknown")).toBe("modern-dark");
		expect(normalizeTheme(null)).toBe("modern-dark");
	});

	it("identifies modern themes vs classic", () => {
		expect(isModernTheme("classic")).toBe(false);
		expect(isModernTheme("modern-dark")).toBe(true);
		expect(isModernTheme("modern-light")).toBe(true);
	});

	it("cycles themes and returns readable labels", () => {
		expect(nextTheme("modern-dark")).toBe("modern-light");
		expect(nextTheme("modern-light")).toBe("classic");
		expect(nextTheme("classic")).toBe("modern-dark");
		expect(themeLabel("classic")).toBe("Classic Tibia");
		expect(themeLabel("modern-dark")).toBe("Modern Dark");
		expect(themeLabel("modern-light")).toBe("Modern Light");
	});

	it("applies data-ui-theme attribute on documentElement when DOM exists", () => {
		const dataset: Record<string, string> = {};
		const style: Record<string, string> = {};
		const prevDoc = globalThis.document;
		Object.defineProperty(globalThis, "document", {
			configurable: true,
			value: { documentElement: { dataset, style } },
		});
		try {
			applyTheme("modern-light");
			expect(dataset.uiTheme).toBe("modern-light");
			expect(style.colorScheme).toBe("light");
			applyTheme("classic");
			expect(dataset.uiTheme).toBe("classic");
			expect(style.colorScheme).toBe("dark");
		} finally {
			Object.defineProperty(globalThis, "document", { configurable: true, value: prevDoc });
		}
	});
});
