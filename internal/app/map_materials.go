package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nekiro/ots-creator/internal/fsutil"
	"github.com/nekiro/ots-creator/internal/mapping/materials"
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
