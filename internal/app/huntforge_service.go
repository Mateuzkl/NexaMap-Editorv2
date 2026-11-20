package app

import (
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"sync"

	"github.com/nekiro/ots-creator/internal/core/imaging"
	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/core/thing"
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

	mu           sync.Mutex
	mapID        int
	sourceRegion huntforge.Region
	analysisSeq  uint64
	source       *huntforge.Template
	analysis     *huntforge.Analysis
	result       *huntforge.Template
}

func NewHuntForgeService(maps *MapService) *HuntForgeService {
	svc := &HuntForgeService{maps: maps}
	if maps != nil {
		maps.mu.Lock()
		maps.closed = append(maps.closed, func(closedID int) {
			svc.mu.Lock()
			if svc.mapID == closedID {
				svc.analysisSeq++
				svc.mapID = 0
				svc.sourceRegion = huntforge.Region{}
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
	hf.analysisSeq++
	hf.mapID = 0
	hf.sourceRegion = huntforge.Region{}
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
	hf.mu.Lock()
	hf.analysisSeq++
	seq := hf.analysisSeq
	hf.mu.Unlock()

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
		if hf.analysisSeq == seq {
			hf.mapID = mapID
			hf.sourceRegion = region
			hf.source = tpl
			hf.analysis = analysis
			hf.result = nil
		}
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
	if err != nil {
		hf.mu.Lock()
		if hf.analysisSeq == seq {
			hf.mapID = 0
			hf.sourceRegion = huntforge.Region{}
			hf.source = nil
			hf.analysis = nil
			hf.result = nil
		}
		hf.mu.Unlock()
	}
	return view, err
}

// Generate runs HuntForge synthesis on the last analyzed hunt (or re-analyzes region if needed).
func (hf *HuntForgeService) Generate(mapID int, region huntforge.Region, name string, opts huntforge.Options, showMonsters bool) (HuntGenerateView, error) {
	region = region.Normalized()
	hf.mu.Lock()
	analysis := hf.analysis
	sameSource := hf.mapID == mapID && hf.sourceRegion == region
	hf.mu.Unlock()

	if analysis == nil || !sameSource {
		if _, err := hf.Analyze(mapID, region, name, showMonsters); err != nil {
			return HuntGenerateView{}, err
		}
		hf.mu.Lock()
		if hf.mapID == mapID && hf.sourceRegion == region {
			analysis = hf.analysis
		} else {
			analysis = nil
		}
		hf.mu.Unlock()
		if analysis == nil {
			return HuntGenerateView{}, errors.New("analysis was superseded by another request")
		}
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

	// Render real client sprites up to 160x160 tiles (stitching <=128x128 blocks when needed).
	if w <= 160 && h <= 160 && data != nil && data.project != nil {
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
		if img, tilePx, err := renderTiledSpriteImage(scene, w, h, floor, opts, 1600); err == nil {
			if showMonsters {
				drawMissingCreatureBadges(img, tpl, floor, w, h, data, opened.editor.Items)
			}
			pngBytes, err := imaging.EncodePNG(img)
			if err != nil {
				return "", 0, err
			}
			return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes), tilePx, nil
		}
	}

	// Fast high-contrast minimap/topology renderer for huge hunts (> 160x160 tiles).
	scale := 5
	if max(w, h) > 220 {
		scale = 4
	}
	if max(w, h) > 360 {
		scale = 3
	}
	img := renderMinimapTemplate(tpl, floor, opened.editor, opened.editor.Items, scale, showMonsters)
	pngBytes, err := imaging.EncodePNG(img)
	if err != nil {
		return "", 0, err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes), scale, nil
}

// renderTiledSpriteImage renders real client sprites into an image bounded by maxDim
// on either axis without allocating an oversized full-resolution buffer in RAM.
func renderTiledSpriteImage(scene *maprender.Scene, w, h, floor int, opts maprender.GeometryOptions, maxDim int) (*image.NRGBA, int, error) {
	spriteSize := max(1, scene.SpriteSize)
	fullW, fullH := w*spriteSize, h*spriteSize
	factor := 1
	for fullW/factor > maxDim || fullH/factor > maxDim {
		factor++
	}
	if factor == 1 && w*h <= maprender.MaxImageTiles {
		img, err := maprender.RenderImage(scene, image.Rect(0, 0, w, h), floor, opts)
		return img, spriteSize, err
	}

	dstW, dstH := max(1, fullW/factor), max(1, fullH/factor)
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	for i := 3; i < len(dst.Pix); i += 4 {
		dst.Pix[i] = 0xFF
	}

	// Choose a block size that keeps each temporary sprite block small (<= ~16 MiB)
	// and aligned to integer tile boundaries.
	block := 64
	if spriteSize >= 48 {
		block = 32
	}
	for y0 := 0; y0 < h; y0 += block {
		y1 := min(h, y0+block)
		for x0 := 0; x0 < w; x0 += block {
			x1 := min(w, x0+block)
			part, err := maprender.RenderImage(scene, image.Rect(x0, y0, x1, y1), floor, opts)
			if err != nil {
				return nil, 0, err
			}
			blitDownsampledBlock(dst, part, x0*spriteSize, y0*spriteSize, factor)
		}
	}
	return dst, max(1, spriteSize/factor), nil
}

func blitDownsampledBlock(dst, src *image.NRGBA, originX, originY, factor int) {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := dst.Bounds().Dx(), dst.Bounds().Dy()
	dxStart := originX / factor
	dyStart := originY / factor
	dxEnd := min(dw, (originX+sw+factor-1)/factor)
	dyEnd := min(dh, (originY+sh+factor-1)/factor)

	for dy := dyStart; dy < dyEnd; dy++ {
		sy0 := max(0, dy*factor-originY)
		sy1 := min(sh, (dy+1)*factor-originY)
		if sy0 >= sy1 {
			continue
		}
		for dx := dxStart; dx < dxEnd; dx++ {
			sx0 := max(0, dx*factor-originX)
			sx1 := min(sw, (dx+1)*factor-originX)
			if sx0 >= sx1 {
				continue
			}
			var r, g, b, a, count uint32
			for sy := sy0; sy < sy1; sy++ {
				rowOff := sy*src.Stride + sx0*4
				for sx := sx0; sx < sx1; sx++ {
					off := rowOff + (sx-sx0)*4
					r += uint32(src.Pix[off])
					g += uint32(src.Pix[off+1])
					b += uint32(src.Pix[off+2])
					a += uint32(src.Pix[off+3])
					count++
				}
			}
			if count > 0 {
				dst.SetNRGBA(dx, dy, color.NRGBA{
					R: uint8(r / count),
					G: uint8(g / count),
					B: uint8(b / count),
					A: uint8(a / count),
				})
			}
		}
	}
}

func creatureHasVisibleSprite(name string, data *mapData, items *otitems.DB) bool {
	if data == nil || data.project == nil {
		return false
	}
	outfit, known := data.outfit(name)
	if !known {
		return false
	}
	if outfit.LookItem != 0 {
		if items == nil {
			return false
		}
		typ := items.Get(outfit.LookItem)
		if typ == nil || typ.ClientID == 0 {
			return false
		}
		th, err := data.project.Thing(thing.CategoryItem, uint32(typ.ClientID))
		return err == nil && th != nil
	}
	if outfit.LookType == 0 {
		return false
	}
	th, err := data.project.Thing(thing.CategoryOutfit, uint32(outfit.LookType))
	return err == nil && th != nil
}

func drawMissingCreatureBadges(img *image.NRGBA, tpl *huntforge.Template, floor, cols, rows int, data *mapData, items *otitems.DB) {
	if cols <= 0 || rows <= 0 {
		return
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	scaleX := float64(w) / float64(cols)
	scaleY := float64(h) / float64(rows)
	rad := max(2, int(math.Round(min(scaleX, scaleY)/4.5)))
	dot := color.NRGBA{255, 86, 118, 255}
	rim := color.NRGBA{15, 10, 20, 255}
	for _, pc := range tpl.Creatures {
		if pc.Z != floor {
			continue
		}
		if creatureHasVisibleSprite(pc.Creature.Name, data, items) {
			continue
		}
		cx := int(math.Round((float64(pc.X) + 0.5) * scaleX))
		cy := int(math.Round((float64(pc.Y) + 0.5) * scaleY))
		for dy := -rad - 1; dy <= rad+1; dy++ {
			for dx := -rad - 1; dx <= rad+1; dx++ {
				px, py := cx+dx, cy+dy
				if px < 0 || py < 0 || px >= w || py >= h {
					continue
				}
				d2 := dx*dx + dy*dy
				if d2 <= rad*rad {
					img.SetNRGBA(px, py, dot)
				} else if d2 <= (rad+1)*(rad+1) {
					img.SetNRGBA(px, py, rim)
				}
			}
		}
	}
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
