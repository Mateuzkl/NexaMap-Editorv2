package app

import (
	"fmt"
	"math"
	"strings"

	"github.com/nekiro/ots-creator/internal/mapping/materials"
)

type PaletteBrush struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// Look is the server id of the item shown.
	Look uint16 `json:"look"`
	// Builtin is true for bundled NexaMap brushes that have not been customized by the user.
	Builtin bool `json:"builtin,omitempty"`
	// Preview shows a brush that places a group of tiles (a stamp made from
	// a selection) as the whole group; nil for one-tile brushes.
	Preview *BrushPreview `json:"preview,omitempty"`
}

// BrushPreview is the ground floor of the first group of a doodad brush,
// at most previewMax tiles a side: W x H tiles, items bottom to top.
type BrushPreview struct {
	W     int           `json:"w"`
	H     int           `json:"h"`
	Tiles []PreviewTile `json:"tiles"`
}

type PreviewTile struct {
	X     int      `json:"x"`
	Y     int      `json:"y"`
	Items []uint16 `json:"items"`
}

const previewMax = 12

func brushPreview(brush *materials.Brush) *BrushPreview {
	if len(brush.Alternatives) == 0 || len(brush.Alternatives[0].Composites) == 0 {
		return nil
	}
	tiles := brush.Alternatives[0].Composites[0].Tiles
	minX, minY, maxX, maxY := math.MaxInt, math.MaxInt, math.MinInt, math.MinInt
	for _, tile := range tiles {
		if tile.Z == 0 {
			minX, minY, maxX, maxY = min(minX, tile.X), min(minY, tile.Y), max(maxX, tile.X), max(maxY, tile.Y)
		}
	}
	if minX > maxX || (minX == maxX && minY == maxY) {
		return nil
	}
	preview := &BrushPreview{W: min(maxX-minX+1, previewMax), H: min(maxY-minY+1, previewMax)}
	for _, tile := range tiles {
		x, y := tile.X-minX, tile.Y-minY
		if tile.Z != 0 || x >= preview.W || y >= preview.H {
			continue
		}
		previewTile := PreviewTile{X: x, Y: y}
		for _, item := range tile.Items {
			previewTile.Items = append(previewTile.Items, item.ID)
		}
		preview.Tiles = append(preview.Tiles, previewTile)
	}
	return preview
}

type PaletteTileset struct {
	Name    string         `json:"name"`
	Auto    bool           `json:"auto"`
	Terrain []PaletteBrush `json:"terrain"`
	Doodads []PaletteBrush `json:"doodads"`
	Items   []uint16       `json:"items"`
	Raw     []uint16       `json:"raw"`
}

type Palette struct {
	Tilesets  []PaletteTileset          `json:"tilesets"`
	Brushes   []PaletteBrush            `json:"brushes"`
	Creatures []*materials.CreatureType `json:"creatures"`
	DoorTypes []string                  `json:"doorTypes"`
}

func (ms *MapService) Palette() (palette Palette, err error) {
	err = ms.withData(func(data *mapData) error {
		palette = data.palette()
		return nil
	})
	return palette, err
}

func (data *mapData) palette() Palette {
	eff := data.effective
	if eff == nil {
		eff = data.materials
	}
	userBrushes := make(map[string]bool, len(data.materials.Brushes))
	for _, b := range data.materials.Brushes {
		userBrushes[strings.ToLower(b.Name)] = true
	}

	palette := Palette{DoorTypes: materials.DoorTypes, Creatures: data.allCreatures()}
	brushByName := make(map[string]PaletteBrush, len(eff.Brushes))
	for _, brush := range eff.Brushes {
		isBuiltin := !userBrushes[strings.ToLower(brush.Name)]
		if paletteBrush, ok := data.paletteBrush(brush, isBuiltin); ok {
			palette.Brushes = append(palette.Brushes, paletteBrush)
			brushByName[strings.ToLower(brush.Name)] = paletteBrush
		}
	}

	userTilesets := make(map[string]bool, len(data.materials.Tilesets))
	for _, ts := range data.materials.Tilesets {
		userTilesets[strings.ToLower(ts.Name)] = true
	}

	usedNames := map[string]bool{}
	for _, tileset := range eff.Tilesets {
		isUser := userTilesets[strings.ToLower(tileset.Name)]
		paletteTileset := PaletteTileset{
			Name:    tileset.Name,
			Auto:    tileset.Auto,
			Terrain: lookupPaletteBrushes(brushByName, tileset.Terrain),
			Doodads: lookupPaletteBrushes(brushByName, tileset.Doodads),
		}
		if isUser {
			paletteTileset.Items = tileset.Items
			paletteTileset.Raw = tileset.Raw
		} else {
			paletteTileset.Items = data.filterItems(tileset.Items)
			paletteTileset.Raw = data.filterItems(tileset.Raw)
			if len(paletteTileset.Terrain) == 0 && len(paletteTileset.Doodads) == 0 && len(paletteTileset.Items) == 0 && len(paletteTileset.Raw) == 0 {
				continue
			}
		}
		palette.Tilesets = append(palette.Tilesets, paletteTileset)
		usedNames[strings.ToLower(paletteTileset.Name)] = true
	}

	for _, tileset := range materials.AutoTilesets(data.items) {
		name := uniqueTilesetName(tileset.Name, usedNames)
		usedNames[strings.ToLower(name)] = true
		palette.Tilesets = append(palette.Tilesets, PaletteTileset{
			Name:  name,
			Auto:  true,
			Items: tileset.Items,
			Raw:   tileset.Raw,
		})
	}
	return palette
}

func uniqueTilesetName(base string, used map[string]bool) string {
	if !used[strings.ToLower(base)] {
		return base
	}
	candidate := base + " (Flags)"
	if !used[strings.ToLower(candidate)] {
		return candidate
	}
	for n := 2; ; n++ {
		candidate = fmt.Sprintf("%s (Flags %d)", base, n)
		if !used[strings.ToLower(candidate)] {
			return candidate
		}
	}
}

func (data *mapData) filterItems(ids []uint16) []uint16 {
	if len(ids) == 0 {
		return nil
	}
	out := make([]uint16, 0, len(ids))
	for _, id := range ids {
		if data.hasItem(id) {
			out = append(out, id)
		}
	}
	return out
}

func lookupPaletteBrushes(byName map[string]PaletteBrush, names []string) []PaletteBrush {
	if len(names) == 0 {
		return nil
	}
	out := make([]PaletteBrush, 0, len(names))
	for _, name := range names {
		if pb, ok := byName[strings.ToLower(name)]; ok {
			out = append(out, pb)
		}
	}
	return out
}

func (data *mapData) paletteBrush(brush *materials.Brush, builtin bool) (PaletteBrush, bool) {
	if brush == nil {
		return PaletteBrush{}, false
	}
	kind := brush.Type
	// A wall brush without walls only places its alternatives.
	if brush.Type == materials.TypeWall && len(brush.Walls) == 0 && len(brush.Alternatives) > 0 {
		kind = materials.TypeDoodad
	}
	look := data.brushes.Look(brush.Name)
	if look == 0 {
		look = brush.Look
	}
	if resolved := data.resolveBrushLook(brush, look); resolved != 0 {
		look = resolved
	}
	return PaletteBrush{Name: brush.Name, Kind: kind, Look: look, Builtin: builtin, Preview: brushPreview(brush)}, true
}
