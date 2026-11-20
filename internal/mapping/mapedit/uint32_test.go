package mapedit

import (
	"encoding/json"
	"testing"

	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

func TestUint32ExtendedItemIDs(t *testing.T) {
	const (
		highGround uint32 = 66620
		highItem   uint32 = 66625
		highItem2  uint32 = 66630
	)
	db := otitems.NewDB(otitems.ModeOTB, []*otitems.Type{
		{
			ID32:       highGround,
			ClientID32: highGround,
			Name:       "extended ground",
			Flags:      otitems.FlagGround,
			Group:      otitems.GroupGround,
		},
		{
			ID32:       highItem,
			ClientID32: highItem,
			Name:       "extended statue",
		},
		{
			ID32:       highItem2,
			ClientID32: highItem2,
			Name:       "extended pillar",
		},
	})
	e := New(otmap.New(256, 256), db, NewBrushes(db, nil))

	// Unmarshal JSON like Wails frontend sends for item 66625.
	var req DrawRequest
	rawJSON := `{"kind":"raw","item":66625,"points":[{"x":20,"y":20,"z":7}]}`
	if err := json.Unmarshal([]byte(rawJSON), &req); err != nil {
		t.Fatalf("unmarshal DrawRequest with item 66625: %v", err)
	}
	if req.Item != highItem {
		t.Fatalf("req.Item = %d, want %d", req.Item, highItem)
	}

	// Draw ground > 65535 and item > 65535 on the same tile.
	if _, err := e.Draw(DrawRequest{Kind: KindRaw, Item: highGround, Points: points(at(20, 20))}); err != nil {
		t.Fatalf("Draw highGround: %v", err)
	}
	if _, err := e.Draw(req); err != nil {
		t.Fatalf("Draw highItem: %v", err)
	}

	tile := e.Map.Tile(at(20, 20))
	if tile == nil || tile.Ground.ID != highGround {
		t.Fatalf("tile ground = %+v, want %d", tile, highGround)
	}
	if len(tile.Items) != 1 || tile.Items[0].ID != highItem {
		t.Fatalf("tile items = %+v, want [%d]", tile.Items, highItem)
	}

	// Find must locate 32-bit item IDs accurately.
	found, _ := e.Find(FindQuery{IDs: []uint32{highItem}})
	if len(found) != 1 || found[0].ID != highItem || found[0].Name != "extended statue" {
		t.Fatalf("Find(66625) = %+v, want 1 result with ID 66625", found)
	}

	// QuickReplace candidate scan must preserve 32-bit IDs.
	cands := e.CollectQuickReplaceCandidates(false)
	seenHigh := false
	for _, c := range cands {
		if c.ID == highItem && c.Count == 1 {
			seenHigh = true
		}
	}
	if !seenHigh {
		t.Fatalf("CollectQuickReplaceCandidates missing %d: %+v", highItem, cands)
	}

	// ReplaceItems must replace 66625 -> 66630 and support Undo.
	if replaced := e.ReplaceItems(map[uint32]uint32{highItem: highItem2}, false); replaced != 1 {
		t.Fatalf("ReplaceItems replaced %d, want 1", replaced)
	}
	if got := e.Map.Tile(at(20, 20)).Items[0].ID; got != highItem2 {
		t.Fatalf("after replace ID = %d, want %d", got, highItem2)
	}
	e.Undo()
	if got := e.Map.Tile(at(20, 20)).Items[0].ID; got != highItem {
		t.Fatalf("after undo ID = %d, want %d", got, highItem)
	}

	// AddItem and UpdateItem must handle > 65535 IDs cleanly.
	if err := e.AddItem(at(21, 20), highItem); err != nil {
		t.Fatalf("AddItem(66625): %v", err)
	}
	if got := e.Map.Tile(at(21, 20)).Items[0].ID; got != highItem {
		t.Fatalf("AddItem tile ID = %d, want %d", got, highItem)
	}
	newID := highItem2
	if err := e.UpdateItem(ItemRef{Pos: at(21, 20), Index: 0}, ItemPatch{ID: &newID}); err != nil {
		t.Fatalf("UpdateItem(66630): %v", err)
	}
	if got := e.Map.Tile(at(21, 20)).Items[0].ID; got != highItem2 {
		t.Fatalf("UpdateItem tile ID = %d, want %d", got, highItem2)
	}
}
