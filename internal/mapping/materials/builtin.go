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
	builtinClientOnce sync.Once
	builtinClientMat  *Materials
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

func parseBuiltinGzip(data []byte) *Materials {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return New()
	}
	defer gz.Close()
	raw, err := io.ReadAll(gz)
	if err != nil {
		return New()
	}
	m, err := Parse(raw)
	if err != nil {
		return New()
	}
	return m
}

// BuiltinMaterials returns the bundled read-only NexaMap brush, border, and
// tileset catalog for the requested item ID mode (ModeOTB = ServerID,
// ModeClient = ClientID). Callers must not mutate the returned *Materials.
func BuiltinMaterials(mode otitems.Mode) *Materials {
	if mode == otitems.ModeClient {
		builtinClientOnce.Do(func() {
			builtinClientMat = parseBuiltinGzip(builtinMaterialsClientGZ)
		})
		return builtinClientMat
	}
	builtinOTBOnce.Do(func() {
		builtinOTBMat = parseBuiltinGzip(builtinMaterialsOTBGZ)
	})
	return builtinOTBMat
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
