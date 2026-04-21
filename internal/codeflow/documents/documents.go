package documents

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	fileloader "github.com/cloudwego/eino-ext/components/document/loader/file"
	einodoc "github.com/cloudwego/eino/components/document"

	cfconfig "github.com/viko0313/CodeFlow/internal/codeflow/config"
)

type UploadedDocument struct {
	ID        string    `json:"id"`
	FileName  string    `json:"file_name"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	Chunks    int       `json:"chunks"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	cfg cfconfig.DocumentsConfig
}

func NewStore(cfg cfconfig.DocumentsConfig) *Store {
	return &Store{cfg: cfg}
}

func (s *Store) Save(ctx context.Context, header *multipart.FileHeader) (*UploadedDocument, error) {
	if header == nil {
		return nil, fmt.Errorf("file is required")
	}
	if s.cfg.MaxUploadBytes > 0 && header.Size > s.cfg.MaxUploadBytes {
		return nil, fmt.Errorf("file exceeds max upload size")
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowed(ext, s.cfg.AllowedExtensions) {
		return nil, fmt.Errorf("file extension %q is not allowed", ext)
	}
	if err := os.MkdirAll(s.cfg.UploadDir, 0755); err != nil {
		return nil, err
	}
	src, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	id := newID(header.Filename)
	name := sanitizeFileName(header.Filename)
	target := filepath.Join(s.cfg.UploadDir, id+"_"+name)
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	size, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	content, chunks, err := s.loadContent(ctx, target)
	if err != nil {
		return nil, err
	}
	return &UploadedDocument{
		ID:        id,
		FileName:  name,
		Path:      target,
		Size:      size,
		Chunks:    chunks,
		Content:   truncate(content, 12000),
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (s *Store) loadContent(ctx context.Context, path string) (string, int, error) {
	loader, err := fileloader.NewFileLoader(ctx, &fileloader.FileLoaderConfig{UseNameAsID: true})
	if err != nil {
		return "", 0, err
	}
	docs, err := loader.Load(ctx, einodoc.Source{URI: path})
	if err != nil {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", 0, err
		}
		return string(data), 1, nil
	}
	var b strings.Builder
	for _, doc := range docs {
		if strings.TrimSpace(doc.Content) == "" {
			continue
		}
		b.WriteString(strings.TrimSpace(doc.Content))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String()), len(docs), nil
}

func allowed(ext string, values []string) bool {
	for _, value := range values {
		if strings.EqualFold(ext, strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func sanitizeFileName(name string) string {
	name = filepath.Base(name)
	replacer := strings.NewReplacer(" ", "_", "/", "_", "\\", "_", ":", "_")
	name = replacer.Replace(name)
	if name == "" || name == "." {
		return "document.txt"
	}
	return name
}

func newID(name string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s-%d", name, time.Now().UnixNano())))
	return "doc_" + hex.EncodeToString(sum[:])[:12]
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n\n...[document truncated]..."
}
