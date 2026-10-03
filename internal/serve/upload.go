package serve

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// uploadMaxBytes bounds one shared file: an unbounded body would let any
	// peer holding the token fill this machine's disk.
	uploadMaxBytes = 32 << 20
	// uploadMemoryBytes is the in-heap part of a multipart body; the rest spills
	// to os.TempDir so a photo does not double the process's memory.
	uploadMemoryBytes = 4 << 20
	// uploadNameBytes caps one stored filename, leaving room for the prefix and
	// the filesystem's 255-byte component limit.
	uploadNameBytes = 96
)

type uploadedFile struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

type uploadResult struct {
	Dir   string         `json:"dir"`
	Files []uploadedFile `json:"files"`
}

// upload stores the files a remote client shares into
// <workspace>/.reasonix/uploads and answers with host-side absolute paths. The
// workspace is the only useful destination: the model's own tools read from
// there, so a phone-local path would name nothing this machine can open.
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(s.ctl().WorkspaceRoot())
	if root == "" {
		http.Error(w, "the current session has no workspace root to store uploads in", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, uploadMaxBytes)
	if err := r.ParseMultipartForm(uploadMemoryBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, fmt.Sprintf("upload exceeds %d bytes", uploadMaxBytes), http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "expected multipart/form-data: "+err.Error(), http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}
	dir := filepath.Join(root, ".reasonix", "uploads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, "create upload directory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := ignoreUploads(dir); err != nil {
		http.Error(w, "prepare upload directory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	files, err := saveUploads(dir, r.MultipartForm.File)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(files) == 0 {
		http.Error(w, "no file in the request", http.StatusBadRequest)
		return
	}
	writeJSON(w, uploadResult{Dir: dir, Files: files})
}

// ignoreUploads keeps shared files out of the user's git status. The repository
// being talked about is the user's, so its ignore rules know nothing about
// uploads/; "*" also hides this marker, so the tree stays clean either way.
func ignoreUploads(dir string) error {
	marker := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	return os.WriteFile(marker, []byte("*\n"), 0o644)
}

// saveUploads walks the parts in a stable order: a map range would store a
// multi-file share in whatever order the runtime felt like.
func saveUploads(dir string, headers map[string][]*multipart.FileHeader) ([]uploadedFile, error) {
	out := []uploadedFile{}
	for _, key := range slices.Sorted(maps.Keys(headers)) {
		for _, header := range headers[key] {
			name, err := uploadName(header.Filename)
			if err != nil {
				return nil, err
			}
			stored, err := storeUpload(dir, name, header)
			if err != nil {
				return nil, err
			}
			out = append(out, stored)
		}
	}
	return out, nil
}

func storeUpload(dir, name string, header *multipart.FileHeader) (uploadedFile, error) {
	src, err := header.Open()
	if err != nil {
		return uploadedFile{}, fmt.Errorf("read %s: %w", name, err)
	}
	defer src.Close()
	target, err := createUpload(dir, name)
	if err != nil {
		return uploadedFile{}, err
	}
	written, err := io.Copy(target, src)
	if closeErr := target.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(target.Name())
		return uploadedFile{}, fmt.Errorf("store %s: %w", name, err)
	}
	return uploadedFile{Name: filepath.Base(target.Name()), Path: target.Name(), Bytes: written}, nil
}

// createUpload opens <dir>/<stamp>-<name> exclusively, so a second share of the
// same filename never replaces the first.
func createUpload(dir, name string) (*os.File, error) {
	stamp := time.Now().Format("20060102-150405")
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for attempt := range 1000 {
		label := stamp
		if attempt > 0 {
			label = fmt.Sprintf("%s-%d", stamp, attempt)
		}
		file, err := os.OpenFile(filepath.Join(dir, label+"-"+stem+ext), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return file, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("store %s: %w", name, err)
		}
	}
	return nil, fmt.Errorf("store %s: too many uploads share this name", name)
}

// uploadName reduces a client-supplied filename to one safe path component:
// uploads land inside the user's project, so separators, control characters,
// and a Windows-trailing dot must not survive the trip.
func uploadName(raw string) (string, error) {
	name := path.Base(strings.ReplaceAll(strings.TrimSpace(raw), `\`, "/"))
	name = strings.TrimRight(strings.TrimSpace(name), ".")
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "" {
		return "", errors.New("upload has no usable filename")
	}
	return clampFilename(name, uploadNameBytes), nil
}

func clampFilename(name string, max int) string {
	if len(name) <= max {
		return name
	}
	ext := path.Ext(name)
	if len(ext) > 16 {
		ext = ""
	}
	keep := max - len(ext)
	if keep < 1 {
		keep = max
	}
	cut := name[:keep]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	if cut == "" {
		cut = "upload"
	}
	return cut + ext
}
