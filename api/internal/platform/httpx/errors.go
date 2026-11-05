// Package httpx holds the shared HTTP response format and middleware.
package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"
)

type FieldIssue struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// Error is a failure that is safe to show to the caller.
type Error struct {
	Status  int
	Code    string
	Message string
	Details []FieldIssue
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func Validation(field, issue string) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "VALIDATION_FAILED",
		Message: "The request has invalid fields.", Details: []FieldIssue{{field, issue}}}
}

var (
	ErrUnauthenticated = NewError(http.StatusUnauthorized, "UNAUTHENTICATED", "Sign in to continue.")
	ErrForbidden       = NewError(http.StatusForbidden, "FORBIDDEN", "You don't have permission to do this.")
	ErrNotFound        = NewError(http.StatusNotFound, "NOT_FOUND", "The record was not found.")
	ErrBadRequest      = NewError(http.StatusBadRequest, "BAD_REQUEST", "The request could not be read.")
)

// Fail writes err in the shared format. Unknown errors are logged and hidden.
func Fail(c *gin.Context, err error) {
	var e *Error
	if !errors.As(err, &e) {
		slog.ErrorContext(c.Request.Context(), "request failed", "error", err,
			"correlationId", c.GetString(correlationKey))
		e = NewError(http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong.")
	}
	body := gin.H{"code": e.Code, "message": e.Message, "correlationId": c.GetString(correlationKey)}
	if len(e.Details) > 0 {
		body["details"] = e.Details
	}
	c.AbortWithStatusJSON(e.Status, gin.H{"error": body})
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsUUID reports whether s is a well-formed UUID.
func IsUUID(s string) bool { return uuidPattern.MatchString(s) }

// PathID reads the :id parameter. A malformed ID is simply not found.
func PathID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if !IsUUID(id) {
		Fail(c, ErrNotFound)
		return "", false
	}
	return id, true
}

// Page is the parsed pagination for a list request.
type Page struct{ Number, Size int }

func (p Page) Offset() int { return (p.Number - 1) * p.Size }

const (
	DefaultPageSize = 50
	MaxPageSize     = 200
	maxPage         = 1_000_000 // keeps the offset far from overflowing
)

// ParsePage reads pagination. Missing or invalid values fall back to the defaults,
// and pageSize is capped so a client can't request an unbounded result.
func ParsePage(c *gin.Context) Page {
	n, _ := strconv.Atoi(c.Query("page"))
	s, _ := strconv.Atoi(c.Query("pageSize"))
	if n < 1 {
		n = 1
	}
	if n > maxPage {
		n = maxPage
	}
	if s < 1 {
		s = DefaultPageSize
	}
	if s > MaxPageSize {
		s = MaxPageSize
	}
	return Page{n, s}
}

// PageMeta describes one page of a list.
type PageMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"pageSize"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"totalPages"`
	HasNext    bool  `json:"hasNext"`
	HasPrev    bool  `json:"hasPrev"`
}

// Meta builds the metadata. A page past the end stays as requested, and clients
// use TotalPages to move back to the last real page.
func (p Page) Meta(total int64) PageMeta {
	pages := 0
	if p.Size > 0 {
		pages = int((total + int64(p.Size) - 1) / int64(p.Size))
	}
	return PageMeta{p.Number, p.Size, total, pages, p.Number < pages, p.Number > 1 && pages > 0}
}

func List[T any](c *gin.Context, items []T, p Page, total int64) {
	if items == nil {
		items = []T{}
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "meta": p.Meta(total)})
}

func OK(c *gin.Context, status int, v any) { c.JSON(status, gin.H{"data": v}) }
