package container

import (
	"github.com/gammazero/deque"
	"github.com/specterops/dawgs/cardinality"
	"github.com/specterops/dawgs/graph"
)

type Edge struct {
	ID    uint64
	Start uint64
	End   uint64
}

func (s Edge) Pick(direction graph.Direction) uint64 {
	if direction == graph.DirectionOutbound {
		return s.End
	}

	return s.Start
}

type Triplestore interface {
	DirectedGraph

	NumEdges() uint64
	AdjacentEdges(node uint64, direction graph.Direction) []uint64
	EachAdjacentEdge(node uint64, direction graph.Direction, delegate func(next Edge) bool)
}

type triplestore struct {
	nodes        cardinality.Duplex[uint64]
	edges        []Edge
	deletedEdges cardinality.Duplex[uint64]
	startIndex   map[uint64]cardinality.Duplex[uint64]
	endIndex     map[uint64]cardinality.Duplex[uint64]
}

func NewTriplestore() Triplestore {
	return &triplestore{
		nodes:        cardinality.NewBitmap64(),
		deletedEdges: cardinality.NewBitmap64(),
		startIndex:   map[uint64]cardinality.Duplex[uint64]{},
		endIndex:     map[uint64]cardinality.Duplex[uint64]{},
	}
}

func (s *triplestore) DeleteEdge(id uint64) {
	s.deletedEdges.Add(id)
}

func (s *triplestore) NumNodes() uint64 {
	return s.nodes.Cardinality()
}

func (s *triplestore) Nodes() cardinality.Duplex[uint64] {
	return s.nodes.Clone()
}

func (s *triplestore) EachNode(delegate func(node uint64) bool) {
	s.nodes.Each(delegate)
}

func (s *triplestore) AddEdge(edge, start, end uint64) {
	s.edges = append(s.edges, Edge{
		ID:    edge,
		Start: start,
		End:   end,
	})

	edgeIdx := len(s.edges) - 1

	if edgeBitmap, exists := s.startIndex[start]; exists {
		edgeBitmap.Add(uint64(edgeIdx))
	} else {
		edgeBitmap = cardinality.NewBitmap64()
		edgeBitmap.Add(uint64(edgeIdx))

		s.startIndex[start] = edgeBitmap
	}

	if edgeBitmap, exists := s.endIndex[end]; exists {
		edgeBitmap.Add(uint64(edgeIdx))
	} else {
		edgeBitmap = cardinality.NewBitmap64()
		edgeBitmap.Add(uint64(edgeIdx))

		s.endIndex[end] = edgeBitmap
	}

	s.nodes.Add(start, end)
}

func (s *triplestore) adjacentEdgeIndices(node uint64, direction graph.Direction) cardinality.Duplex[uint64] {
	edgeIndices := cardinality.NewBitmap64()

	switch direction {
	case graph.DirectionOutbound:
		if outboundEdges, hasOutbound := s.startIndex[node]; hasOutbound {
			edgeIndices.Or(outboundEdges)
		}

	case graph.DirectionInbound:
		if inboundEdges, hasInbound := s.endIndex[node]; hasInbound {
			edgeIndices.Or(inboundEdges)
		}

	default:
		if outboundEdges, hasOutbound := s.startIndex[node]; hasOutbound {
			edgeIndices.Or(outboundEdges)
		}

		if inboundEdges, hasInbound := s.endIndex[node]; hasInbound {
			edgeIndices.Or(inboundEdges)
		}
	}

	return edgeIndices
}

func (s *triplestore) AdjacentEdges(node uint64, direction graph.Direction) []uint64 {
	var (
		edgeIndices = s.adjacentEdgeIndices(node, direction)
		edgeIDs     = make([]uint64, 0, edgeIndices.Cardinality())
	)

	edgeIndices.Each(func(value uint64) bool {
		edgeIDs = append(edgeIDs, s.edges[value].ID)
		return true
	})

	return edgeIDs
}

func (s *triplestore) adjacent(node uint64, direction graph.Direction) cardinality.Duplex[uint64] {
	nodes := cardinality.NewBitmap64()

	s.adjacentEdgeIndices(node, direction).Each(func(edgeIndex uint64) bool {
		if edge := s.edges[edgeIndex]; !s.deletedEdges.Contains(edge.ID) {
			switch direction {
			case graph.DirectionOutbound:
				nodes.Add(edge.End)

			case graph.DirectionInbound:
				nodes.Add(edge.Start)

			default:
				nodes.Add(edge.End)
				nodes.Add(edge.Start)
			}
		}

		return true
	})

	return nodes
}

func (s *triplestore) AdjacentNodes(node uint64, direction graph.Direction) []uint64 {
	return s.adjacent(node, direction).Slice()
}

func (s *triplestore) EachAdjacentNode(node uint64, direction graph.Direction, delegate func(adjacent uint64) bool) {
	s.adjacent(node, direction).Each(delegate)
}

func (s *triplestore) Degrees(node uint64, direction graph.Direction) uint64 {
	if adjacent := s.adjacent(node, direction); adjacent != nil {
		return adjacent.Cardinality()
	}

	return 0
}

func (s *triplestore) NumEdges() uint64 {
	return uint64(len(s.edges))
}

func (s *triplestore) EachAdjacentEdge(node uint64, direction graph.Direction, delegate func(next Edge) bool) {
	s.adjacentEdgeIndices(node, direction).Each(func(edgeIndex uint64) bool {
		return delegate(s.edges[edgeIndex])
	})
}

func TSBFS(ts Triplestore, nodeID uint64, direction graph.Direction, maxDepth int, descentFilter func(edge Edge) bool, handler func(segment *Segment) bool) int {
	var (
		traversals         deque.Deque[*Segment]
		numImcompletePaths = 0
	)

	traversals.PushBack(&Segment{
		Node: nodeID,
	})

	for remainingTraversals := traversals.Len(); remainingTraversals > 0; remainingTraversals = traversals.Len() {
		var (
			nextSegment  = traversals.PopFront()
			segmentDepth = nextSegment.Depth()
			depthExceded = maxDepth > 0 && maxDepth < segmentDepth
		)

		if !depthExceded {
			ts.EachAdjacentEdge(nextSegment.Node, direction, func(nextEdge Edge) bool {
				if descentFilter(nextEdge) {
					traversals.PushBack(&Segment{
						Node:     nextEdge.Pick(direction),
						Edge:     nextEdge.ID,
						Previous: nextSegment,
					})
				}

				return true
			})
		}

		if segmentDepth > 1 && remainingTraversals-1 == traversals.Len() {
			if depthExceded {
				numImcompletePaths += 1
			}

			if !handler(nextSegment) {
				break
			}
		}
	}

	return numImcompletePaths
}
