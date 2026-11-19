export type UITheme = "classic" | "modern-dark" | "modern-light";

export interface ThemeMeta {
	id: UITheme;
	label: string;
	shortLabel: string;
	description: string;
}

export const UI_THEMES: readonly ThemeMeta[] = [
	{
		id: "classic",
		label: "Classic Tibia",
		shortLabel: "Classic",
		description: "Original OTS Creator interface with Tibia textures, bevel frames, and pixel fonts.",
	},
	{
		id: "modern-dark",
		label: "Modern Dark",
		shortLabel: "Dark",
		description: "Clean dark workspace with sidebar navigation, client/server cards, and crisp typography.",
	},
	{
		id: "modern-light",
		label: "Modern Light",
		shortLabel: "Light",
		description: "Bright modern workspace with high-contrast light surfaces and soft borders.",
	},
];

export function normalizeTheme(value: unknown): UITheme {
	if (value === "classic" || value === "modern-dark" || value === "modern-light") {
		return value;
	}
	return "modern-dark";
}

export function isModernTheme(theme: UITheme): boolean {
	return theme === "modern-dark" || theme === "modern-light";
}

export function themeLabel(theme: UITheme): string {
	return UI_THEMES.find((entry) => entry.id === theme)?.label ?? "Modern Dark";
}

export function nextTheme(theme: UITheme): UITheme {
	const order: UITheme[] = ["modern-dark", "modern-light", "classic"];
	const idx = order.indexOf(theme);
	return order[(idx + 1) % order.length];
}

export function applyTheme(theme: UITheme): void {
	if (typeof document === "undefined") return;
	const root = document.documentElement;
	root.dataset.uiTheme = theme;
	root.style.colorScheme = theme === "modern-light" ? "light" : "dark";
}
