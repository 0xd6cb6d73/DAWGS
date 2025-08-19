package container

import (
	"fmt"

	"github.com/gammazero/deque"
	"github.com/specterops/dawgs/cardinality"
	"github.com/specterops/dawgs/graph"
)

type Weight = float64
type AdjacencyMap map[uint64]cardinality.Duplex[uint64]

type KindMap map[graph.Kind]cardinality.Duplex[uint64]

func (s KindMap) Add(kind graph.Kind, member uint64) {
	if members, hasMembers := s[kind]; hasMembers {
		members.Add(member)
	} else {
		s[kind] = cardinality.NewBitmap64With(member)
	}
}

func (s KindMap) FindFirst(id uint64) graph.Kind {
	for kind, membership := range s {
		if membership.Contains(id) {
			return kind
		}
	}

	panic(fmt.Sprintf("Can't find kind for edge ID %d", id))

	return nil
}

func (s KindMap) FindAll(id uint64) graph.Kinds {
	var matchedKinds graph.Kinds

	for kind, membership := range s {
		if membership.Contains(id) {
			matchedKinds = matchedKinds.Add(kind)
		}
	}

	return matchedKinds
}

type KindDatabase struct {
	EdgeKindMap KindMap
	NodeKindMap KindMap
}

func (s KindDatabase) NodeKind(nodeID uint64) graph.Kinds {
	return s.NodeKindMap.FindAll(nodeID)
}

func (s KindDatabase) EdgeKind(edgeID uint64) graph.Kind {
	return s.EdgeKindMap.FindFirst(edgeID)
}

type ShortestPathTerminal struct {
	NodeID   uint64
	Distance Weight
}

type DirectedGraph interface {
	AddEdge(edge, start, end uint64)
	NumNodes() uint64
	Nodes() cardinality.Duplex[uint64]
	EachNode(delegate func(node uint64) bool)
	Degrees(node uint64, direction graph.Direction) uint64
	AdjacentNodes(node uint64, direction graph.Direction) []uint64
	EachAdjacentNode(node uint64, direction graph.Direction, delegate func(adjacent uint64) bool)
}

func Dimensions(digraph DirectedGraph, direction graph.Direction) (uint64, uint64) {
	var largestRow uint64 = 0

	digraph.EachNode(func(node uint64) bool {
		if degrees := digraph.Degrees(node, direction); degrees > largestRow {
			largestRow = degrees
		}

		return true
	})

	return digraph.Nodes().Cardinality(), largestRow
}

func BFSTree(digraph DirectedGraph, nodeID uint64, direction graph.Direction) []ShortestPathTerminal {
	var (
		visited   = cardinality.NewBitmap64()
		queue     deque.Deque[ShortestPathTerminal]
		terminals []ShortestPathTerminal
	)

	queue.PushBack(ShortestPathTerminal{
		NodeID:   nodeID,
		Distance: 0,
	})

	for queue.Len() > 0 {
		nextCursor := queue.PopFront()

		digraph.EachAdjacentNode(nextCursor.NodeID, direction, func(adjacentNodeID uint64) bool {
			if visited.CheckedAdd(adjacentNodeID) {
				terminalCursor := ShortestPathTerminal{
					NodeID:   adjacentNodeID,
					Distance: nextCursor.Distance + 1,
				}

				// If not visited, descend into this node next
				queue.PushBack(terminalCursor)

				// This reachable node represents one of the shortest path terminals
				terminals = append(terminals, terminalCursor)
			}

			return true
		})
	}

	return terminals
}
