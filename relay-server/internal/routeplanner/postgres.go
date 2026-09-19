package routeplanner

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresLoader struct{ Pool *pgxpool.Pool }

func (l PostgresLoader) ActiveVersion(ctx context.Context) (int64, error) {
	var id int64
	err := l.Pool.QueryRow(ctx, "SELECT id FROM eve_sde_builds WHERE state='active'").Scan(&id)
	return id, err
}
func (l PostgresLoader) LoadGraph(ctx context.Context, id int64) (Graph, error) {
	g := Graph{Version: id, Systems: map[int64]System{}, Neighbors: map[int64][]int64{}}
	rows, err := l.Pool.Query(ctx, "SELECT system_id,name,security_status FROM eve_systems WHERE build_id=$1", id)
	if err != nil {
		return Graph{}, err
	}
	for rows.Next() {
		var s System
		if err := rows.Scan(&s.ID, &s.Name, &s.Security); err != nil {
			rows.Close()
			return Graph{}, err
		}
		g.Systems[s.ID] = s
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Graph{}, err
	}
	rows.Close()
	rows, err = l.Pool.Query(ctx, "SELECT system_id,destination_system_id FROM eve_stargates WHERE build_id=$1 ORDER BY system_id,destination_system_id", id)
	if err != nil {
		return Graph{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var a, b int64
		if err := rows.Scan(&a, &b); err != nil {
			return Graph{}, err
		}
		g.Neighbors[a] = append(g.Neighbors[a], b)
	}
	return g, rows.Err()
}
