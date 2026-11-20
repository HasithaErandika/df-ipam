package links

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var (
	errNoStore     = httpx.NewError(http.StatusServiceUnavailable, "ATTACHMENTS_UNAVAILABLE", "Attachments aren't available: the server has no attachment storage.")
	fileCategories = []string{"contract", "quotation", "diagram", "configuration", "other"}
)

// fileNameOf keeps a readable, harmless name: no path, no control characters.
func fileNameOf(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`"/\<>|:*?`, r) {
			return '_'
		}
		return r
	}, name)
	if r := []rune(name); len(r) > 200 {
		name = string(r[len(r)-200:])
	}
	return name
}

// upload adds one file to a link. Contracts and quotations need commercial access.
func (h *Handler) upload(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if h.files == nil {
		httpx.Fail(c, errNoStore)
		return
	}
	category := c.PostForm("category")
	if !slices.Contains(fileCategories, category) {
		httpx.Fail(c, httpx.Validation("category", "Choose what kind of document this is."))
		return
	}
	if slices.Contains(commercialFiles, category) && !can(c, auth.LinksCommercialManage) {
		httpx.Fail(c, httpx.ErrForbidden)
		return
	}
	header, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, httpx.Validation("file", "Choose a file of up to 15 MB."))
		return
	}
	f, err := header.Open()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer f.Close()
	name := fileNameOf(header.Filename)
	saved, err := h.files.Save(name, f)
	if err != nil {
		httpx.Fail(c, fileError(err))
		return
	}
	var fileID string
	err = db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if err := tx.QueryRow(c, `INSERT INTO network_link_attachments (link_id, category, file_name, content_type, size_bytes, sha256, stored_name, uploaded_by)
			SELECT id, $2, $3, $4, $5, $6, $7, $8::uuid FROM network_links WHERE id = $1::uuid AND archived_at IS NULL RETURNING id::text`,
			id, category, name, saved.ContentType, saved.Size, saved.SHA256, saved.Name, actor(c)).Scan(&fileID); err != nil {
			return err
		}
		return record(c, tx, "attach", "network_link", id, nil, map[string]any{"attachment": name, "category": category, "size": saved.Size, "sha256": saved.SHA256})
	})
	if err != nil {
		h.files.Remove(saved.Name)
		httpx.Fail(c, db.Friendly(err))
		return
	}
	httpx.OK(c, http.StatusCreated, gin.H{"id": fileID})
}

func fileError(err error) error {
	switch {
	case errors.Is(err, errFileType):
		return httpx.Validation("file", "Use a PDF, PNG, JPEG, Word, Excel, PowerPoint, Visio or text file whose content matches its name.")
	case errors.Is(err, errFileSize):
		return httpx.Validation("file", "Files can be at most 15 MB.")
	case errors.Is(err, errFileEmpty):
		return httpx.Validation("file", "This file is empty.")
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return httpx.Validation("file", "Files can be at most 15 MB.")
	}
	return err
}

// attachmentFor finds a file that belongs to the link and may be opened by this person.
func (h *Handler) attachmentFor(c *gin.Context) (id, fileID, stored, name, kind string, ok bool) {
	id, ok = httpx.PathID(c)
	if !ok {
		return
	}
	fileID = c.Param("file")
	if !httpx.IsUUID(fileID) {
		httpx.Fail(c, httpx.ErrNotFound)
		return id, fileID, "", "", "", false
	}
	var category string
	err := h.conn.QueryRow(c, `SELECT stored_name, file_name, content_type, category FROM network_link_attachments
		WHERE id = $1::uuid AND link_id = $2::uuid AND removed_at IS NULL`, fileID, id).Scan(&stored, &name, &kind, &category)
	if err == nil && slices.Contains(commercialFiles, category) && !can(c, auth.LinksCommercialView) {
		err = httpx.ErrForbidden
	}
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return id, fileID, "", "", "", false
	}
	return id, fileID, stored, name, kind, true
}

func (h *Handler) downloadFile(c *gin.Context) {
	if h.files == nil {
		httpx.Fail(c, errNoStore)
		return
	}
	_, _, stored, name, kind, ok := h.attachmentFor(c)
	if !ok {
		return
	}
	f, err := h.files.Open(stored)
	if err != nil {
		httpx.Fail(c, httpx.NewError(http.StatusGone, "FILE_MISSING", "This file is missing from the server's attachment storage."))
		return
	}
	defer f.Close()
	c.Header("Content-Type", kind)
	c.Header("Content-Disposition", `attachment; filename="`+asciiName(name)+`"; filename*=UTF-8''`+url.PathEscape(name))
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, f)
}

func asciiName(name string) string {
	return strings.Map(func(r rune) rune {
		if r > 126 || r < 32 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
}

// removeFile takes a file off the link. The record and the file stay, so the
// history keeps what was attached.
func (h *Handler) removeFile(c *gin.Context) {
	id, fileID, _, name, _, ok := h.attachmentFor(c)
	if !ok {
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var category string
		if err := tx.QueryRow(c, `UPDATE network_link_attachments SET removed_at = now(), removed_by = $2::uuid
			WHERE id = $1::uuid AND removed_at IS NULL RETURNING category`, fileID, actor(c)).Scan(&category); err != nil {
			return err
		}
		if slices.Contains(commercialFiles, category) && !can(c, auth.LinksCommercialManage) {
			return httpx.ErrForbidden
		}
		return record(c, tx, "detach", "network_link", id, map[string]any{"attachment": name, "category": category}, nil)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	c.Status(http.StatusNoContent)
}
