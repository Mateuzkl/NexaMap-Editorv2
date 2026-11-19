package huntforge

import (
	"testing"

	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

func buildTestHuntMap() (*otmap.Map, *otitems.DB) {
	db := otitems.NewDB(otitems.ModeClient, []*otitems.Type{
		{ID: 100, ClientID: 100, Group: otitems.GroupGround, Flags: otitems.FlagGround, Name: "stone floor"},
		{ID: 101, ClientID: 101, Group: otitems.GroupGround, Flags: otitems.FlagGround | otitems.FlagUnpassable, Name: "rock wall"},
		{ID: 200, ClientID: 200, TopOrder: 2, Flags: otitems.FlagUnpassable, Name: "wall"},
	})
	m := otmap.New(2048, 2048)

	// Create a classic 2-room + 4-way cross corridor dungeon with walls around it (50x50 at 100..150, z=7).
	walk := map[[2]int]bool{}
	carveRect := func(x1, y1, x2, y2 int) {
		for y := y1; y <= y2; y++ {
			for x := x1; x <= x2; x++ {
				walk[[2]int{x, y}] = true
			}
		}
	}
	// Central room + 4 corridors + 4 outer rooms
	carveRect(120, 120, 130, 130)
	carveRect(108, 123, 142, 127)
	carveRect(123, 108, 127, 142)
	carveRect(102, 121, 109, 129)
	carveRect(141, 121, 148, 129)
	carveRect(121, 102, 129, 109)
	carveRect(121, 141, 129, 148)

	for y := 98; y <= 152; y++ {
		for x := 98; x <= 152; x++ {
			pos := otmap.Position{X: uint16(x), Y: uint16(y), Z: 7}
			if walk[[2]int{x, y}] {
				m.SetTile(pos, &otmap.Tile{Ground: otmap.Item{ID: 100}})
			} else {
				m.SetTile(pos, &otmap.Tile{Ground: otmap.Item{ID: 101}, Items: []otmap.Item{{ID: 200}}})
			}
		}
	}

	// Place 16 monsters in the rooms and corridors (within linkDistance=12).
	spots := [][2]int{
		{122, 122}, {125, 125}, {128, 128}, {122, 128},
		{114, 125}, {135, 125}, {125, 114}, {125, 135},
		{105, 125}, {107, 124}, {144, 125}, {146, 124},
		{125, 105}, {124, 107}, {125, 144}, {126, 146},
	}
	for i, sp := range spots {
		pos := otmap.Position{X: uint16(sp[0]), Y: uint16(sp[1]), Z: 7}
		tile := m.Tile(pos).Clone()
		ext := tile.EnsureExt()
		name := "Dragon Lord"
		if i%3 == 0 {
			name = "Dragon"
		}
		ext.Creatures = append(ext.Creatures, otmap.Creature{Name: name, SpawnTime: 60})
		ext.Spawn = &otmap.Spawn{Radius: 2}
		m.SetTile(pos, tile)
	}
	return m, db
}

func TestDetectAnalyzeAndGenerateAllModes(t *testing.T) {
	m, db := buildTestHuntMap()

	detected := Detect(m, 12, 8, 6)
	if len(detected) == 0 {
		t.Fatal("expected at least 1 detected hunt")
	}
	if detected[0].Creatures != 16 {
		t.Fatalf("detected creatures = %d, want 16", detected[0].Creatures)
	}

	tpl := Extract(m, detected[0].Region, detected[0].Name)
	if len(tpl.Tiles) == 0 || len(tpl.Creatures) != 16 {
		t.Fatalf("extracted tiles=%d creatures=%d", len(tpl.Tiles), len(tpl.Creatures))
	}

	analysis := Analyze(tpl, db, -1)
	if analysis.WalkArea < 100 || analysis.CorridorWidth < 2 || len(analysis.Runs) == 0 || len(analysis.Rooms) == 0 {
		t.Fatalf("unexpected analysis: %s", analysis.Describe())
	}

	for _, tc := range []struct {
		name string
		opts Options
	}{
		{"clone", Options{Mode: ModeClone, Seed: 42, SpawnDensity: 1.0}},
		{"expand", Options{Mode: ModeExpand, AreaFactor: 1.5, Seed: 42, SpawnDensity: 1.0, LoopChance: -1, Balanced: true}},
		{"random", Options{Mode: ModeRandom, AreaFactor: 1.2, Seed: 42, SpawnDensity: 1.0, LoopChance: 0.4, Balanced: true}},
		{"mix_cross", Options{Mode: ModeMix, Style: StyleCross, AreaFactor: 1.5, Seed: 42, SpawnDensity: 1.0, Balanced: true}},
		{"mix_star", Options{Mode: ModeMix, Style: StyleStar, AreaFactor: 1.5, Seed: 7, SpawnDensity: 1.2, Balanced: true}},
	} {
		res := Generate(analysis, tc.opts)
		if res == nil || res.Hunt == nil || len(res.Hunt.Tiles) == 0 {
			t.Fatalf("%s: generated empty hunt", tc.name)
		}
		if res.WalkArea == 0 {
			t.Fatalf("%s: walkArea = 0", tc.name)
		}
		outMap := res.Hunt.ToMap(otmap.Position{X: 500, Y: 500, Z: 7}, m)
		if outMap.TileCount() == 0 {
			t.Fatalf("%s: ToMap produced 0 tiles", tc.name)
		}
	}
}
