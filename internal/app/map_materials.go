package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nekiro/ots-creator/internal/fsutil"
	"github.com/nekiro/ots-creator/internal/mapping/mapedit"
	"github.com/nekiro/ots-creator/internal/mapping/materials"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

func (ms *MapService) saveMaterials(data *mapData) error {
	if data.materialsPath == "" {
		return errors.New("no materials file")
	}
	if err := os.MkdirAll(filepath.Dir(data.materialsPath), fsutil.DirMode); err != nil {
		return err
	}
	if err := data.materials.Save(data.materialsPath); err != nil {
		return err
	}
	data.index()
	for _, opened := range ms.maps {
		if opened.editor.Items.Mode == data.items.Mode {
			opened.editor.SetBrushes(data.brushes)
		}
	}
	return nil
}

func (ms *MapService) editMaterials(fn func(materialSet *materials.Materials) error) error {
	return ms.withData(func(data *mapData) error {
		backup, _ := json.Marshal(data.materials)
		if err := fn(data.materials); err != nil {
			if restored, parseErr := materials.Parse(backup); parseErr == nil {
				data.materials = restored
			}
			return err
		}
		if err := ms.saveMaterials(data); err != nil {
			if restored, parseErr := materials.Parse(backup); parseErr == nil {
				data.materials = restored
			}
			return err
		}
		return nil
	})
}

type MaterialsResult struct {
	Data     DataInfo `json:"data"`
	Warnings []string `json:"warnings"`
}

func (ms *MapService) ImportRME(dir string) (result MaterialsResult, err error) {
	err = ms.withData(func(data *mapData) error {
		found, err := materials.FindRMEData(dir, int(data.project.Info().Version.Value))
		if err != nil {
			return err
		}
		imported, warnings, err := materials.ImportRME(found, data.items.FromClient)
		if err != nil {
			return err
		}
		backup, _ := json.Marshal(data.materials)
		data.materials.Merge(imported)
		if err := ms.saveMaterials(data); err != nil {
			if restored, parseErr := materials.Parse(backup); parseErr == nil {
				data.materials = restored
			}
			return err
		}
		result = MaterialsResult{Data: data.info(), Warnings: warnings}
		return nil
	})
	return result, err
}

func (ms *MapService) ImportMaterials(path string) (result MaterialsResult, err error) {
	err = ms.withData(func(data *mapData) error {
		imported, err := materials.Load(path)
		if err != nil {
			return err
		}
		backup, _ := json.Marshal(data.materials)
		data.materials.Merge(imported)
		if err := ms.saveMaterials(data); err != nil {
			if restored, parseErr := materials.Parse(backup); parseErr == nil {
				data.materials = restored
			}
			return err
		}
		result = MaterialsResult{Data: data.info()}
		return nil
	})
	return result, err
}

func (ms *MapService) ExportMaterials(path string) error {
	return ms.withData(func(data *mapData) error { return data.materials.Save(path) })
}

func (ms *MapService) Brush(name string) (brush *materials.Brush, err error) {
	err = ms.withData(func(data *mapData) error {
		if brush = data.materials.Brush(name); brush != nil {
			return nil
		}
		if data.effective != nil {
			if brush = data.effective.Brush(name); brush != nil {
				return nil
			}
		}
		return fmt.Errorf("no brush %q", name)
	})
	return brush, err
}

func (ms *MapService) Borders() (borders []*materials.Border, err error) {
	err = ms.withData(func(data *mapData) error {
		if data.effective != nil {
			borders = data.effective.Borders
		} else {
			borders = data.materials.Borders
		}
		return nil
	})
	return borders, err
}

func (ms *MapService) SaveBorder(border materials.Border) (id uint32, err error) {
	err = ms.editMaterials(func(materialSet *materials.Materials) error {
		id = materialSet.SaveBorder(border)
		return nil
	})
	return id, err
}

