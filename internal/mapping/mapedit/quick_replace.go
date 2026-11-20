package mapedit

import (
	"sort"

	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/mapping/materials"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

// QuickReplaceCandidate summarizes one distinct placed item inside a selection
// or map for the Quick Replace Items dialog.
type QuickReplaceCandidate struct {
	ID       uint16 `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// QuickReplaceChoice is a suggested replacement target item scored against the
// source item's category, item flags, brush family, and sprite appearance.
type QuickReplaceChoice struct {
	ID       uint16 `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Brush    string `json:"brush,omitempty"`
	Score    int    `json:"score"`
}

// ClassifyItem returns the Quick Replace architectural category for an item ID
// (Floor, Border, Wall, Door, Carpet, Table, Doodad, or Item).
func (e *Editor) ClassifyItem(id uint16) string {
	isGround := false
	if it := e.itemType(id); it != nil && it.IsGround() {
		isGround = true
	}
	return e.classifyPlacedItem(isGround, id)
}

var quickReplaceCategoryOrder = map[string]int{
	"Floor":  0,
	"Border": 1,
	"Wall":   2,
	"Door":   3,
	"Carpet": 4,
	"Table":  5,
	"Doodad": 6,
	"Item":   7,
}

func (e *Editor) classifyPlacedItem(isGround bool, id uint16) string {
	if isGround {
		return "Floor"
	}
	if e.Brushes != nil {
		if e.Brushes.IsDoor(id) {
			return "Door"
		}
		if e.Brushes.isBorder(id) {
			return "Border"
		}
		if _, kind := e.Brushes.BrushOf(id); kind != "" {
			switch kind {
			case materials.TypeGround:
				return "Floor"
			case materials.TypeWall, materials.TypeDecoration:
				return "Wall"
			case materials.TypeCarpet:
				return "Carpet"
			case materials.TypeTable:
				return "Table"
			case materials.TypeDoodad:
				return "Doodad"
			}
		}
	}
	if it := e.itemType(id); it != nil {
		switch {
		case it.IsGround():
			return "Floor"
		case it.IsDoor():
			return "Door"
		case it.TopOrder == borderTopOrder:
			return "Border"
		case it.TopOrder == bottomTopOrder:
			return "Wall"
		}
	}
	return "Item"
}

// CollectQuickReplaceCandidates gathers all distinct non-meta placed items
// in the active selection (or the whole map when inSelection is false),
// grouped by item ID with its architectural category and sorted by category then count.
func (e *Editor) CollectQuickReplaceCandidates(inSelection bool) []QuickReplaceCandidate {
	type entry struct {
		cat   string
		count int
	}
	byID := map[uint16]*entry{}
	add := func(isGround bool, id uint16) {
		if id == 0 {
			return
		}
		if it := e.itemType(id); it != nil && it.Flags.Has(otitems.FlagMeta) {
			return
		}
		cat := e.classifyPlacedItem(isGround, id)
		if cur, ok := byID[id]; ok {
			cur.count++
			if quickReplaceCategoryOrder[cat] < quickReplaceCategoryOrder[cur.cat] {
				cur.cat = cat
			}
			return
		}
		byID[id] = &entry{cat: cat, count: 1}
	}

	e.eachTile(inSelection, func(_ otmap.Position, tile *otmap.Tile) {
		if tile.Ground.ID != 0 {
			add(true, tile.Ground.ID)
		}
		for _, item := range tile.Items {
			add(false, item.ID)
		}
	})

	out := make([]QuickReplaceCandidate, 0, len(byID))
	for id, ent := range byID {
		out = append(out, QuickReplaceCandidate{
			ID:       id,
			Name:     e.name(id),
			Category: ent.cat,
			Count:    ent.count,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		oi, oj := quickReplaceCategoryOrder[out[i].Category], quickReplaceCategoryOrder[out[j].Category]
		if oi != oj {
			return oi < oj
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].ID < out[j].ID
	})
	return out
}
