package algo

import (
	"maps"
	"sort"
)

type Weight = float64
type WeightMap map[uint64]Weight

func (s WeightMap) Keys() []uint64 {
	keys := make([]uint64, 0, len(s))

	for key := range s {
		keys = append(keys, key)
	}

	return keys
}

func (s WeightMap) Copy() WeightMap {
	return maps.Clone(s)
}

func (s WeightMap) MultiplyInclusiveOnly(other WeightMap) {
	for k := range s {
		s[k] *= other[k]
	}
}

func (s WeightMap) SumInclusiveOnly(other WeightMap) {
	for k := range s {
		s[k] += other[k]
	}
}

func (s WeightMap) VisitSorted(visitor func(k uint64, v Weight) bool) {
	type tuple struct {
		key   uint64
		value Weight
	}

	sorted := make([]tuple, 0, len(s))

	for k, v := range s {
		sorted = append(sorted, tuple{
			key:   k,
			value: v,
		})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].value > sorted[j].value
	})

	for _, nextTuple := range sorted {
		if !visitor(nextTuple.key, nextTuple.value) {
			break
		}
	}
}