// SaveBrush creates or replaces a brush; oldName is its current name (""
// for a new brush). tilesets, when not nil, are the collections the brush is
// in afterwards: missing ones are created, others no longer list it.
func (ms *MapService) SaveBrush(oldName string, brush materials.Brush, tilesets []string) error {
	return ms.editMaterials(func(materialSet *materials.Materials) error { return materialSet.SaveBrush(oldName, brush, tilesets) })
}

func (ms *MapService) RemoveBrush(name string) error {
	return ms.withData(func(data *mapData) error {
		if data.materials.Brush(name) == nil && data.effective != nil && data.effective.Brush(name) != nil {
			return errors.New("built-in brushes cannot be removed; edit them to customize")
		}
		backup, _ := json.Marshal(data.materials)
		data.materials.RemoveBrush(name)
		if err := ms.saveMaterials(data); err != nil {
			if restored, parseErr := materials.Parse(backup); parseErr == nil {
				data.materials = restored
			}
			return err
		}
		return nil
	})
}

func (ms *MapService) SaveTileset(oldName string, tileset materials.Tileset) error {
	return ms.editMaterials(func(materialSet *materials.Materials) error { return materialSet.SaveTileset(oldName, tileset) })
}

// RemoveTileset deletes a tileset (its brushes stay).
func (ms *MapService) RemoveTileset(name string) error {
	return ms.editMaterials(func(materialSet *materials.Materials) error { materialSet.RemoveTileset(name); return nil })
}

func (ms *MapService) AddToTileset(name string, items []uint16) error {
	return ms.editMaterials(func(materialSet *materials.Materials) error { materialSet.AddRawItems(name, items); return nil })
}

func (ms *MapService) SaveCreature(creature materials.CreatureType) error {
	return ms.editMaterials(func(materialSet *materials.Materials) error { return materialSet.SaveCreature(creature) })
}

// DoodadFromSelection creates a doodad brush from the selected items, as
// one composite relative to the selection's top-left tile.
func (ms *MapService) DoodadFromSelection(id int, name string, tilesets []string) error {
	return ms.withMap(id, func(opened *openMap, data *mapData) error {
		clip := opened.editor.Copy()
		if clip == nil {
			return errors.New("select the items of the doodad first")
		}
		brush, err := clip.DoodadBrush(name)
		if err != nil {
			return err
		}
		backup, _ := json.Marshal(data.materials)
		if err := data.materials.SaveBrush("", brush, tilesets); err != nil {
			return err
		}
		if err := ms.saveMaterials(data); err != nil {
			if restored, parseErr := materials.Parse(backup); parseErr == nil {
				data.materials = restored
			}
			return err
		}
		return nil
	})
}

// LearnBorder analyzes the active map selection to infer 12-edge auto-borders.
func (ms *MapService) LearnBorder(id int) (*mapedit.LearnedBorderReport, error) {
	var report *mapedit.LearnedBorderReport
	err := ms.withMapRead(id, func(opened *openMap, _ *mapData) error {
		var err error
		report, err = opened.editor.LearnBorderFromSelection()
		return err
	})
	return report, err
}

// LearnBordersFromMap scans every floor of the open map to automatically
// discover 12-edge auto-borders for custom or unconfigured grounds.
func (ms *MapService) LearnBordersFromMap(id int, onlyMissing bool) (*mapedit.WholeMapBorderReport, error) {
	var report *mapedit.WholeMapBorderReport
	err := ms.withMapRead(id, func(opened *openMap, _ *mapData) error {
		report = opened.editor.LearnBordersFromMap(onlyMissing)
		return nil
	})
	return report, err
}

