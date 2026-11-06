package imports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type uploadRequest struct {
	FileName        string `json:"fileName"`
	Format          string `json:"format"`
	Content         string `json:"content"`
	RoutingDomainID string `json:"routingDomainId"`
	Environment     string `json:"environment"`
}

func (r uploadRequest) validate() error {
	switch {
	case r.FileName == "" || len(r.FileName) > 255:
		return httpx.Validation("fileName", "Name the file.")
	case r.Format == templateFormat:
		if strings.TrimSpace(r.Content) == "" || len(r.Content) > maxBody {
			return httpx.Validation("content", "Upload a non-empty file of at most 5 MB.")
		}
		return nil
	case Formats[r.Format] == "":
		return httpx.Validation("format", "Choose a file format.")
	case !httpx.IsUUID(r.RoutingDomainID):
		return httpx.Validation("routingDomainId", "Choose the routing domain these networks belong to.")
	case !environments[r.Environment]:
		return httpx.Validation("environment", "Choose an environment.")
	case strings.TrimSpace(r.Content) == "" || len(r.Content) > maxBytes:
		return httpx.Validation("content", "Upload a non-empty file of at most 5 MB.")
	}
	return nil
}

func (h *Handler) upload(c *gin.Context) {
	var req uploadRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody+1<<16)
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	req.FileName = strings.TrimSpace(req.FileName)
	if err := req.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out Batch
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) (err error) {
		if req.Format == templateFormat {
			out, err = storeTemplate(ctx, tx, actor, req)
		} else {
			out, err = storeUpload(ctx, tx, actor, req)
		}
		return err
	})
	switch db.ErrorCode(err) {
	case db.CodeUnique:
		err = httpx.NewError(http.StatusConflict, "IMPORT_DUPLICATE_FILE", "This exact file is already imported or waiting for approval.")
	case db.CodeForeignKey:
		err = httpx.Validation("routingDomainId", "That routing domain doesn't exist.")
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, out)
}

func storeUpload(ctx context.Context, tx pgx.Tx, actor string, req uploadRequest) (Batch, error) {
	sum := sha256.Sum256([]byte(req.Content))
	hash := hex.EncodeToString(sum[:])
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO import_batches (file_name, file_hash, format, status, uploaded_by, routing_domain_id, environment)
		VALUES ($1, $2, $3, 'validated', $4::uuid, $5::uuid, $6) RETURNING id::text`,
		req.FileName, hash, req.Format, actor, req.RoutingDomainID, req.Environment).Scan(&id); err != nil {
		return Batch{}, err
	}
	lines := ParseFile(req.Format, req.Content)
	if err := classify(ctx, tx, req.RoutingDomainID, lines, map[string]int{}); err != nil {
		return Batch{}, err
	}
	if err := insertRecords(ctx, tx, id, lines); err != nil {
		return Batch{}, err
	}
	if err := recount(ctx, tx, id); err != nil {
		return Batch{}, err
	}
	out, err := loadBatch(ctx, tx, id, false)
	if err != nil {
		return out, err
	}
	return out, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "import.upload", ObjectType: "import_batch", ObjectID: id,
		New: map[string]any{"fileName": req.FileName, "fileHash": hash, "format": req.Format, "summary": out.Summary}})
}

func insertRecords(ctx context.Context, tx pgx.Tx, batchID string, lines []Line) error {
	if len(lines) == 0 {
		return nil
	}
	numbers := make([]int32, len(lines))
	raw, parsed, statuses, errs := make([]string, len(lines)), make([]string, len(lines)), make([]string, len(lines)), make([]string, len(lines))
	for i := range lines {
		l := &lines[i]
		p, _ := json.Marshal(l.Parsed)
		e, _ := json.Marshal(append([]string{}, l.Errors...))
		numbers[i], raw[i], parsed[i], statuses[i], errs[i] = int32(l.Number), l.Raw, string(p), l.Status, string(e)
	}
	_, err := tx.Exec(ctx, `INSERT INTO import_records (batch_id, line_number, raw_text, parsed, status, errors)
		SELECT $1::uuid, n, raw, parsed::jsonb, status, errors::jsonb
		FROM unnest($2::int[], $3::text[], $4::text[], $5::text[], $6::text[]) AS t(n, raw, parsed, status, errors)`,
		batchID, numbers, raw, parsed, statuses, errs)
	return err
}

// classify checks valid lines against the file and the inventory: an exact match
// is a duplicate, any other overlap in the routing domain is an overlap. seen holds
// the networks met so far, with their line numbers.
func classify(ctx context.Context, tx pgx.Tx, routingDomainID string, lines []Line, seen map[string]int) error {
	var check []*Line
	var cidrs []string
	for i := range lines {
		l := &lines[i]
		if l.Status != "valid" && l.Status != "needs_enrichment" {
			continue
		}
		if first, ok := seen[l.Parsed.CIDR]; ok {
			l.Status, l.Errors = "duplicate", []string{fmt.Sprintf("Same network as line %d.", first)}
			continue
		}
		seen[l.Parsed.CIDR] = l.Number
		check, cidrs = append(check, l), append(cidrs, l.Parsed.CIDR)
	}
	if len(check) == 0 {
		return nil
	}
	matches, err := inventoryMatches(ctx, tx, routingDomainID, cidrs)
	if err != nil {
		return err
	}
	for _, l := range check {
		m := matches[l.Parsed.CIDR]
		switch {
		case m.exact != nil:
			l.Status, l.Errors = "duplicate", []string{"Already in the inventory: " + *m.exact + "."}
		case m.overlap != nil:
			l.Status, l.Errors = "overlap", []string{"Overlaps " + *m.overlap + " in this routing domain."}
		}
	}
	return nil
}

type inventoryMatch struct{ exact, overlap *string }

// inventoryMatches looks up every network in one query.
func inventoryMatches(ctx context.Context, tx pgx.Tx, routingDomainID string, cidrs []string) (map[string]inventoryMatch, error) {
	rows, err := tx.Query(ctx, `SELECT t.c,
		(SELECT name || ' (' || cidr::text || ')' FROM subnets WHERE routing_domain_id = $1::uuid AND archived_at IS NULL AND cidr = t.c::cidr LIMIT 1),
		(SELECT name || ' (' || cidr::text || ')' FROM subnets WHERE routing_domain_id = $1::uuid AND archived_at IS NULL AND cidr && t.c::cidr LIMIT 1)
		FROM unnest($2::text[]) AS t(c)`, routingDomainID, cidrs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]inventoryMatch, len(cidrs))
	for rows.Next() {
		var cidr string
		var m inventoryMatch
		if err := rows.Scan(&cidr, &m.exact, &m.overlap); err != nil {
			return nil, err
		}
		out[cidr] = m
	}
	return out, rows.Err()
}
