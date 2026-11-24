package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/mapping/mapedit"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

func TestMapServiceMultiFloorCopyPasteBetweenMapsSaveReopen(t *testing.T) {
	ms, _ := mapTestService(t)
	srcInfo, err := ms.NewMap(NewMapOptions{Width: 128, Height: 128, ItemIDs: ItemIDsChoice{Mode: otitems.ModeClient}})
	if err != nil {
		t.Fatal(err)
	}
	dstInfo, err := ms.NewMap(NewMapOptions{Width: 128, Height: 128, ItemIDs: ItemIDsChoice{Mode: otitems.ModeClient}})
	if err != nil {
		t.Fatal(err)
	}

	// Build a 3-story house (Z=5, 6, 7) on source map with ground, stacked item, house, flags, creature & spawn.
	srcTown := mustChange(t)(ms.SaveTown(srcInfo.ID, otmap.Town{Name: "Origin", Temple: otmap.Position{X: 20, Y: 20, Z: 7}}))
	srcHouse := mustChange(t)(ms.SaveHouse(srcInfo.ID, otmap.House{Name: "Src House", TownID: uint32(srcTown.Count)}))
	dstTown := mustChange(t)(ms.SaveTown(dstInfo.ID, otmap.Town{Name: "Dest", Temple: otmap.Position{X: 50, Y: 50, Z: 7}}))
	dstHouse := mustChange(t)(ms.SaveHouse(dstInfo.ID, otmap.House{Name: "Existing Target House", TownID: uint32(dstTown.Count)}))

	for z := uint8(5); z <= 7; z++ {
		for y := uint16(20); y <= 21; y++ {
			for x := uint16(20); x <= 21; x++ {
				pos := otmap.Position{X: x, Y: y, Z: z}
				mustChange(t)(ms.Draw(srcInfo.ID, mapedit.DrawRequest{Kind: mapedit.KindRaw, Item: 100, Points: []otmap.Position{pos}}))
				mustChange(t)(ms.Draw(srcInfo.ID, mapedit.DrawRequest{Kind: mapedit.KindRaw, Item: 101, Points: []otmap.Position{pos}}))
				mustChange(t)(ms.UpdateTile(srcInfo.ID, pos, mapedit.TilePatch{
					Flags:   ptr(otmap.FlagPZ),
					HouseID: ptr(uint32(srcHouse.Count)),
				}))
			}
		}
	}
	// Mark an item attribute on Z=6 so save/reopen proves attributes survive too.
	mustChange(t)(ms.UpdateItem(srcInfo.ID, mapedit.ItemRef{Pos: otmap.Position{X: 20, Y: 20, Z: 6}, Index: 0}, mapedit.ItemPatch{
		ActionID: ptr(uint16(777)),
	}))

	// Select all visible floors from Z=7 (non-compensated so exact 2x2x3 column is selected).
	rect := mapedit.Rect{
		From: otmap.Position{X: 20, Y: 20, Z: 7},
		To:   otmap.Position{X: 21, Y: 21, Z: 7},
	}
	sel := mustChange(t)(ms.SelectRect(srcInfo.ID, rect, mapedit.FloorsVisible, false, false, false))
	if sel.Count != 12 {
		t.Fatalf("SelectRect(Visible) count = %d, want 12 (2x2 across floors 5..7)", sel.Count)
	}
	copied, err := ms.CopyAt(srcInfo.ID, 7)
	if err != nil || copied != 12 {
		t.Fatalf("CopyAt(7) = %d, %v; want 12, nil", copied, err)
	}
	clip, err := ms.ClipboardFor(dstInfo.ID)
	if err != nil || clip == nil || clip.OriginZ != 7 || clip.MinDZ != -2 || clip.MaxDZ != 0 {
		t.Fatalf("ClipboardFor(dst) = %+v, %v", clip, err)
	}

	// Paste into destination map at Z=7: floors 5, 6, 7 must land at 5, 6, 7 and foreign house ID must be stripped.
	pasted := mustChange(t)(ms.Paste(dstInfo.ID, otmap.Position{X: 60, Y: 60, Z: 7}))
	if pasted.Count != 12 {
		t.Fatalf("Paste count = %d, want 12", pasted.Count)
	}
	if len(pasted.Warnings) == 0 || !strings.Contains(pasted.Warnings[0], "house IDs") {
		t.Fatalf("expected cross-map house warning, got %v", pasted.Warnings)
	}
	for z := uint8(5); z <= 7; z++ {
		for dy := uint16(0); dy <= 1; dy++ {
			for dx := uint16(0); dx <= 1; dx++ {
				pos := otmap.Position{X: 60 + dx, Y: 60 + dy, Z: z}
				tile, err := ms.Tile(dstInfo.ID, pos)
				if err != nil || !tile.Exists || tile.Ground == nil || tile.Ground.ID != 100 || len(tile.Items) != 1 || tile.Items[0].ID != 101 {
					t.Fatalf("dst tile %v = %+v, %v", pos, tile, err)
				}
				if tile.Flags != otmap.FlagPZ {
					t.Fatalf("dst tile %v flags = %d, want FlagPZ", pos, tile.Flags)
				}
				if tile.HouseID != 0 {
					t.Fatalf("dst tile %v houseID = %d, want 0 (must not collide with target house %d)", pos, tile.HouseID, dstHouse.Count)
				}
			}
		}
	}
	midTile, err := ms.Tile(dstInfo.ID, otmap.Position{X: 60, Y: 60, Z: 6})
	if err != nil || midTile.Items[0].Ext == nil || midTile.Items[0].Ext.ActionID != 777 {
		t.Fatalf("dst (60,60,6) item ext = %+v, want ActionID 777", midTile)
	}

	// Undo/Redo on destination map acts as one atomic operation across all 3 floors.
	mustChange(t)(ms.Undo(dstInfo.ID))
	for z := uint8(5); z <= 7; z++ {
		tile, _ := ms.Tile(dstInfo.ID, otmap.Position{X: 60, Y: 60, Z: z})
		if tile.Exists {
			t.Fatalf("dst floor %d still exists after single Undo", z)
		}
	}
	mustChange(t)(ms.Redo(dstInfo.ID))
	for z := uint8(5); z <= 7; z++ {
		tile, _ := ms.Tile(dstInfo.ID, otmap.Position{X: 60, Y: 60, Z: z})
		if !tile.Exists || tile.Ground == nil || tile.Ground.ID != 100 {
			t.Fatalf("dst floor %d missing after single Redo", z)
		}
	}

	// Save destination map as OTBM, close, reopen and verify all 3 floors & attributes persist.
	outPath := filepath.Join(t.TempDir(), "multi-floor.otbm")
	mustChange(t)(ms.SaveMap(dstInfo.ID, outPath))
	ms.CloseMap(dstInfo.ID)
	reopened, err := ms.OpenMap(outPath, ItemIDsChoice{Mode: otitems.ModeClient})
	if err != nil {
		t.Fatal(err)
	}
	for z := uint8(5); z <= 7; z++ {
		tile, err := ms.Tile(reopened.Info.ID, otmap.Position{X: 60, Y: 60, Z: z})
		if err != nil || !tile.Exists || tile.Ground == nil || tile.Ground.ID != 100 || len(tile.Items) != 1 || tile.Items[0].ID != 101 {
			t.Fatalf("reopened tile floor %d = %+v, %v", z, tile, err)
		}
	}
	reopenedMid, _ := ms.Tile(reopened.Info.ID, otmap.Position{X: 60, Y: 60, Z: 6})
	if reopenedMid.Items[0].Ext == nil || reopenedMid.Items[0].Ext.ActionID != 777 {
		t.Fatalf("reopened (60,60,6) ActionID lost: %+v", reopenedMid.Items[0].Ext)
	}

	// Same-map paste back into srcInfo still preserves srcHouse (cross-map paste did not mutate clipboard).
	sameMapPaste := mustChange(t)(ms.Paste(srcInfo.ID, otmap.Position{X: 80, Y: 80, Z: 7}))
	if len(sameMapPaste.Warnings) != 0 {
		t.Fatalf("unexpected warning on same-map paste: %v", sameMapPaste.Warnings)
	}
	sameTile, _ := ms.Tile(srcInfo.ID, otmap.Position{X: 80, Y: 80, Z: 7})
	if sameTile.HouseID != uint32(srcHouse.Count) {
		t.Fatalf("same-map pasted houseID = %d, want %d", sameTile.HouseID, srcHouse.Count)
	}
}

