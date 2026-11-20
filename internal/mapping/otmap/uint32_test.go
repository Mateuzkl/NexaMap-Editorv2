package otmap

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nekiro/ots-creator/internal/core/otitems"
)

func TestUint32ExtendedItemIDsRoundTripAndOTBMGuard(t *testing.T) {
	const (
		highGround uint32 = 66620
		highItem   uint32 = 66625
	)
	db := otitems.NewDB(otitems.ModeOTB, []*otitems.Type{
		{ID32: highGround, ClientID32: highGround, Flags: otitems.FlagGround, Group: otitems.GroupGround},
		{ID32: highItem, ClientID32: highItem},
	})

	m := New(256, 256)
	pos := Position{X: 32, Y: 48, Z: 7}
	m.SetTile(pos, &Tile{
		Ground: Item{ID: highGround},
		Items:  []Item{{ID: highItem}},
	})

	// Compacting/packing must keep tiles with ID > 65535 lossless (never wrapping 66625 -> 1089).
	tile := m.Tile(pos)
	if tile == nil || tile.Ground.ID != highGround || len(tile.Items) != 1 || tile.Items[0].ID != highItem {
		t.Fatalf("tile after SetTile = %+v, want ground %d and item %d", tile, highGround, highItem)
	}

	// Text format (.otmap) must save and load 32-bit IDs losslessly.
	otmapPath := filepath.Join(t.TempDir(), "extended.otmap")
	if _, err := Save(otmapPath, m, db, Format{}); err != nil {
		t.Fatalf("Save .otmap: %v", err)
	}
	loaded, _, err := Load(otmapPath, db, Format{})
	if err != nil {
		t.Fatalf("Load .otmap: %v", err)
	}
	got := loaded.Tile(pos)
	if got == nil || got.Ground.ID != highGround || len(got.Items) != 1 || got.Items[0].ID != highItem {
		t.Fatalf("reloaded tile = %+v, want ground %d and item %d", got, highGround, highItem)
	}

	// Standard 16-bit OTBM must refuse IDs > 65535 with a clear error instead of silently truncating.
	otbmPath := filepath.Join(t.TempDir(), "extended.otbm")
	_, err = Save(otbmPath, m, db, Format{})
	if err == nil {
		t.Fatalf("Save .otbm with ID %d succeeded, want overflow error", highGround)
	}
	if !strings.Contains(err.Error(), "65535") {
		t.Fatalf("Save .otbm error = %q, want mention of 65535 limit", err.Error())
	}
}
