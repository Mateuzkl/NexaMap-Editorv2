package huntforge

import (
	"fmt"
	"sort"

	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

// Region is an inclusive 3D bounding box on a map.
type Region struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
	Z1 int `json:"z1"`
	Z2 int `json:"z2"`
}

func (r Region) Normalized() Region {
	if r.X1 > r.X2 {
		r.X1, r.X2 = r.X2, r.X1
	}
	if r.Y1 > r.Y2 {
		r.Y1, r.Y2 = r.Y2, r.Y1
	}
	if r.Z1 > r.Z2 {
		r.Z1, r.Z2 = r.Z2, r.Z1
	}
	if r.X1 < 0 {
		r.X1 = 0
	}
	if r.Y1 < 0 {
		r.Y1 = 0
	}
	if r.Z1 < 0 {
		r.Z1 = 0
	}
	if r.Z2 > otmap.MaxFloor {
		r.Z2 = otmap.MaxFloor
	}
	return r
}

func (r Region) Width() int  { return max(0, r.X2-r.X1+1) }
func (r Region) Height() int { return max(0, r.Y2-r.Y1+1) }
func (r Region) Floors() int { return max(0, r.Z2-r.Z1+1) }

func (r Region) Contains(pos otmap.Position) bool {
	x, y, z := int(pos.X), int(pos.Y), int(pos.Z)
	return x >= r.X1 && x <= r.X2 && y >= r.Y1 && y <= r.Y2 && z >= r.Z1 && z <= r.Z2
}

// SpeciesCount counts occurrences of a monster species in a hunt.
type SpeciesCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Suggestion is an automatically detected hunt cluster from map spawns.
type Suggestion struct {
	Name      string         `json:"name"`
	Region    Region         `json:"region"`
	Creatures int            `json:"creatures"`
	Species   []SpeciesCount `json:"species"`
}

// PositionedTile holds a tile at coordinates relative to a Template's origin.
type PositionedTile struct {
	X, Y, Z int
	Tile    *otmap.Tile
}

// PlacedCreature holds a monster/NPC at coordinates relative to a Template's origin.
type PlacedCreature struct {
	X, Y, Z  int
	Creature otmap.Creature
}

// Template is a hunt copied from a map with relative coordinates (0,0,0 at top-left).
type Template struct {
	Name      string
	Source    Region
	Tiles     []PositionedTile
	Creatures []PlacedCreature
}

func (t *Template) Width() int  { return t.Source.Width() }
func (t *Template) Height() int { return t.Source.Height() }
func (t *Template) Floors() int { return t.Source.Floors() }

// Species returns non-NPC monster species sorted by descending count then name.
func (t *Template) Species() []SpeciesCount {
	counts := map[string]int{}
	for _, c := range t.Creatures {
		if !c.Creature.NPC && c.Creature.Name != "" {
			counts[c.Creature.Name]++
		}
	}
	return sortedSpecies(counts)
}

