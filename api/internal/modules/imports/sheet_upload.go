package imports

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// templateBytes reads the upload. A workbook is binary, so it arrives as base64.
func templateBytes(req uploadRequest) ([]byte, error) {
	if !strings.HasSuffix(strings.ToLower(req.FileName), ".xlsx") {
		return []byte(req.Content), nil
	}
	data, err := base64.StdEncoding.DecodeString(req.Content)
	if err != nil {
		return nil, httpx.Validation("content", "This file didn't upload correctly. Try again.")
	}
	return data, nil
}

func toLines(checked []*rowCheck) []Line {
	lines := make([]Line, len(checked))
	for i, r := range checked {
		s := r.row.sheet
		lines[i] = Line{Number: r.row.line, Raw: r.row.raw, Status: r.status, Errors: r.errors, Parsed: Parsed{
			Type: s.Key, Sheet: s.Title, Row: r.row.number, Fields: r.row.fields, Name: r.row.fields[s.KeyCol]}}
	}
	return lines
}

func storeTemplate(ctx context.Context, tx pgx.Tx, actor string, req uploadRequest) (Batch, error) {
	data, err := templateBytes(req)
	if err != nil {
		return Batch{}, err
	}
	if len(data) > maxBytes {
		return Batch{}, httpx.Validation("content", "Upload a file of at most 5 MB.")
	}
	rows, err := readTemplate(req.FileName, data)
	if err != nil {
		return Batch{}, err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO import_batches (file_name, file_hash, format, status, uploaded_by)
		VALUES ($1, $2, $3, 'validated', $4::uuid) RETURNING id::text`, req.FileName, hash, templateFormat, actor).Scan(&id); err != nil {
		return Batch{}, err
	}
	checked, err := checkTemplate(ctx, tx, rows)
	if err != nil {
		return Batch{}, err
	}
	if err := insertRecords(ctx, tx, id, toLines(checked)); err != nil {
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
		New: map[string]any{"fileName": req.FileName, "fileHash": hash, "format": templateFormat, "summary": out.Summary}})
}

// discard closes a batch that won't be approved. Its rows stay on record.
func (h *Handler) discard(c *gin.Context) {
	h.changeBatch(c, func(ctx context.Context, tx pgx.Tx, actor, id string) (Batch, error) {
		b, err := loadBatch(ctx, tx, id, true)
		if err != nil {
			return b, err
		}
		if b.Status != "validated" {
			return b, httpx.NewError(http.StatusConflict, "IMPORT_LOCKED", "Only batches waiting for approval can be discarded.")
		}
		if _, err := tx.Exec(ctx, `UPDATE import_batches SET status = 'discarded' WHERE id = $1::uuid`, id); err != nil {
			return b, err
		}
		out, err := loadBatch(ctx, tx, id, false)
		if err != nil {
			return out, err
		}
		return out, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "import.discard", ObjectType: "import_batch", ObjectID: id})
	})
}
