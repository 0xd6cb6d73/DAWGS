package algo

import (
	"context"
	"math"

	"github.com/gammazero/deque"
	"github.com/specterops/dawgs/cardinality"
	"github.com/specterops/dawgs/container"
	"github.com/specterops/dawgs/graph"
	"github.com/specterops/dawgs/util"
)

func StronglyConnectedComponents(ctx context.Context, digraph container.DirectedGraph) ([]cardinality.Duplex[uint64], map[uint64]uint64) {
	defer util.SLogMeasure("StronglyConnectedComponents")()

	type descentCursor struct {
		id        uint64
		branches  []uint64
		branchIdx int
	}

	var (
		numNodes                    = digraph.NumNodes()
		initialAlloc                = int64(math.Sqrt(float64(numNodes)))
		lastSearchedNodeID          = uint64(0)
		index                       = 0
		visitedIndex                = make(map[uint64]int, numNodes)
		lowLinks                    = make(map[uint64]int, numNodes)
		onStack                     = cardinality.NewBitmap64()
		stack                       = make([]uint64, 0, initialAlloc)
		dfsDescentStack             = make([]*descentCursor, 0, initialAlloc)
		stronglyConnectedComponents = make([]cardinality.Duplex[uint64], 0, initialAlloc)
		nodeToSCCIndex              = make(map[uint64]uint64, numNodes)
	)

	digraph.EachNode(func(node uint64) bool {
		if _, visited := visitedIndex[node]; !visited {
			dfsDescentStack = append(dfsDescentStack, &descentCursor{
				id:        node,
				branches:  digraph.AdjacentNodes(node, graph.DirectionOutbound),
				branchIdx: 0,
			})

			for len(dfsDescentStack) > 0 {
				nextCursor := dfsDescentStack[len(dfsDescentStack)-1]

				if nextCursor.branchIdx == 0 {
					// First visit of this node
					visitedIndex[nextCursor.id] = index
					lowLinks[nextCursor.id] = index
					index += 1

					stack = append(stack, nextCursor.id)
					onStack.Add(nextCursor.id)
				} else if lastSearchedNodeID != nextCursor.id {
					// Revisiting this node from a descending DFS
					lowLinks[nextCursor.id] = min(lowLinks[nextCursor.id], lowLinks[lastSearchedNodeID])
				}

				// Set to the current cursor ID for ascent
				lastSearchedNodeID = nextCursor.id

				if nextCursor.branchIdx < len(nextCursor.branches) {
					// Advance to the next branch
					nextBranchID := nextCursor.branches[nextCursor.branchIdx]
					nextCursor.branchIdx += 1

					if _, visited := visitedIndex[nextBranchID]; !visited {
						// This node has not been visited yet, run a DFS for it
						lastSearchedNodeID = nextBranchID

						dfsDescentStack = append(dfsDescentStack, &descentCursor{
							id:        nextBranchID,
							branches:  digraph.AdjacentNodes(nextBranchID, graph.DirectionOutbound),
							branchIdx: 0,
						})
					} else if onStack.Contains(nextBranchID) {
						// Branch is on the traversal stack; hence it is also in the current SCC
						lowLinks[nextCursor.id] = min(lowLinks[nextCursor.id], visitedIndex[nextBranchID])
					}
				} else {
					// Finished visiting branches; exiting node
					dfsDescentStack = dfsDescentStack[:len(dfsDescentStack)-1]

					if lowLinks[nextCursor.id] == visitedIndex[nextCursor.id] {
						var (
							scc   = cardinality.NewBitmap64()
							sccID = uint64(len(stronglyConnectedComponents))
						)

						for {
							// Unwind the stack to the root of the component
							currentNode := stack[len(stack)-1]
							stack = stack[:len(stack)-1]

							onStack.Remove(currentNode)

							scc.Add(currentNode)

							// Reverse index origin node to SCC
							nodeToSCCIndex[currentNode] = sccID

							if currentNode == nextCursor.id {
								break
							}
						}

						stronglyConnectedComponents = append(stronglyConnectedComponents, scc)
					}
				}
			}
		}

		return util.IsContextLive(ctx)
	})

	return stronglyConnectedComponents, nodeToSCCIndex
}

type ComponentGraph struct {
	componentMembers      []cardinality.Duplex[uint64]
	memberComponentLookup map[uint64]uint64
	digraph               container.DirectedGraph
}

func (s ComponentGraph) Digraph() container.DirectedGraph {
	return s.digraph
}

func (s ComponentGraph) ContainingComponent(memberID uint64) (uint64, bool) {
	component, inComponentDigraph := s.memberComponentLookup[memberID]
	return component, inComponentDigraph
}

func (s ComponentGraph) CollectComponentMembers(componentID uint64, members cardinality.Duplex[uint64]) {
	members.Or(s.componentMembers[componentID])
}

