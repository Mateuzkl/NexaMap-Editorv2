package app

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/mapping/mapedit"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

type mapTool struct {
	run       func(editor *mapedit.Editor, inSelection bool) int
	redrawAll bool
}

var mapTools = map[string]mapTool{
	"borderizeSelection": {run: func(editor *mapedit.Editor, _ bool) int { return editor.BorderizeSelection() }},
	"borderizeMap":       {run: func(editor *mapedit.Editor, _ bool) int { return editor.BorderizeMap() }},
	"randomizeSelection": {run: func(editor *mapedit.Editor, _ bool) int { return editor.RandomizeSelection() }},
	"randomizeMap":       {run: func(editor *mapedit.Editor, _ bool) int { return editor.RandomizeMap() }},
	"removeCorpses":      {run: (*mapedit.Editor).RemoveCorpses},
	"removeInvalid":      {run: (*mapedit.Editor).RemoveInvalid},
	"removeDuplicates":   {run: (*mapedit.Editor).RemoveDuplicates},
	"removeUnreachable":  {run: func(editor *mapedit.Editor, _ bool) int { return editor.RemoveUnreachable() }},
	"removeEmptySpawns":  {run: func(editor *mapedit.Editor, _ bool) int { return editor.RemoveEmptySpawns() }},
	"clearInvalidHouses": {run: func(editor *mapedit.Editor, _ bool) int { return editor.ClearInvalidHouses() }},
	"clearModified": {
		run:       func(editor *mapedit.Editor, _ bool) int { editor.ClearModified(); return 0 },
		redrawAll: true,
	},
}

// Tool runs a map tool: "borderizeSelection", "borderizeMap",
// "randomizeSelection", "randomizeMap", "removeCorpses", "removeInvalid",
// "removeUnreachable", "removeEmptySpawns", "clearInvalidHouses",
// "clearModified". inSelection limits item tools to the selection. Tools
// that go through the whole map send EventMapProgress.
func (ms *MapService) Tool(id int, name string, inSelection bool) (MapChange, error) {
	tool, ok := mapTools[name]
	if !ok {
		return MapChange{}, fmt.Errorf("unknown tool %q", name)
	}
	report := ms.mapProgress()
	report("Going through the map", 0)
	change, err := ms.edit(id, func(editor *mapedit.Editor, _ *mapData) (int, error) {
		editor.Progress = func(done, total int) {
			report("Going through the map", progressTotal*done/max(total, 1))
		}
		defer func() { editor.Progress = nil }()
		return tool.run(editor, inSelection), nil
	})
	change.All = change.All || tool.redrawAll
	return change, err
}

// FindResult lists found items; More is set when the limit was reached.
type FindResult struct {
	Results []mapedit.FindResult `json:"results"`
	More    bool                 `json:"more"`
}

func (ms *MapService) Find(id int, query mapedit.FindQuery) (found FindResult, err error) {
	err = ms.withMapRead(id, func(opened *openMap, data *mapData) error {
		if query.SpriteID != 0 {
			clientIDs := map[uint32]bool{}
			if data.project != nil {
				for _, t := range data.project.ItemsSnapshot() {
					if t == nil {
						continue
					}
					for _, g := range t.FrameGroups {
						if g != nil && slices.Contains(g.Sprites, query.SpriteID) {
							clientIDs[t.ID] = true
							break
						}
					}
				}
			}
			if len(clientIDs) == 0 || data.items == nil {
				found.Results = []mapedit.FindResult{}
				return nil
			}
			var serverIDs []uint16
			data.items.Each(func(it *otitems.Type) {
				if it.EffectiveID() <= 0xFFFF && clientIDs[it.EffectiveClientID()] {
					serverIDs = append(serverIDs, uint16(it.EffectiveID()))
				}
			})
			if len(serverIDs) == 0 {
				found.Results = []mapedit.FindResult{}
				return nil
			}
			query.IDs = serverIDs
		}
		found.Results, found.More = opened.editor.Find(query)
		return nil
	})
	return found, err
}

