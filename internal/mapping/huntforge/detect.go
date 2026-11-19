package huntforge

import (
	"sort"
	"strings"

	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

type monsterEntry struct {
	pos  otmap.Position
	name string
}

type bucketKey struct {
	bx, by, z int
}

// Detect clusters monsters on gameMap that are within linkDistance tiles of each other
// on the same floor, returning hunt suggestions sorted alphabetically.
func Detect(gameMap *otmap.Map, linkDistance, minCreatures, margin int) []Suggestion {
	if gameMap == nil {
		return nil
	}
	if linkDistance <= 0 {
		linkDistance = 12
	}
	if minCreatures <= 0 {
		minCreatures = 8
	}
	if margin < 0 {
		margin = 10
	}

	var all []monsterEntry
	gameMap.Each(func(pos otmap.Position, tile *otmap.Tile) {
		for _, cr := range tile.Creatures() {
			if !cr.NPC && cr.Name != "" {
				all = append(all, monsterEntry{pos: pos, name: cr.Name})
			}
		}
	})
	if len(all) == 0 {
		return nil
	}

	parent := make([]int, len(all))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	buckets := make(map[bucketKey][]int, len(all)/4+1)
	for i, m := range all {
		k := bucketKey{int(m.pos.X) / linkDistance, int(m.pos.Y) / linkDistance, int(m.pos.Z)}
		buckets[k] = append(buckets[k], i)
	}
	for i, m := range all {
		bx, by, z := int(m.pos.X)/linkDistance, int(m.pos.Y)/linkDistance, int(m.pos.Z)
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				list := buckets[bucketKey{bx + dx, by + dy, z}]
				for _, j := range list {
					if j <= i {
						continue
					}
					q := all[j].pos
					if max(absInt(int(m.pos.X)-int(q.X)), absInt(int(m.pos.Y)-int(q.Y))) <= linkDistance {
						union(i, j)
					}
				}
			}
		}
	}

	groups := map[int][]monsterEntry{}
	for i, m := range all {
		r := find(i)
		groups[r] = append(groups[r], m)
	}

	out := make([]Suggestion, 0, len(groups))
	for _, members := range groups {
		if len(members) < minCreatures {
			continue
		}
		x1, y1 := int(members[0].pos.X), int(members[0].pos.Y)
		x2, y2 := x1, y1
		z := int(members[0].pos.Z)
		counts := map[string]int{}
		for _, m := range members {
			x, y := int(m.pos.X), int(m.pos.Y)
			if x < x1 {
				x1 = x
			}
			if x > x2 {
				x2 = x
			}
			if y < y1 {
				y1 = y
			}
			if y > y2 {
				y2 = y
			}
			counts[m.name]++
		}
		species := sortedSpecies(counts)
		var top []string
		for i := 0; i < len(species) && i < 2; i++ {
			top = append(top, species[i].Name)
		}
		out = append(out, Suggestion{
			Name: strings.Join(top, " / "),
			Region: Region{
				X1: max(0, x1-margin),
				Y1: max(0, y1-margin),
				X2: min(0xFFFF, x2+margin),
				Y2: min(0xFFFF, y2+margin),
				Z1: z,
				Z2: z,
			},
			Creatures: len(members),
			Species:   species,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Creatures > out[j].Creatures
	})
	return out
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
