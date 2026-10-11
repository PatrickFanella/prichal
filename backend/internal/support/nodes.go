package support

import (
	"context"
	"encoding/json"
)

// Nodes projects bounded management measurements only; it never contacts a host.
func (s *Store) Nodes(ctx context.Context, owner string) (json.RawMessage, error) {
	role, err := s.role(ctx, owner)
	if err != nil || role == "" {
		return nil, ErrNotFound
	}
	var view []byte
	err = s.Pool.QueryRow(ctx, `WITH page AS (
 SELECT n.id,n.region,n.state,n.operator_slots,n.slots,n.measured_cpus,n.measured_memory_bytes,n.last_verified_at,
 (SELECT count(*) FROM instance_placements p WHERE p.node_id=n.id) AS reserved,
 (n.state='active' AND n.last_verified_at>now()-interval '1 minute' AND n.last_verified_at<=now()) AS fresh
 FROM hosting_nodes n ORDER BY n.created_at,n.id LIMIT 101
), visible AS (SELECT * FROM page LIMIT 100)
SELECT jsonb_build_object('items',COALESCE((SELECT jsonb_agg(jsonb_build_object(
 'id',id,'region',region,'state',state,'operatorSlots',operator_slots,'slots',slots,'reserved',reserved,
 'fresh',COALESCE(fresh,false),'available',CASE WHEN fresh THEN GREATEST(slots-reserved,0) ELSE 0 END,
 'cpus',COALESCE(measured_cpus,0),'memoryBytes',COALESCE(measured_memory_bytes,0),'checkedAt',last_verified_at
 )) FROM visible),'[]'::jsonb),'hasMore',(SELECT count(*)>100 FROM page))`).Scan(&view)
	return view, err
}
