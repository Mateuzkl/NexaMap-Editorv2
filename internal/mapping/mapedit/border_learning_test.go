package mapedit

import (
	"strings"
	"testing"

	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/mapping/materials"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

func TestLearnBorderFromSelection(t *testing.T) {
	items := otitems.NewDB(otitems.ModeOTB, []*otitems.Type{
		{ID: 100, ClientID: 100, Name: "grass", Flags: otitems.FlagGround},
		{ID: 101, ClientID: 101, Name: "dirt", Flags: otitems.FlagGround},
		{ID: 201, ClientID: 201, Name: "border n", TopOrder: borderTopOrder},
		{ID: 202, ClientID: 202, Name: "border e", TopOrder: borderTopOrder},
		{ID: 203, ClientID: 203, Name: "border s", TopOrder: borderTopOrder},
		{ID: 204, ClientID: 204, Name: "border w", TopOrder: borderTopOrder},
		{ID: 205, ClientID: 205, Name: "border cnw", TopOrder: borderTopOrder},
		{ID: 206, ClientID: 206, Name: "border cne", TopOrder: borderTopOrder},
		{ID: 207, ClientID: 207, Name: "border csw", TopOrder: borderTopOrder},
		{ID: 208, ClientID: 208, Name: "border cse", TopOrder: borderTopOrder},
	})
	m := otmap.New(64, 64)
	// 5x5 dirt patch with a 3x3 grass island in the middle (x=11..13, y=11..13)
	// and outer border items placed on the surrounding dirt ring (x=10..14, y=10..14).
	for y := 10; y <= 14; y++ {
		for x := 10; x <= 14; x++ {
			pos := otmap.Position{X: uint16(x), Y: uint16(y), Z: 7}
			isGrass := x >= 11 && x <= 13 && y >= 11 && y <= 13
			tile := &otmap.Tile{}
			if isGrass {
				tile.Ground = otmap.Item{ID: 100, Selected: true}
			} else {
				tile.Ground = otmap.Item{ID: 101, Selected: true}
				switch {
				case x == 10 && y == 10:
					tile.Items = []otmap.Item{{ID: 208, Selected: true}} // grass is SE -> cse
				case x == 14 && y == 10:
					tile.Items = []otmap.Item{{ID: 207, Selected: true}} // grass is SW -> csw
				case x == 10 && y == 14:
					tile.Items = []otmap.Item{{ID: 206, Selected: true}} // grass is NE -> cne
				case x == 14 && y == 14:
					tile.Items = []otmap.Item{{ID: 205, Selected: true}} // grass is NW -> cnw
				case y == 10:
					tile.Items = []otmap.Item{{ID: 203, Selected: true}} // grass is S -> s
				case y == 14:
					tile.Items = []otmap.Item{{ID: 201, Selected: true}} // grass is N -> n
				case x == 10:
					tile.Items = []otmap.Item{{ID: 202, Selected: true}} // grass is E -> e
				case x == 14:
					tile.Items = []otmap.Item{{ID: 204, Selected: true}} // grass is W -> w
				}
			}
			tile.SetSelected(true)
			m.SetTile(pos, tile)
		}
	}

	ed := New(m, items, NewBrushes(items, materials.New()))
	ed.RebuildSelection()
	report, err := ed.LearnBorderFromSelection()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Transitions) == 0 {
		t.Fatal("expected at least one learned transition")
	}
	tr := report.Transitions[0]
	if tr.AssignedCount != 8 {
		t.Fatalf("assigned = %d, want 8 (%+v)", tr.AssignedCount, tr.Slots)
	}
	want := map[string]uint16{"n": 201, "e": 202, "s": 203, "w": 204, "cnw": 205, "cne": 206, "csw": 207, "cse": 208}
	for _, slot := range tr.Slots {
		if expected, ok := want[slot.Edge]; ok && slot.ItemID != expected {
			t.Fatalf("edge %s = %d, want %d", slot.Edge, slot.ItemID, expected)
		}
	}

	xmlOut := FormatLearnedBorderXML(materials.Border{ID: 900, Edges: want}, "custom grass", []uint16{100}, "outer", "all", 3500)
	if !strings.Contains(xmlOut, `<border id="900">`) || !strings.Contains(xmlOut, `<borderitem edge="n" item="201"/>`) || !strings.Contains(xmlOut, `<brush name="custom grass"`) {
		t.Fatalf("unexpected xml:\n%s", xmlOut)
	}
}

func TestRemoveDuplicatesAndFindEmptySpawns(t *testing.T) {
	items := otitems.NewDB(otitems.ModeOTB, []*otitems.Type{
		{ID: 100, ClientID: 100, Name: "grass", Flags: otitems.FlagGround},
		{ID: 300, ClientID: 300, Name: "stone"},
	})
	m := otmap.New(64, 64)
	tile := &otmap.Tile{
		Ground: otmap.Item{ID: 100},
		Items: []otmap.Item{
			{ID: 300},
			{ID: 300},
			{ID: 300, Ext: &otmap.ItemExt{ActionID: 1234}}, // complex item must be preserved!
		},
	}
	tile.EnsureExt().Spawn = &otmap.Spawn{Radius: 3}
	m.SetTile(otmap.Position{X: 20, Y: 20, Z: 7}, tile)

	ed := New(m, items, NewBrushes(items, materials.New()))
	emptySpawns, _ := ed.Find(FindQuery{Kind: "emptySpawn"})
	if len(emptySpawns) != 1 || emptySpawns[0].Pos.X != 20 {
		t.Fatalf("emptySpawns = %+v", emptySpawns)
	}
	removed := ed.RemoveDuplicates(false)
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	after := m.Tile(otmap.Position{X: 20, Y: 20, Z: 7})
	if len(after.Items) != 2 || after.Items[1].ActionID() != 1234 {
		t.Fatalf("items after dedup = %+v", after.Items)
	}

	cands := ed.CollectQuickReplaceCandidates(false)
	if len(cands) != 2 || cands[0].Category != "Floor" || cands[0].ID != 100 || cands[1].ID != 300 || cands[1].Count != 2 {
		t.Fatalf("quick replace candidates = %+v", cands)
	}
}
