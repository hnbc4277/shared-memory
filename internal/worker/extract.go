package worker

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"sharedmemory/internal/attachment"
)

const (
	extractTimeout = 60 * time.Second
	mimeDocx       = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)

// runFunc executes an external tool with data on stdin and returns stdout.
type runFunc func(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)

func execRun(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && stderr.Len() > 0 {
		return nil, errors.New(strings.TrimSpace(stderr.String()))
	}
	return out, err
}

// DocExtractor turns attachments into searchable text: text as-is, docx in
// pure Go, PDF via pdftotext and images via tesseract OCR. A missing tool
// disables that format instead of failing the job.
type DocExtractor struct {
	OCRLangs string
	run      runFunc
	missing  sync.Map
}

func NewExtractor(ocrLangs string) *DocExtractor {
	return &DocExtractor{OCRLangs: ocrLangs, run: execRun}
}

func (e *DocExtractor) Extract(ctx context.Context, mimeType string, data []byte) (string, error) {
	switch {
	case attachment.IsText(mimeType):
		if !utf8.Valid(data) {
			return "", nil
		}
		return string(data), nil
	case mimeType == mimeDocx:
		return docxText(data)
	case mimeType == "application/pdf":
		return e.tool(ctx, "pdftotext", []string{"-q", "-layout", "-", "-"}, data)
	case strings.HasPrefix(mimeType, "image/"):
		if e.OCRLangs == "" {
			return "", nil
		}
		return e.tool(ctx, "tesseract", []string{"stdin", "stdout", "-l", e.OCRLangs}, data)
	}
	return "", nil
}

func (e *DocExtractor) tool(ctx context.Context, name string, args []string, data []byte) (string, error) {
	if _, gone := e.missing.Load(name); gone {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, extractTimeout)
	defer cancel()
	out, err := e.run(ctx, name, args, data)
	if errors.Is(err, exec.ErrNotFound) {
		e.missing.Store(name, true)
		log.Printf("%s not installed; skipping that attachment type", name)
		return "", nil
	}
	if err != nil {
		// A corrupt or unsupported file is not worth retrying the job for.
		log.Printf("%s: %v", name, err)
		return "", nil
	}
	if !utf8.Valid(out) {
		out = bytes.ToValidUTF8(out, nil)
	}
	return string(out), nil
}

// docxText concatenates the paragraphs of word/document.xml.
func docxText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", nil
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", nil
		}
		defer rc.Close()
		var b strings.Builder
		dec := xml.NewDecoder(io.LimitReader(rc, attachment.MaxBytes))
		inText := false
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				inText = t.Name.Local == "t"
				if t.Name.Local == "tab" {
					b.WriteByte('\t')
				}
			case xml.EndElement:
				switch t.Name.Local {
				case "t":
					inText = false
				case "p":
					b.WriteByte('\n')
				}
			case xml.CharData:
				if inText {
					b.Write(t)
				}
			}
		}
		return b.String(), nil
	}
	return "", nil
}