func TestMapServiceMultiFloorCutAtAndIncompatibleMode(t *testing.T) {
	ms, _ := mapTestService(t)
	clientMap, err := ms.NewMap(NewMapOptions{Width: 64, Height: 64, ItemIDs: ItemIDsChoice{Mode: otitems.ModeClient}})
	if err != nil {
		t.Fatal(err)
	}
	for z := uint8(0); z <= 7; z++ {
		pos := otmap.Position{X: 15, Y: 15, Z: z}
		mustChange(t)(ms.Draw(clientMap.ID, mapedit.DrawRequest{Kind: mapedit.KindRaw, Item: 100, Points: []otmap.Position{pos}}))
	}
	rect := mapedit.Rect{From: otmap.Position{X: 15, Y: 15, Z: 7}, To: otmap.Position{X: 15, Y: 15, Z: 7}}
	mustChange(t)(ms.SelectRect(clientMap.ID, rect, mapedit.FloorsVisible, false, false, false))
	cut := mustChange(t)(ms.CutAt(clientMap.ID, 7))
	if cut.Count != 8 {
		t.Fatalf("CutAt(7) count = %d, want 8", cut.Count)
	}
	// Out-of-bounds paste at Z=3 (minDz=-7 -> floor -4) fails cleanly without changing the map.
	if _, err := ms.Paste(clientMap.ID, otmap.Position{X: 30, Y: 30, Z: 3}); err == nil {
		t.Fatal("expected out-of-bounds Paste at Z=3 to fail")
	}
	// Valid paste at Z=7 puts floors 0..7 back on 0..7.
	mustChange(t)(ms.Paste(clientMap.ID, otmap.Position{X: 30, Y: 30, Z: 7}))
	for z := uint8(0); z <= 7; z++ {
		tile, _ := ms.Tile(clientMap.ID, otmap.Position{X: 30, Y: 30, Z: z})
		if !tile.Exists {
			t.Fatalf("floor %d missing after CutAt+Paste", z)
		}
	}
	// Check ClipboardFor detects ID mode mismatch cleanly.
	ms.mu.Lock()
	ms.clipboardMode = otitems.ModeOTB
	ms.mu.Unlock()
	if _, err := ms.ClipboardFor(clientMap.ID); err == nil || !strings.Contains(err.Error(), "different item ID mode") {
		t.Fatalf("ClipboardFor mismatch err = %v, want mode mismatch error", err)
	}
	if _, err := ms.Paste(clientMap.ID, otmap.Position{X: 40, Y: 40, Z: 7}); err == nil || !strings.Contains(err.Error(), "different item ID mode") {
		t.Fatalf("Paste mismatch err = %v, want mode mismatch error", err)
	}
}
