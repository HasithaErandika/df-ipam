package links

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MaxFileBytes caps one attachment.
const MaxFileBytes = 15 << 20

// fileKinds are the attachments people may add: the extension, what the
// content must look like, and the type it is served as.
var fileKinds = map[string]struct{ sniff, serve string }{
	".pdf":  {"application/pdf", "application/pdf"},
	".png":  {"image/png", "image/png"},
	".jpg":  {"image/jpeg", "image/jpeg"},
	".jpeg": {"image/jpeg", "image/jpeg"},
	".docx": {"application/zip", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	".xlsx": {"application/zip", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	".pptx": {"application/zip", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
	".vsdx": {"application/zip", "application/vnd.ms-visio.drawing"},
	".txt":  {"text/plain", "text/plain; charset=utf-8"},
	".csv":  {"text/plain", "text/csv; charset=utf-8"},
	".cfg":  {"text/plain", "text/plain; charset=utf-8"},
	".conf": {"text/plain", "text/plain; charset=utf-8"},
}

// Store keeps attachments as files named by a random id in one private
// directory (a Docker volume), outside anything the web server serves.
type Store struct{ root *os.Root }

// OpenStore opens or creates the directory. Files are readable by this user only.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("attachment directory: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("attachment directory: %w", err)
	}
	return &Store{root: root}, nil
}

type savedFile struct {
	Name, SHA256, ContentType string
	Size                      int64
}

var (
	errFileType  = errors.New("type")
	errFileSize  = errors.New("size")
	errFileEmpty = errors.New("empty")
)

// Save checks the content against the file's extension and writes it.
func (s *Store) Save(fileName string, r io.Reader) (savedFile, error) {
	kind, ok := fileKinds[strings.ToLower(filepath.Ext(fileName))]
	if !ok {
		return savedFile{}, errFileType
	}
	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return savedFile{}, err
	}
	if n == 0 {
		return savedFile{}, errFileEmpty
	}
	if !strings.HasPrefix(http.DetectContentType(head[:n]), kind.sniff) {
		return savedFile{}, errFileType
	}
	name, err := randomID()
	if err != nil {
		return savedFile{}, err
	}
	f, err := s.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return savedFile{}, err
	}
	sum := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, sum), io.LimitReader(io.MultiReader(bytes.NewReader(head[:n]), r), MaxFileBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && size > MaxFileBytes {
		err = errFileSize
	}
	if err != nil {
		_ = s.root.Remove(name)
		return savedFile{}, err
	}
	return savedFile{Name: name, SHA256: hex.EncodeToString(sum.Sum(nil)), ContentType: kind.serve, Size: size}, nil
}

// Open reads a stored file by its id.
func (s *Store) Open(name string) (*os.File, error) { return s.root.Open(name) }

// Remove deletes a stored file, used only to undo a save whose record failed.
func (s *Store) Remove(name string) { _ = s.root.Remove(name) }

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6], b[8] = (b[6]&0x0f)|0x40, (b[8]&0x3f)|0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}
