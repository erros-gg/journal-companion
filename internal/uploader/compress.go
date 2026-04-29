package uploader

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

// FileInfo holds the compressed bytes and original-file metadata needed for
// the upload request headers.
type FileInfo struct {
	Compressed   []byte
	OriginalSize int64
	Hash         string // "sha256:<hex>" of the original (uncompressed) bytes
	Filename     string
}

// CompressFile reads the file at path, gzip-compresses it, and returns FileInfo.
// The SHA-256 hash is computed from the original bytes as they are compressed,
// so only one read pass is needed.
func CompressFile(path string) (FileInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileInfo{}, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return FileInfo{}, err
	}

	h := sha256.New()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)

	// Hash the original bytes while compressing them in a single read.
	if _, err := io.Copy(gz, io.TeeReader(f, h)); err != nil {
		return FileInfo{}, err
	}
	if err := gz.Close(); err != nil {
		return FileInfo{}, err
	}

	return FileInfo{
		Compressed:   buf.Bytes(),
		OriginalSize: stat.Size(),
		Hash:         fmt.Sprintf("sha256:%x", h.Sum(nil)),
		Filename:     stat.Name(),
	}, nil
}
