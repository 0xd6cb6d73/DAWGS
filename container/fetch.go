package container

import (
	"context"

	"github.com/specterops/dawgs/cypher/models/cypher"
	"github.com/specterops/dawgs/database"
	"github.com/specterops/dawgs/graph"
	"github.com/specterops/dawgs/query"
	"github.com/specterops/dawgs/util"
)

const (
	channelBufferLen = 4096
)

type anonymousEdge struct {
	EdgeID  uint64
	StartID uint64
	EndID   uint64
}

func FetchAdjacencyGraph(ctx context.Context, graphDB database.Instance, relationshipFilter cypher.SyntaxNode) (DirectedGraph, error) {
	digraph := NewAdjacencyMapGraph()

	return digraph, graphDB.Session(ctx, func(ctx context.Context, driver database.Driver) error {
		builder := query.New()

		if relationshipFilter != nil {
			builder.Where(relationshipFilter)
		}

		builder.Return(
			query.Relationship().ID(),
			query.Start().ID(),
			query.End().ID(),
		)

		if preparedQuery, err := builder.Build(); err != nil {
			return err
		} else {
			result := driver.Exec(ctx, preparedQuery.Query, preparedQuery.Parameters)
			defer result.Close(ctx)

			for result.HasNext(ctx) {
				var (
					edgeID  uint64
					startID uint64
					endID   uint64
				)

				if err := result.Scan(&edgeID, &startID, &endID); err != nil {
					return err
				}

				digraph.AddEdge(edgeID, startID, endID)
			}

			return result.Error()
		}
	})
}

func FetchKindDatabase(ctx context.Context, graphDB database.Instance) (KindDatabase, error) {
	defer util.SLogMeasure("FetchKindDatabase")()

	edgeKinds := KindMap{}

	if err := graphDB.Session(ctx, func(ctx context.Context, driver database.Driver) error {
		builder := query.New()
		builder.Return(query.Relationship().ID(), query.Relationship().Kind())

		if builtQuery, err := builder.Build(); err != nil {
			return err
		} else {
			var (
				result = driver.Exec(ctx, builtQuery.Query, builtQuery.Parameters)
				edgeID uint64
				kind   graph.Kind
			)

			for result.HasNext(ctx) {
				if err := result.Scan(&edgeID, &kind); err != nil {
					return err
				}

				edgeKinds.Add(kind, edgeID)
			}

			result.Close(ctx)
			return result.Error()
		}
	}); err != nil {
		return KindDatabase{}, err
	}

	return KindDatabase{
		EdgeKindMap: edgeKinds,
	}, nil
}

type TSDB struct {
	Triplestore Triplestore
	EdgeKinds   KindMap
}

func FetchTriplestore(ctx context.Context, graphDB database.Instance, filter cypher.SyntaxNode) (TSDB, error) {
	tsdb := TSDB{
		Triplestore: NewTriplestore(),
		EdgeKinds:   KindMap{},
	}

	defer util.SLogMeasure("FetchTriplestore")()

	return tsdb, graphDB.Session(ctx, func(ctx context.Context, driver database.Driver) error {
		query := query.New().Return(
			query.Start().ID(),
			query.Relationship().ID(),
			query.Relationship().Kind(),
			query.End().ID(),
		)

		if filter != nil {
			query.Where(filter)
		}

		if builtQuery, err := query.Build(); err != nil {
			return err
		} else {
			result := driver.Exec(ctx, builtQuery.Query, builtQuery.Parameters)
			defer result.Close(ctx)

			for result.HasNext(ctx) {
				var (
					startID          uint64
					relationshipID   uint64
					relationshipKind graph.Kind
					endID            uint64
				)

				if err := result.Scan(&startID, &relationshipID, &relationshipKind, &endID); err != nil {
					return err
				}

				tsdb.Triplestore.AddEdge(relationshipID, startID, endID)
				tsdb.EdgeKinds.Add(relationshipKind, relationshipID)
			}

			return result.Error()
		}
	})
}