// SaveLearnedBordersBatch persists multiple auto-learned borders and their
// ground brushes in a single atomic save so Auto-Bordering works immediately.
func (ms *MapService) SaveLearnedBordersBatch(entries []mapedit.AutoLearnedBorder) (saved int, err error) {
	if len(entries) == 0 {
		return 0, errors.New("select at least one learned border to save")
	}
	err = ms.editMaterials(func(materialSet *materials.Materials) error {
		for _, entry := range entries {
			cleanEdges := map[string]uint16{}
			for edge, eid := range entry.Edges {
				if eid != 0 {
					cleanEdges[edge] = eid
				}
			}
			if len(cleanEdges) == 0 {
				continue
			}
			borderID := materialSet.SaveBorder(materials.Border{Edges: cleanEdges})
			saved++
			brushName := strings.TrimSpace(entry.Name)
			if brushName == "" && len(entry.GroundIDs) > 0 {
				brushName = fmt.Sprintf("ground_%d", entry.GroundIDs[0])
			}
			if brushName == "" || len(entry.GroundIDs) == 0 {
				continue
			}
			align := entry.Align
			if align != "inner" {
				align = "outer"
			}
			toTarget := strings.TrimSpace(entry.ToTarget)
			if toTarget == "" {
				if align == "inner" {
					toTarget = "none"
				} else {
					toTarget = "all"
				}
			}
			zOrder := entry.ZOrder
			if zOrder <= 0 {
				zOrder = 3500
			}
			var brush materials.Brush
			oldName := ""
			if existing := materialSet.Brush(brushName); existing != nil {
				brush = *existing
				oldName = existing.Name
			} else {
				brush = materials.Brush{
					Name:   brushName,
					Type:   materials.TypeGround,
					Look:   entry.GroundIDs[0],
					ZOrder: zOrder,
				}
				for _, gid := range entry.GroundIDs {
					if gid != 0 {
						brush.Items = append(brush.Items, materials.Chance{ID: gid, Chance: 100})
					}
				}
			}
			if brush.Look == 0 && len(entry.GroundIDs) > 0 {
				brush.Look = entry.GroundIDs[0]
			}
			brush.Borders = append([]materials.BorderRule{{Align: align, To: toTarget, Border: borderID}}, brush.Borders...)
			if err := materialSet.SaveBrush(oldName, brush, []string{"Auto-Learned Borders"}); err != nil {
				return err
			}
		}
		if saved == 0 {
			return errors.New("no valid border edges to save")
		}
		return nil
	})
	return saved, err
}

// ExportLearnedBordersBatchXML writes all selected learned borders, ground
// brushes, and an Auto-Learned Borders tileset into one ready-to-use RME XML.
func (ms *MapService) ExportLearnedBordersBatchXML(path string, entries []mapedit.AutoLearnedBorder, startBorderID uint32) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("no output file selected")
	}
	if len(entries) == 0 {
		return "", errors.New("select at least one learned border to export")
	}
	xmlText := mapedit.FormatMultiLearnedBordersXML(entries, startBorderID)
	if err := fsutil.WriteAtomic(path, []byte(xmlText)); err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}

// SaveLearnedBorder persists a learned border and attaches it to a ground brush
// (creating or updating the brush in the "Learned Borders" collection) so it is
// immediately active with auto-bordering on the map.
func (ms *MapService) SaveLearnedBorder(name string, groundIDs []uint16, edges map[string]uint16, align, toTarget string, zOrder int) (borderID uint32, err error) {
	cleanEdges := map[string]uint16{}
	for edge, id := range edges {
		if id != 0 {
			cleanEdges[edge] = id
		}
	}
	if len(cleanEdges) == 0 {
		return 0, errors.New("at least one border edge item is required")
	}
	err = ms.editMaterials(func(materialSet *materials.Materials) error {
		borderID = materialSet.SaveBorder(materials.Border{Edges: cleanEdges})
		brushName := strings.TrimSpace(name)
		if brushName == "" || len(groundIDs) == 0 {
			return nil
		}
		if align != "inner" {
			align = "outer"
		}
		if toTarget == "" {
			if align == "inner" {
				toTarget = "none"
			} else {
				toTarget = "all"
			}
		}
		if zOrder <= 0 {
			zOrder = 3500
		}
		var brush materials.Brush
		oldName := ""
		if existing := materialSet.Brush(brushName); existing != nil {
			brush = *existing
			oldName = existing.Name
		} else {
			brush = materials.Brush{
				Name:   brushName,
				Type:   materials.TypeGround,
				Look:   groundIDs[0],
				ZOrder: zOrder,
			}
			for _, gid := range groundIDs {
				if gid != 0 {
					brush.Items = append(brush.Items, materials.Chance{ID: gid, Chance: 100})
				}
			}
		}
		if brush.Look == 0 && len(groundIDs) > 0 {
			brush.Look = groundIDs[0]
		}
		brush.Borders = append([]materials.BorderRule{{Align: align, To: toTarget, Border: borderID}}, brush.Borders...)
		return materialSet.SaveBrush(oldName, brush, []string{"Learned Borders"})
	})
	return borderID, err
}

