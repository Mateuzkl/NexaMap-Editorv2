import { toCss } from "./pixelscale";

export interface MenuItem {
	label: string;
	keys?: string;
	action?: () => unknown;
	disabled?: () => boolean;
	checked?: () => boolean;
	/** Render checked state as a mutually-exclusive radio marker instead of a checkbox. */
	radio?: boolean;
	/** Nested submenu entries opened beside this entry. */
	children?: MenuEntry[];
}
export type MenuEntry = MenuItem | "-";

export interface FlatMenuCommand {
	label: string;
	detail: string;
	keys?: string;
	action: () => unknown;
}

/** Flattens top-level menus and nested submenus into executable command entries. */
export function flattenMenuItems(menus: { label: string; items: MenuEntry[] }[]): FlatMenuCommand[] {
	const out: FlatMenuCommand[] = [];
	const walk = (entries: MenuEntry[], path: string) => {
		for (const entry of entries) {
			if (entry === "-" || entry.disabled?.()) continue;
			if (entry.children?.length) {
				walk(entry.children, `${path} › ${entry.label}`);
				continue;
			}
			if (!entry.action) continue;
			out.push({
				label: entry.label.replace(/…$/, ""),
				detail: path,
				keys: entry.keys,
				action: entry.action,
			});
		}
	};
	for (const menu of menus) walk(menu.items, menu.label);
	return out;
}

export const contextMenu = $state<{ items: MenuEntry[] | null; x: number; y: number }>({ items: null, x: 0, y: 0 });

export function openContextMenu(event: MouseEvent, items: MenuEntry[]): void {
	event.preventDefault();
	event.stopPropagation();
	contextMenu.items = items;
	contextMenu.x = toCss(event.clientX);
	contextMenu.y = toCss(event.clientY);
}

export function closeContextMenu(): void {
	contextMenu.items = null;
}