// Replace replaces items (to 0 removes them).
func (ms *MapService) Replace(id int, from, to []uint16, inSelection bool) (MapChange, error) {
	if len(from) != len(to) {
		return MapChange{}, errors.New("replace needs pairs of ids")
	}
	replacements := map[uint16]uint16{}
	for i := range from {
		replacements[from[i]] = to[i]
	}
	return ms.edit(id, func(editor *mapedit.Editor, _ *mapData) (int, error) {
		return editor.ReplaceItems(replacements, inSelection), nil
	})
}

// QuickReplaceCandidates returns every distinct item placed in the selection
// (or the whole map) with its category and occurrence count.
func (ms *MapService) QuickReplaceCandidates(id int, inSelection bool) ([]mapedit.QuickReplaceCandidate, error) {
	var out []mapedit.QuickReplaceCandidate
	err := ms.withMapRead(id, func(opened *openMap, _ *mapData) error {
		out = opened.editor.CollectQuickReplaceCandidates(inSelection)
		return nil
	})
	if out == nil {
		out = []mapedit.QuickReplaceCandidate{}
	}
	return out, err
}

// QuickReplaceChoices returns replacement target items scored (0..100%) by
// item category, flags, brush family, map co-occurrence, and sprite similarity.
func (ms *MapService) QuickReplaceChoices(id int, sourceID uint16, category string, sameCategory bool, query string, limit int) ([]mapedit.QuickReplaceChoice, error) {
	if limit <= 0 || limit > 400 {
		limit = 160
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var out []mapedit.QuickReplaceChoice
	err := ms.withMapRead(id, func(opened *openMap, data *mapData) error {
		editor := opened.editor
		items := editor.Items
		if items == nil {
			return nil
		}
		baseType := items.Get(sourceID)
		baseCat := category
		if baseCat == "" && sourceID != 0 {
			baseCat = editor.ClassifyItem(sourceID)
		}
		var baseBrush string
		if editor.Brushes != nil && sourceID != 0 {
			baseBrush, _ = editor.Brushes.BrushOf(sourceID)
		}
		baseTokens := map[string]bool{}
		if baseType != nil {
			for _, tok := range strings.Fields(strings.ToLower(baseType.Name)) {
				if len(tok) >= 3 {
					baseTokens[tok] = true
				}
			}
		}

		subScore := map[uint16]float64{}
		if opened.suggestions != nil && sourceID != 0 {
			for _, s := range opened.suggestions.Substitutes(sourceID, 64) {
				subScore[s.ID] = s.Score
			}
		}

		type cand struct {
			id       uint16
			name     string
			cat      string
			brush    string
			preScore float64
		}
		var pool []cand
		items.Each(func(it *otitems.Type) {
			eid := it.EffectiveID()
			if eid == 0 || eid > 0xFFFF || it.EffectiveClientID() == 0 || it.Flags.Has(otitems.FlagMeta) {
				return
			}
			itemID := uint16(eid)
			if itemID == sourceID {
				return
			}
			cat := editor.ClassifyItem(itemID)
			if sameCategory && baseCat != "" && cat != baseCat {
				return
			}
			var brushName string
			if editor.Brushes != nil {
				brushName, _ = editor.Brushes.BrushOf(itemID)
			}
			name := it.Name
			if name == "" {
				name = fmt.Sprintf("item %d", itemID)
			}
			if q != "" {
				idStr := strconv.Itoa(int(itemID))
				if !strings.Contains(strings.ToLower(name), q) &&
					!strings.Contains(idStr, q) &&
					!strings.Contains(strings.ToLower(brushName), q) {
					return
				}
			}

			score := 20.0
			if cat == baseCat && baseCat != "" {
				score += 22.0
			}
			if baseType != nil {
				if it.TopOrder == baseType.TopOrder {
					score += 10.0
				}
				if it.Group == baseType.Group {
					score += 6.0
				}
				const structural = otitems.FlagGround | otitems.FlagUnpassable | otitems.FlagHangable |
					otitems.FlagHookEast | otitems.FlagHookSouth | otitems.FlagPickupable | otitems.FlagRotatable
				if it.Flags&structural == baseType.Flags&structural {
					score += 12.0
				} else if it.Flags.Has(otitems.FlagUnpassable) == baseType.Flags.Has(otitems.FlagUnpassable) {
					score += 5.0
				}
			}
			if baseBrush != "" && brushName == baseBrush {
				score += 14.0
			} else if brushName != "" {
				score += 3.0
			}
			if len(baseTokens) > 0 && it.Name != "" {
				matches := 0
				for _, tok := range strings.Fields(strings.ToLower(it.Name)) {
					if baseTokens[tok] {
						matches++
					}
				}
				if matches > 0 {
					score += math.Min(12.0, float64(matches)*6.0)
				}
			}
			if ss, ok := subScore[itemID]; ok {
				score += 8.0 + math.Min(10.0, ss*10.0)
			}
			// Nearby item IDs in Tibia/OT clients are very often direct variations/themes of the same object.
			if sourceID != 0 {
				diff := int(itemID) - int(sourceID)
				if diff < 0 {
					diff = -diff
				}
				if diff <= 120 {
					score += 8.0 * (1.0 - float64(diff)/120.0)
				}
			}
			pool = append(pool, cand{id: itemID, name: name, cat: cat, brush: brushName, preScore: score})
		})

		slices.SortFunc(pool, func(a, b cand) int {
			if a.preScore != b.preScore {
				if a.preScore > b.preScore {
					return -1
				}
				return 1
			}
			return int(a.id) - int(b.id)
		})

		// Refine the top slice with real sprite color/coverage/size visual similarity.
		refineCap := min(len(pool), max(limit*2, 320))
		refineIDs := make([]uint16, refineCap)
		for i := 0; i < refineCap; i++ {
			refineIDs[i] = pool[i].id
		}
		vis := ms.looks.visualScores(data, items, sourceID, refineIDs)

		out = make([]mapedit.QuickReplaceChoice, 0, refineCap)
		for i := 0; i < refineCap; i++ {
			c := pool[i]
			combined := c.preScore
			if sim, ok := vis[c.id]; ok {
				combined = 0.62*math.Min(100, c.preScore) + 0.38*(sim*100.0)
			}
			pct := int(math.Round(combined))
			if pct < 1 {
				pct = 1
			}
			if pct > 99 {
				pct = 99
			}
			out = append(out, mapedit.QuickReplaceChoice{
				ID:       c.id,
				Name:     c.name,
				Category: c.cat,
				Brush:    c.brush,
				Score:    pct,
			})
		}
		slices.SortFunc(out, func(a, b mapedit.QuickReplaceChoice) int {
			if a.Score != b.Score {
				return b.Score - a.Score
			}
			return int(a.ID) - int(b.ID)
		})
		if len(out) > limit {
			out = out[:limit]
		}
		return nil
	})
	if out == nil {
		out = []mapedit.QuickReplaceChoice{}
	}
	return out, err
}

// SetSpawnTimes sets the spawn time of the creatures in the selection or
// the whole map; a non-empty name limits it to creatures of that name.
// Count is the number of creatures changed.
func (ms *MapService) SetSpawnTimes(id int, seconds int32, name string, inSelection bool) (MapChange, error) {
	return ms.edit(id, func(editor *mapedit.Editor, _ *mapData) (int, error) {
		return editor.SetSpawnTimes(seconds, name, inSelection)
	})
}

func (ms *MapService) Stats(id int) (stats mapedit.Statistics, err error) {
	err = ms.withMapRead(id, func(opened *openMap, _ *mapData) error {
		stats = opened.editor.Stats()
		return nil
	})
	return stats, err
}

// FindPath searches the way a player walks between two tiles (see
// mapedit.Editor.FindPath).
func (ms *MapService) FindPath(id int, from, to otmap.Position, limit int) (path mapedit.PathResult, err error) {
	err = ms.withMapRead(id, func(opened *openMap, _ *mapData) error {
		path, err = opened.editor.FindPath(from, to, limit)
		return err
	})
	return path, err
}

// Validate looks for mistakes that break a map in game (see
// mapedit.Editor.Validate).
func (ms *MapService) Validate(id int, limit int) (validation mapedit.Validation, err error) {
	err = ms.withMapRead(id, func(opened *openMap, _ *mapData) error {
		validation = opened.editor.Validate(limit)
		return nil
	})
	return validation, err
}