func (s ComponentGraph) ComponentSearch(startComponent, endComponent uint64) bool {
	if startComponent == endComponent {
		return true
	}

	var (
		traversals deque.Deque[uint64]
		visited    = cardinality.NewBitmap64()
		reachable  = false
	)

	traversals.PushBack(startComponent)

	for remainingTraversals := traversals.Len(); !reachable && remainingTraversals > 0; remainingTraversals = traversals.Len() {
		nextComponent := traversals.PopFront()

		s.digraph.EachAdjacentNode(nextComponent, graph.DirectionOutbound, func(adjacentComponent uint64) bool {
			reachable = adjacentComponent == endComponent

			if !reachable {
				if visited.CheckedAdd(adjacentComponent) {
					traversals.PushBack(adjacentComponent)
				}
			}

			return !reachable
		})
	}

	return reachable
}

func (s ComponentGraph) ComponentReachable(startComponent, endComponent uint64) bool {
	if startComponent == endComponent {
		return true
	}

	var (
		outboundQueue      deque.Deque[uint64]
		inboundQueue       deque.Deque[uint64]
		outboundComponents = cardinality.NewBitmap64()
		inboundComponents  = cardinality.NewBitmap64()
		visitedComponents  = cardinality.NewBitmap64()
		reachable          = false
	)

	outboundQueue.PushBack(startComponent)
	outboundComponents.Add(startComponent)

	inboundQueue.PushBack(endComponent)
	inboundComponents.Add(endComponent)

	for !reachable {
		var (
			outboundQueueLen = outboundQueue.Len()
			inboundQueueLen  = inboundQueue.Len()
		)

		if outboundQueueLen > 0 && outboundQueueLen <= inboundQueueLen {
			nextComponent := outboundQueue.PopFront()

			if !visitedComponents.CheckedAdd(nextComponent) {
				continue
			}

			s.digraph.EachAdjacentNode(nextComponent, graph.DirectionOutbound, func(adjacentComponent uint64) bool {
				if outboundComponents.CheckedAdd(adjacentComponent) {
					// Haven't seen this component yet, append to the traversal queue and check for reachability
					outboundQueue.PushBack(adjacentComponent)
					reachable = inboundComponents.Contains(adjacentComponent)
				}

				// Continue iterating if not reachable
				return !reachable
			})
		} else if inboundQueueLen > 0 {
			nextComponent := inboundQueue.PopFront()

			s.digraph.EachAdjacentNode(nextComponent, graph.DirectionInbound, func(adjacentComponent uint64) bool {
				if inboundComponents.CheckedAdd(adjacentComponent) {
					// Haven't seen this component yet, append to the traversal queue and check for reachability
					inboundQueue.PushBack(adjacentComponent)
					reachable = outboundComponents.Contains(adjacentComponent)
				}

				// Continue iterating if not reachable
				return !reachable
			})
		} else {
			// No more expansions remain
			break
		}
	}

	return reachable
}

func (s ComponentGraph) ComponentHistogram(originNodes []uint64) map[uint64]uint64 {
	histogram := map[uint64]uint64{}

	for _, originNode := range originNodes {
		if component, inComponent := s.ContainingComponent(originNode); inComponent {
			histogram[component] += 1
		}
	}

	return histogram
}

func (s ComponentGraph) OriginReachable(startID, endID uint64) bool {
	var (
		startComponent, startInComponent = s.ContainingComponent(startID)
		endComponent, endInComponent     = s.ContainingComponent(endID)
	)

	if !startInComponent || !endInComponent {
		return false
	}

	return s.ComponentReachable(startComponent, endComponent)
}

func NewComponentGraph(ctx context.Context, originGraph container.DirectedGraph) ComponentGraph {
	var (
		componentMembers, memberComponentLookup = StronglyConnectedComponents(ctx, originGraph)
		componentDigraph                        = container.NewAdjacencyMapGraph()
		nextEdgeID                              = uint64(1)
	)

	defer util.SLogMeasure("NewComponentGraph")()

	// Ensure all components are present as vertices, even if they have no edges
	for componentID := range componentMembers {
		componentDigraph.Nodes().Add(uint64(componentID))
	}

	originGraph.EachNode(func(node uint64) bool {
		nodeComponent := memberComponentLookup[node]

		originGraph.EachAdjacentNode(node, graph.DirectionInbound, func(adjacent uint64) bool {
			if adjacentComponent := memberComponentLookup[adjacent]; nodeComponent != adjacentComponent {
				componentDigraph.AddEdge(nextEdgeID, adjacentComponent, nodeComponent)
				nextEdgeID += 1
			}

			return util.IsContextLive(ctx)
		})

		originGraph.EachAdjacentNode(node, graph.DirectionOutbound, func(adjacent uint64) bool {
			if adjacentComponent := memberComponentLookup[adjacent]; nodeComponent != adjacentComponent {
				componentDigraph.AddEdge(nextEdgeID, nodeComponent, adjacentComponent)
				nextEdgeID += 1
			}

			return util.IsContextLive(ctx)
		})

		return util.IsContextLive(ctx)
	})

	return ComponentGraph{
		componentMembers:      componentMembers,
		memberComponentLookup: memberComponentLookup,
		digraph:               componentDigraph,
	}
}
