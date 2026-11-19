import type { ItemTable } from "./api";

export const TOP_ORDER = { common: 0, border: 1, bottom: 2, top: 3 } as const;

export const MAX_UINT32_ITEM_ID = 4294967295;
const DENSE_ITEM_LIMIT = 262144;

interface SparseItemEntry {
	client: number;
	flags: number;
	top: number;
	minimap: number;
	kind: number;
	speed: number;
	name: string;
	lowerName: string;
}

export interface ParsedUint32Id {
	valid: boolean;
	empty: boolean;
	overflow: boolean;
	value: number;
	error: string;
}

/** Validates a 32-bit unsigned decimal item/client/sprite ID (0..4294967295). */
export function parseUint32ItemId(text: string): ParsedUint32Id {
	const trimmed = text.trim();
	if (!trimmed) {
		return {
			valid: false,
			empty: true,
			overflow: false,
			value: 0,
			error: "Enter an ID between 0 and 4294967295.",
		};
	}
	if (trimmed.startsWith("-")) {
		return {
			valid: false,
			empty: false,
			overflow: false,
			value: 0,
			error: "ID cannot be negative (valid range: 0 to 4294967295).",
		};
	}
	if (!/^\d+$/.test(trimmed)) {
		return {
			valid: false,
			empty: false,
			overflow: false,
			value: 0,
			error: "ID must contain only decimal digits (0-9).",
		};
	}
	const num = Number(trimmed);
	if (!Number.isSafeInteger(num) || num > MAX_UINT32_ITEM_ID) {
		return {
			valid: false,
			empty: false,
			overflow: true,
			value: 0,
			error: "ID exceeds 32-bit maximum (4294967295).",
		};
	}
	return { valid: true, empty: false, overflow: false, value: num, error: "" };
}

export class ItemIndex {
	readonly maxId: number;
	readonly denseCount: number;
	readonly client: Uint32Array;
	readonly flags: Uint32Array;
	readonly top: Uint8Array;
	readonly minimap: Uint16Array;
	readonly kind: Uint8Array;
	/** Ground speed (0: not a ground or no speed). */
	readonly speed: Uint16Array;
	readonly exists: Uint8Array;
	readonly names: string[];
	readonly ids: number[];
	readonly bits: Record<string, number>;
	readonly mode: string;
	private lowercaseNames: string[];
	private sparse = new Map<number, SparseItemEntry>();

	constructor(table: ItemTable) {
		this.maxId = table.maxId ?? 0;
		const count = Math.min(this.maxId, DENSE_ITEM_LIMIT) + 1;
		this.denseCount = count;
		this.client = new Uint32Array(count);
		this.flags = new Uint32Array(count);
		this.top = new Uint8Array(count);
		this.minimap = new Uint16Array(count);
		this.kind = new Uint8Array(count);
		this.speed = new Uint16Array(count);
		this.exists = new Uint8Array(count);
		this.names = new Array(count);
		this.lowercaseNames = new Array(count);
		this.ids = table.ids ?? [];
		this.bits = (table.flagbits ?? {}) as Record<string, number>;
		this.mode = table.mode;
		const ids = table.ids ?? [];
		for (let i = 0; i < ids.length; i++) {
			const id = ids[i];
			const client = table.clients?.[i] ?? 0;
			const flags = table.flags?.[i] ?? 0;
			const top = table.tops?.[i] ?? 0;
			const minimap = table.minimap?.[i] ?? 0;
			const kind = table.kinds?.[i] ?? 0;
			const speed = table.speeds?.[i] ?? 0;
			const name = table.names?.[i] ?? "";
			const lowerName = name.toLowerCase();
			if (id < count) {
				this.client[id] = client;
				this.flags[id] = flags;
				this.top[id] = top;
				this.minimap[id] = minimap;
				this.kind[id] = kind;
				this.speed[id] = speed;
				this.exists[id] = 1;
				this.names[id] = name;
				this.lowercaseNames[id] = lowerName;
			} else {
				this.sparse.set(id, { client, flags, top, minimap, kind, speed, name, lowerName });
			}
		}
	}

	has(id: number): boolean {
		if (!(id > 0 && id <= this.maxId)) return false;
		if (id < this.denseCount) return this.exists[id] === 1;
		return this.sparse.has(id);
	}

	clientOf(id: number): number {
		if (id < this.denseCount) return this.client[id] ?? 0;
		return this.sparse.get(id)?.client ?? 0;
	}

	clientId(id: number): number {
		return this.clientOf(id);
	}

	topOf(id: number): number {
		if (id < this.denseCount) return this.top[id] ?? 0;
		return this.sparse.get(id)?.top ?? 0;
	}

	is(id: number, flag: string): boolean {
		if (id > this.maxId) return false;
		const mask = this.bits[flag] ?? 0;
		if (id < this.denseCount) return (this.flags[id] & mask) !== 0;
		return ((this.sparse.get(id)?.flags ?? 0) & mask) !== 0;
	}

	name(id: number): string {
		let name = "";
		if (id <= this.maxId) {
			name = id < this.denseCount ? (this.names[id] ?? "") : (this.sparse.get(id)?.name ?? "");
		}
		return name || `item ${id}`;
	}

	private lowerNameOf(id: number): string {
		if (id < this.denseCount) return this.lowercaseNames[id] ?? "";
		return this.sparse.get(id)?.lowerName ?? "";
	}

	search(query: string, limit = 500): number[] {
		const text = query.trim().toLowerCase();
		if (!text) return [];
		const out: number[] = [];
		const parsed = parseUint32ItemId(text);
		const queryId = parsed.valid ? parsed.value : NaN;
		if (this.has(queryId)) out.push(queryId);
		const range = text.match(/^(\d+)\s*-\s*(\d+)$/);
		if (range) {
			const fromId = Number(range[1]);
			const toId = Math.min(Number(range[2]), this.maxId);
			if (toId - fromId > DENSE_ITEM_LIMIT) {
				for (const id of this.ids) {
					if (out.length >= limit) break;
					if (id >= fromId && id <= toId && id !== queryId) out.push(id);
				}
			} else {
				for (let id = fromId; id <= toId && out.length < limit; id++) {
					if (this.has(id) && id !== queryId) out.push(id);
				}
			}
			return out;
		}
		for (const id of this.ids) {
			if (out.length >= limit) break;
			if (id !== queryId && this.lowerNameOf(id).includes(text)) out.push(id);
		}
		return out;
	}
}

/** Rgb of each 8-bit client color, three bytes each: a 6x6x6 cube, black at 0 and from 216 (client: Color::from8bit). */
export const CLIENT_COLOR_RGB: Uint8Array = (() => {
	const table = new Uint8Array(256 * 3);
	for (let color = 1; color < 216; color++) {
		table[color * 3] = Math.floor(color / 36) * 51;
		table[color * 3 + 1] = (Math.floor(color / 6) % 6) * 51;
		table[color * 3 + 2] = (color % 6) * 51;
	}
	return table;
})();

/** Offset of an 8-bit client color in CLIENT_COLOR_RGB; colors outside the cube are black. */
export function clientColorOffset(color: number): number {
	return color > 0 && color < 216 ? color * 3 : 0;
}

export function minimapCss(color: number): string {
	if (color >= 216) return "#000";
	const offset = clientColorOffset(color);
	return `rgb(${CLIENT_COLOR_RGB[offset]},${CLIENT_COLOR_RGB[offset + 1]},${CLIENT_COLOR_RGB[offset + 2]})`;
}
