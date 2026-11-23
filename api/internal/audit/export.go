package audit

import (
	"context"
	"net/url"

	"github.com/jackc/pgx/v5"
)

type beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Export records that someone downloaded data: which export or report, the
// format and the filters used. A download changes nothing, so the row has a
// transaction of its own; the file is only sent once the row is written.
func Export(ctx context.Context, conn beginner, actorID, what, format string, query url.Values) error {
	filters := map[string]string{}
	for k, v := range query {
		if k != "format" && len(v) > 0 && v[0] != "" {
			filters[k] = v[0]
		}
	}
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		return Write(ctx, tx, Entry{ActorID: actorID, Action: "export", ObjectType: what,
			New: map[string]any{"format": format, "filters": filters}})
	})
}