func sortedSpecies(counts map[string]int) []SpeciesCount {
	out := make([]SpeciesCount, 0, len(counts))
	for name, n := range counts {
		out = append(out, SpeciesCount{Name: name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Extract copies tiles and creatures from gameMap inside region into a relative Template.
// House IDs are cleared so hunts never drag town house ownership with them.
func Extract(gameMap *otmap.Map, region Region, name string) *Template {
	region = region.Normalized()
	tpl := &Template{
		Name:   name,
		Source: region,
	}
	if gameMap == nil {
		return tpl
	}
	gameMap.Each(func(pos otmap.Position, tile *otmap.Tile) {
		if !region.Contains(pos) || tile.Empty() {
			return
		}
		rx := int(pos.X) - region.X1
		ry := int(pos.Y) - region.Y1
		rz := int(pos.Z) - region.Z1
		cloned := tile.Clone()
		cloned.HouseID = 0
		if cloned.Ext != nil {
			for _, cr := range cloned.Ext.Creatures {
				tpl.Creatures = append(tpl.Creatures, PlacedCreature{X: rx, Y: ry, Z: rz, Creature: cr})
			}
			// Keep structural tile clean of spawn/creature metadata in template tiles;
			// creatures are tracked explicitly in tpl.Creatures and re-attached in ToMap.
			cloned.Ext.Creatures = nil
			cloned.Ext.Spawn = nil
			cloned.Ext.SpawnNPC = nil
			cloned.Compact()
		}
		if cloned.Ground.ID != 0 || len(cloned.Items) > 0 || cloned.Flags != 0 {
			tpl.Tiles = append(tpl.Tiles, PositionedTile{X: rx, Y: ry, Z: rz, Tile: cloned})
		}
	})
	return tpl
}

// ToMap materializes the template into a standalone *otmap.Map anchored at origin.
func (t *Template) ToMap(origin otmap.Position, ref *otmap.Map) *otmap.Map {
	width, height := uint16(2048), uint16(2048)
	if ref != nil {
		width, height = ref.Width, ref.Height
	}
	if needW := int(origin.X) + t.Width() + 32; needW > int(width) && needW <= 0xFFFF {
		width = uint16(needW)
	}
	if needH := int(origin.Y) + t.Height() + 32; needH > int(height) && needH <= 0xFFFF {
		height = uint16(needH)
	}
	m := otmap.New(width, height)
	if ref != nil {
		m.ItemsMajor = ref.ItemsMajor
		m.ItemsMinor = ref.ItemsMinor
		m.SpawnFormat = ref.SpawnFormat
	}
	m.Description = fmt.Sprintf("Hunt Forge: %s (%dx%d)", t.Name, t.Width(), t.Height())
	for _, pt := range t.Tiles {
		pos, ok := origin.Add(pt.X, pt.Y, pt.Z)
		if !ok {
			continue
		}
		m.SetTile(pos, pt.Tile.Clone())
	}
	for _, pc := range t.Creatures {
		pos, ok := origin.Add(pc.X, pc.Y, pc.Z)
		if !ok {
			continue
		}
		tile := m.Tile(pos)
		if tile == nil {
			tile = &otmap.Tile{}
		} else {
			tile = tile.Clone()
		}
		ext := tile.EnsureExt()
		cr := pc.Creature
		cr.Selected = false
		ext.Creatures = append(ext.Creatures, cr)
		if cr.NPC {
			if ext.SpawnNPC == nil {
				ext.SpawnNPC = &otmap.Spawn{Radius: 1}
			}
		} else {
			if ext.Spawn == nil {
				ext.Spawn = &otmap.Spawn{Radius: 1}
			}
		}
		m.SetTile(pos, tile)
	}
	return m
}

// Mode controls how HuntGenerator creates the output hunt.
type Mode string

const (
	ModeClone  Mode = "clone"
	ModeExpand Mode = "expand"
	ModeRandom Mode = "random"
	ModeMix    Mode = "mix"
)

// ShapeStyle controls planned arm layouts in ModeMix.
type ShapeStyle string

const (
	StyleAuto       ShapeStyle = "Auto"
	StyleStar       ShapeStyle = "Star"
	StyleCross      ShapeStyle = "Cross"
	StyleDiagonal   ShapeStyle = "Diagonal"
	StyleOctagon    ShapeStyle = "Octagon"
	StyleArena      ShapeStyle = "Arena"
	StyleBranches   ShapeStyle = "Branches"
	StyleAsymmetric ShapeStyle = "Asymmetric"
	StyleOrganic    ShapeStyle = "Organic"
)

func StyleLabel(s ShapeStyle) string {
	switch s {
	case StyleStar:
		return "Estrela 8 pontas"
	case StyleCross:
		return "Cruz expandida"
	case StyleDiagonal:
		return "X diagonal"
	case StyleOctagon:
		return "Octógono"
	case StyleArena:
		return "Arena com braços"
	case StyleBranches:
		return "Ramificado"
	case StyleAsymmetric:
		return "Assimétrico"
	case StyleOrganic:
		return "Orgânico"
	default:
		return "Automático"
	}
}

// Options configures a HuntGenerator run.
type Options struct {
	Mode         Mode       `json:"mode"`
	AreaFactor   float64    `json:"areaFactor"`
	Seed         int64      `json:"seed"`
	SpawnDensity float64    `json:"spawnDensity"`
	LoopChance   float64    `json:"loopChance"`
	Name         string     `json:"name"`
	Balanced     bool       `json:"balanced"`
	Style        ShapeStyle `json:"style"`
}

func (o Options) normalized() Options {
	if o.Mode == "" {
		o.Mode = ModeExpand
	}
	if o.AreaFactor <= 0 {
		o.AreaFactor = 1.75
	}
	if o.SpawnDensity < 0 {
		o.SpawnDensity = 1.0
	}
	if o.Style == "" {
		o.Style = StyleAuto
	}
	return o
}

// AnalysisSummary exposes the measured hunt metrics to the UI.
type AnalysisSummary struct {
	WalkArea                int            `json:"walkArea"`
	CorridorWidth           int            `json:"corridorWidth"`
	Rooms                   int            `json:"rooms"`
	Runs                    int            `json:"runs"`
	DiagonalShare           float64        `json:"diagonalShare"`
	LoopsPerRoom            float64        `json:"loopsPerRoom"`
	Separation              int            `json:"separation"`
	FillDepth               int            `json:"fillDepth"`
	ObstaclesAsFloor        int            `json:"obstaclesAsFloor"`
	Context5                int            `json:"context5"`
	Context3                int            `json:"context3"`
	Creatures               int            `json:"creatures"`
	CorridorCreatureDensity float64        `json:"corridorCreatureDensity"`
	RoomCreatureDensity     float64        `json:"roomCreatureDensity"`
	CreatureSpacing         int            `json:"creatureSpacing"`
	Floor                   int            `json:"floor"`
	Width                   int            `json:"width"`
	Height                  int            `json:"height"`
	Species                 []SpeciesCount `json:"species"`
	Description             string         `json:"description"`
}

// Result holds the generated hunt template and statistics.
type Result struct {
	Hunt           *Template      `json:"-"`
	WalkArea       int            `json:"walkArea"`
	RoomsAdded     int            `json:"roomsAdded"`
	CorridorsAdded int            `json:"corridorsAdded"`
	Creatures      int            `json:"creatures"`
	OriginalOffset [3]int         `json:"originalOffset"`
	ContextMisses  int            `json:"contextMisses"`
	Style          string         `json:"style"`
	Report         string         `json:"report"`
	Species        []SpeciesCount `json:"species"`
	Width          int            `json:"width"`
	Height         int            `json:"height"`
}
