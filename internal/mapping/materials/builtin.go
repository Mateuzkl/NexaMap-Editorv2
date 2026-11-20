package materials

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"strings"
	"sync"

	"github.com/nekiro/ots-creator/internal/core/otitems"
)

//go:embed builtin_creatures.xml
var builtinCreaturesXML []byte

//go:embed builtin_materials_otb.json.gz
var builtinMaterialsOTBGZ []byte

//go:embed builtin_materials_client.json.gz
var builtinMaterialsClientGZ []byte

var (
	builtinCreaturesOnce sync.Once
	builtinCreaturesList []*CreatureType
	builtinCreaturesByLC map[string]*CreatureType

	builtinOTBOnce    sync.Once
	builtinOTBMat     *Materials
	builtinOTBErr     error
	builtinClientOnce sync.Once
	builtinClientMat  *Materials
	builtinClientErr  error
)

func initBuiltinCreatures() {
	builtinCreaturesOnce.Do(func() {
		builtinCreaturesByLC = map[string]*CreatureType{}
		list, err := ParseRMECreatures(builtinCreaturesXML)
		if err != nil {
			return
		}
		builtinCreaturesList = list
		for _, c := range list {
			key := strings.ToLower(c.Name)
			if key != "" && builtinCreaturesByLC[key] == nil {
				builtinCreaturesByLC[key] = c
			}
		}
	})
}

func parseBuiltinGzip(data []byte) (*Materials, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return New(), err
	}
	defer gz.Close()
	raw, err := io.ReadAll(gz)
	if err != nil {
		return New(), err
	}
	m, err := Parse(raw)
	if err != nil {
		return New(), err
	}
	return m, nil
}

// BuiltinMaterials returns the bundled read-only NexaMap brush, border, and
// tileset catalog for the requested item ID mode (ModeOTB = ServerID,
// ModeClient = ClientID). Callers must not mutate the returned *Materials.
func BuiltinMaterials(mode otitems.Mode) (*Materials, error) {
	if mode == otitems.ModeClient {
		builtinClientOnce.Do(func() {
			builtinClientMat, builtinClientErr = parseBuiltinGzip(builtinMaterialsClientGZ)
		})
		return builtinClientMat, builtinClientErr
	}
	builtinOTBOnce.Do(func() {
		builtinOTBMat, builtinOTBErr = parseBuiltinGzip(builtinMaterialsOTBGZ)
	})
	return builtinOTBMat, builtinOTBErr
}

// BuiltinCreatures returns the bundled default Tibia monster and NPC outfits
// so creatures render on maps even before importing custom RME brushes or
// pointing at a server monster/npc directory.
func BuiltinCreatures() []*CreatureType {
	initBuiltinCreatures()
	return builtinCreaturesList
}

// BuiltinCreature looks up a bundled default creature by name (case-insensitive).
func BuiltinCreature(name string) *CreatureType {
	initBuiltinCreatures()
	return builtinCreaturesByLC[strings.ToLower(name)]
}
