package materials

import (
	_ "embed"
	"strings"
	"sync"
)

//go:embed builtin_creatures.xml
var builtinCreaturesXML []byte

var (
	builtinCreaturesOnce sync.Once
	builtinCreaturesList []*CreatureType
	builtinCreaturesByLC map[string]*CreatureType
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
