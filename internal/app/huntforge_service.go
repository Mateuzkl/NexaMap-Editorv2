package app

import (
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"sync"

	"github.com/nekiro/ots-creator/internal/core/imaging"
	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/mapping/huntforge"
	"github.com/nekiro/ots-creator/internal/mapping/mapedit"
	"github.com/nekiro/ots-creator/internal/mapping/maprender"
	"github.com/nekiro/ots-creator/internal/mapping/materials"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

// HuntAnalysisView returns the analyzed source hunt metrics and a PNG preview data URL.
type HuntAnalysisView struct {
	Name    string                    `json:"name"`
	Region  huntforge.Region          `json:"region"`
	Summary huntforge.AnalysisSummary `json:"summary"`
	Preview string                    `json:"preview"`
	TilePx  int                       `json:"tilePx"`
}

// HuntGenerateView returns the generated hunt report, stats, and a PNG preview data URL.
type HuntGenerateView struct {
	Result  huntforge.Result `json:"result"`
	Preview string           `json:"preview"`
	TilePx  int              `json:"tilePx"`
}

// HuntForgeService exposes procedural hunt detection, analysis, generation,
// direct undoable map placement, and standalone OTBM export to the frontend.
type HuntForgeService struct {
	maps *MapService

	mu       sync.Mutex
	mapID    int
	source   *huntforge.Template
	analysis *huntforge.Analysis
	result   *huntforge.Template
}

func NewHuntForgeService(maps *MapService) *HuntForgeService {
	svc := &HuntForgeService{maps: maps}
	if maps != nil {
		maps.mu.Lock()
		maps.closed = append(maps.closed, func(closedID int) {
			svc.mu.Lock()
			if svc.mapID == closedID {
				svc.mapID = 0
				svc.source = nil
				svc.analysis = nil
				svc.result = nil
			}
			svc.mu.Unlock()
		})
		maps.mu.Unlock()
	}
	return svc
}

// Clear releases cached templates/analyses when the HuntForge dialog closes.
func (hf *HuntForgeService) Clear() {
	hf.mu.Lock()
	hf.mapID = 0
	hf.source = nil
	hf.analysis = nil
	hf.result = nil
	hf.mu.Unlock()
}

// Detect finds monster clusters on the open map and returns hunt suggestions.
func (hf *HuntForgeService) Detect(mapID int) ([]huntforge.Suggestion, error) {
	var out []huntforge.Suggestion
	err := hf.maps.withMapRead(mapID, func(opened *openMap, _ *mapData) error {
		out = huntforge.Detect(opened.editor.Map, 12, 8, 10)
		return nil
	})
	if out == nil {
		out = []huntforge.Suggestion{}
	}
	return out, err
}

// SelectionRegion returns the inclusive bounding box of the current map selection.
func (hf *HuntForgeService) SelectionRegion(mapID int) (huntforge.Region, error) {
	var reg huntforge.Region
	err := hf.maps.withMapRead(mapID, func(opened *openMap, _ *mapData) error {
		minX, minY, minZ, maxX, maxY, maxZ := 0xFFFF, 0xFFFF, otmap.MaxFloor, 0, 0, 0
		found := false
		opened.editor.Map.Each(func(pos otmap.Position, tile *otmap.Tile) {
			if !tileHasSelected(tile) {
				return
			}
			found = true
			x, y, z := int(pos.X), int(pos.Y), int(pos.Z)
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if z < minZ {
				minZ = z
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
			if z > maxZ {
				maxZ = z
			}
		})
		if !found {
			return errors.New("no tiles selected on the map")
		}
		reg = huntforge.Region{X1: minX, Y1: minY, X2: maxX, Y2: maxY, Z1: minZ, Z2: maxZ}
		return nil
	})
	return reg, err
}

func tileHasSelected(tile *otmap.Tile) bool {
	if tile == nil {
		return false
	}
	if tile.Ground.Selected {
		return true
	}
	for _, it := range tile.Items {
		if it.Selected {
			return true
		}
	}
	for _, cr := range tile.Creatures() {
		if cr.Selected {
			return true
		}
	}
	if sp := tile.Spawn(); sp != nil && sp.Selected {
		return true
	}
	if sp := tile.SpawnNPC(); sp != nil && sp.Selected {
		return true
	}
	return false
}

// Analyze extracts region from mapID, learns its topology/style, and returns metrics + PNG preview.
func (hf *HuntForgeService) Analyze(mapID int, region huntforge.Region, name string, showMonsters bool) (HuntAnalysisView, error) {
	region = region.Normalized()
	if region.Width() <= 0 || region.Height() <= 0 {
		return HuntAnalysisView{}, errors.New("empty hunt region")
	}
	if region.Width() > 512 || region.Height() > 512 {
		return HuntAnalysisView{}, errors.New("region too large (maximum 512x512 tiles)")
	}
	var view HuntAnalysisView
	err := hf.maps.withMapRead(mapID, func(opened *openMap, data *mapData) error {
		if name == "" {
			name = fmt.Sprintf("hunt_%d_%d_z%d", region.X1, region.Y1, region.Z1)
		}
		tpl := huntforge.Extract(opened.editor.Map, region, name)
		if len(tpl.Tiles) == 0 {
			return errors.New("no tiles found in the selected region")
		}
		analysis := huntforge.Analyze(tpl, opened.editor.Items, -1)
		preview, tilePx, err := renderTemplatePreview(tpl, analysis.Floor, opened, data, showMonsters)
		if err != nil {
			return err
		}

		hf.mu.Lock()
		hf.mapID = mapID
		hf.source = tpl
		hf.analysis = analysis
		hf.result = nil
		hf.mu.Unlock()

		view = HuntAnalysisView{
			Name:    name,
			Region:  region,
			Summary: analysis.Summary(),
			Preview: preview,
			TilePx:  tilePx,
		}
		return nil
	})
	return view, err
}

// Generate runs HuntForge synthesis on the last analyzed hunt (or re-analyzes region if needed).
func (hf *HuntForgeService) Generate(mapID int, region huntforge.Region, name string, opts huntforge.Options, showMonsters bool) (HuntGenerateView, error) {
	hf.mu.Lock()
	analysis := hf.analysis
	sameMap := hf.mapID == mapID
	hf.mu.Unlock()

	if analysis == nil || !sameMap {
		if _, err := hf.Analyze(mapID, region, name, showMonsters); err != nil {
			return HuntGenerateView{}, err
		}
		hf.mu.Lock()
		analysis = hf.analysis
		hf.mu.Unlock()
	}

	res := huntforge.Generate(analysis, opts)
	if res == nil || res.Hunt == nil {
		return HuntGenerateView{}, errors.New("generation produced an empty hunt")
	}

	var preview string
	var tilePx int
	err := hf.maps.withMapRead(mapID, func(opened *openMap, data *mapData) error {
		var err error
		preview, tilePx, err = renderTemplatePreview(res.Hunt, 0, opened, data, showMonsters)
		return err
	})
	if err != nil {
		return HuntGenerateView{}, err
	}

	hf.mu.Lock()
	hf.result = res.Hunt
	hf.mu.Unlock()

	return HuntGenerateView{
		Result:  *res,
		Preview: preview,
		TilePx:  tilePx,
	}, nil
}

// RenderPreview re-renders either "source" or "result" preview with/without monsters.
func (hf *HuntForgeService) RenderPreview(mapID int, which string, showMonsters bool) (string, error) {
	hf.mu.Lock()
	var tpl *huntforge.Template
	floor := 0
	if which == "result" {
		tpl = hf.result
	} else {
		tpl = hf.source
		if hf.analysis != nil {
			floor = hf.analysis.Floor
		}
	}
	hf.mu.Unlock()
	if tpl == nil {
		return "", nil
	}
	var preview string
	err := hf.maps.withMapRead(mapID, func(opened *openMap, data *mapData) error {
		var err error
		preview, _, err = renderTemplatePreview(tpl, floor, opened, data, showMonsters)
		return err
	})
	return preview, err
}

// PlaceInMap inserts the generated hunt directly into the open map at (x, y, z)
// as a single undoable transaction (Ctrl+Z undoes the whole placement).
func (hf *HuntForgeService) PlaceInMap(mapID, x, y, z int, replace bool) (MapChange, error) {
	hf.mu.Lock()
	res := hf.result
	hf.mu.Unlock()
	if res == nil {
		return MapChange{}, errors.New("generate a hunt first")
	}
	if x < 0 || y < 0 || x > 0xFFFF || y > 0xFFFF || z < 0 || z > otmap.MaxFloor {
		return MapChange{}, errors.New("destination coordinates out of map bounds")
	}
	return hf.maps.edit(mapID, func(editor *mapedit.Editor, _ *mapData) (int, error) {
		generatedMap := res.ToMap(otmap.Position{X: 0, Y: 0, Z: 0}, editor.Map)
		return editor.Import(generatedMap, mapedit.ImportOptions{
			DX:      x,
			DY:      y,
			DZ:      z,
			Houses:  "skip",
			Replace: replace,
			Towns:   false,
		})
	})
}

// ExportOTBM writes the generated hunt as a standalone .otbm + -spawn.xml pair.
func (hf *HuntForgeService) ExportOTBM(mapID int, path string, x, y, z int) (string, error) {
	hf.mu.Lock()
	res := hf.result
	hf.mu.Unlock()
	if res == nil {
		return "", errors.New("generate a hunt first")
	}
	if path == "" {
		return "", errors.New("no output file selected")
	}
	if x < 0 || y < 0 || x > 0xFFFF || y > 0xFFFF || z < 0 || z > otmap.MaxFloor {
		return "", errors.New("destination coordinates out of map bounds")
	}
	var savedPath string
	err := hf.maps.withMapRead(mapID, func(opened *openMap, data *mapData) error {
		outMap := res.ToMap(otmap.Position{X: uint16(x), Y: uint16(y), Z: uint8(z)}, opened.editor.Map)
		outMap.ForgetAuxFileNames()
		if _, err := otmap.Save(path, outMap, data.items, opened.editor.Settings.Format()); err != nil {
			return err
		}
		savedPath = filepath.Clean(path)
		return nil
	})
	return savedPath, err
}

func renderTemplatePreview(tpl *huntforge.Template, floor int, opened *openMap, data *mapData, showMonsters bool) (string, int, error) {
	w, h := tpl.Width(), tpl.Height()
	if w <= 0 || h <= 0 {
		return "", 1, nil
	}

	// For compact hunts (<= 96x96 tiles) with loaded client sprites, render real game sprites!
	if w*h <= 96*96 && data != nil && data.project != nil {
		tempMap := tpl.ToMap(otmap.Position{X: 0, Y: 0, Z: 0}, opened.editor.Map)
		scene := &maprender.Scene{
			Map:          tempMap,
			Items:        opened.editor.Items,
			Things:       data.project,
			SpriteSize:   data.project.SpriteSize(),
			SpritePixels: data.project.SpritePixels,
			IsBlocking:   opened.editor.IsBlocking,
			Outfit: func(creature string) materials.Outfit {
				outfit, _ := data.outfit(creature)
				return outfit
			},
			MarkedItems: markedItemColors(opened.editor),
		}
		opts := maprender.GeometryOptions{}
		if showMonsters {
			opts.ViewFlags = maprender.ViewCreatures
		}
		if img, err := maprender.RenderImage(scene, image.Rect(0, 0, w, h), floor, opts); err == nil {
			tilePx := scene.SpriteSize
			// Downscale very large sprite renders so PNG transfer stays snappy (< 1280px).
			if img.Bounds().Dx() > 1280 || img.Bounds().Dy() > 1280 {
				img, tilePx = downsampleNRGBA(img, tilePx, 1280)
			}
			pngBytes, err := imaging.EncodePNG(img)
			if err != nil {
				return "", 0, err
			}
			return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes), tilePx, nil
		}
	}

	// Fast high-contrast minimap/topology renderer for large hunts (works at any size).
	scale := 4
	if max(w, h) > 220 {
		scale = 3
	}
	if max(w, h) > 360 {
		scale = 2
	}
	img := renderMinimapTemplate(tpl, floor, opened.editor, opened.editor.Items, scale, showMonsters)
	pngBytes, err := imaging.EncodePNG(img)
	if err != nil {
		return "", 0, err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes), scale, nil
}

func renderMinimapTemplate(tpl *huntforge.Template, floor int, editor *mapedit.Editor, db *otitems.DB, scale int, showMonsters bool) *image.NRGBA {
	w, h := tpl.Width(), tpl.Height()
	img := image.NewNRGBA(image.Rect(0, 0, w*scale, h*scale))
	bg := color.NRGBA{11, 14, 20, 255}
	for y := 0; y < h*scale; y++ {
		for x := 0; x < w*scale; x++ {
			img.SetNRGBA(x, y, bg)
		}
	}
	for _, pt := range tpl.Tiles {
		if pt.Z != floor || pt.X < 0 || pt.X >= w || pt.Y < 0 || pt.Y >= h {
			continue
		}
		mc := editor.MinimapColor(pt.Tile)
		var col color.NRGBA
		if mc != 0 {
			r, g, b := mapedit.MinimapRGB(mc)
			col = color.NRGBA{r, g, b, 255}
		} else if huntforge.IsWalkable(pt.Tile, db) {
			col = color.NRGBA{110, 125, 145, 255}
		} else {
			col = color.NRGBA{52, 60, 74, 255}
		}
		for dy := 0; dy < scale; dy++ {
			for dx := 0; dx < scale; dx++ {
				img.SetNRGBA(pt.X*scale+dx, pt.Y*scale+dy, col)
			}
		}
	}
	if showMonsters {
		dot := color.NRGBA{255, 76, 106, 255}
		rim := color.NRGBA{18, 12, 20, 255}
		for _, pc := range tpl.Creatures {
			if pc.Z != floor || pc.X < 0 || pc.X >= w || pc.Y < 0 || pc.Y >= h {
				continue
			}
			cx, cy := pc.X*scale+scale/2, pc.Y*scale+scale/2
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					px, py := cx+dx, cy+dy
					if px < 0 || py < 0 || px >= w*scale || py >= h*scale {
						continue
					}
					d2 := dx*dx + dy*dy
					if d2 <= 1 {
						img.SetNRGBA(px, py, dot)
					} else if d2 <= 4 {
						img.SetNRGBA(px, py, rim)
					}
				}
			}
		}
	}
	return img
}

func downsampleNRGBA(src *image.NRGBA, tilePx, maxDim int) (*image.NRGBA, int) {
	bw, bh := src.Bounds().Dx(), src.Bounds().Dy()
	factor := 2
	for bw/factor > maxDim || bh/factor > maxDim {
		factor++
	}
	nw, nh := max(1, bw/factor), max(1, bh/factor)
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := y * factor
		for x := 0; x < nw; x++ {
			sx := x * factor
			dst.SetNRGBA(x, y, src.NRGBAAt(sx, sy))
		}
	}
	return dst, max(1, tilePx/factor)
}