// ExportLearnedBorderXML writes a standalone RME-compatible XML definition for
// the learned border and optional ground brush.
func (ms *MapService) ExportLearnedBorderXML(path string, borderID uint32, name string, groundIDs []uint16, edges map[string]uint16, align, toTarget string, zOrder int) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("no output file selected")
	}
	if borderID == 0 {
		borderID = 1000
	}
	xmlText := mapedit.FormatLearnedBorderXML(materials.Border{ID: borderID, Edges: edges}, name, groundIDs, align, toTarget, zOrder)
	if err := fsutil.WriteAtomic(path, []byte(xmlText)); err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}

// ExportSelectionOTBM saves the current map selection as a standalone .otbm terrain file.
func (ms *MapService) ExportSelectionOTBM(id int, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("no output file selected")
	}
	var savedPath string
	err := ms.withMapRead(id, func(opened *openMap, data *mapData) error {
		selected := opened.editor.Selected()
		if len(selected) == 0 {
			return errors.New("select terrain tiles first")
		}
		lo, hi, ok := opened.editor.SelectionBounds()
		if !ok {
			return errors.New("select terrain tiles first")
		}
		w := int(hi.X) - int(lo.X) + 1
		h := int(hi.Y) - int(lo.Y) + 1
		outMap := otmap.New(uint16(max(64, w+32)), uint16(max(64, h+32)))
		outMap.ItemsMajor = opened.editor.Map.ItemsMajor
		outMap.ItemsMinor = opened.editor.Map.ItemsMinor
		outMap.SpawnFormat = opened.editor.Map.SpawnFormat
		outMap.Description = fmt.Sprintf("Saved Terrain (%dx%d)", w, h)
		const originX, originY = 10, 10
		for _, pos := range selected {
			src := opened.editor.Map.Tile(pos)
			if src == nil {
				continue
			}
			px := originX + (int(pos.X) - int(lo.X))
			py := originY + (int(pos.Y) - int(lo.Y))
			if px < 0 || py < 0 || px > 0xFFFF || py > 0xFFFF {
				continue
			}
			t := src.Clone()
			t.SetSelected(false)
			t.Ground.Selected = false
			for i := range t.Items {
				t.Items[i].Selected = false
			}
			if t.Ext != nil {
				for i := range t.Ext.Creatures {
					t.Ext.Creatures[i].Selected = false
				}
				if t.Ext.Spawn != nil {
					t.Ext.Spawn.Selected = false
				}
				if t.Ext.SpawnNPC != nil {
					t.Ext.SpawnNPC.Selected = false
				}
			}
			outMap.SetTile(otmap.Position{X: uint16(px), Y: uint16(py), Z: pos.Z}, t)
		}
		outMap.ForgetAuxFileNames()
		if _, err := otmap.Save(path, outMap, data.items, opened.editor.Settings.Format()); err != nil {
			return err
		}
		savedPath = filepath.Clean(path)
		return nil
	})
	return savedPath, err
}
