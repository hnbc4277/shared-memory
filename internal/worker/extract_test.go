package worker

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func docx(t *testing.T, xmlBody string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(xmlBody))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtract_Docx(t *testing.T) {
	body := `<?xml version="1.0"?><w:document xmlns:w="w"><w:body>` +
		`<w:p><w:r><w:t>部署说明</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>端口</w:t></w:r><w:r><w:tab/><w:t>18080</w:t></w:r></w:p></w:body></w:document>`
	got, err := NewExtractor("").Extract(context.Background(), mimeDocx, docx(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if got != "部署说明\n端口\t18080\n" {
		t.Fatalf("got %q", got)
	}
}

func TestExtract_ImageUsesTesseractOnlyWhenEnabled(t *testing.T) {
	var calls []string
	e := NewExtractor("chi_sim+eng")
	e.run = func(_ context.Context, name string, args []string, stdin []byte) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return []byte("截图里的文字"), nil
	}
	got, _ := e.Extract(context.Background(), "image/png", []byte("png"))
	if got != "截图里的文字" || len(calls) != 1 || !strings.Contains(calls[0], "-l chi_sim+eng") {
		t.Fatalf("got %q calls %v", got, calls)
	}

	off := NewExtractor("")
	off.run = e.run
	if got, _ := off.Extract(context.Background(), "image/png", []byte("png")); got != "" || len(calls) != 1 {
		t.Fatal("OCR must be skipped when no languages are configured")
	}
}

func TestExtract_MissingToolIsSkippedNotFatal(t *testing.T) {
	n := 0
	e := NewExtractor("eng")
	e.run = func(context.Context, string, []string, []byte) ([]byte, error) {
		n++
		return nil, &exec.Error{Name: "pdftotext", Err: exec.ErrNotFound}
	}
	for i := 0; i < 2; i++ {
		got, err := e.Extract(context.Background(), "application/pdf", []byte("%PDF"))
		if err != nil || got != "" {
			t.Fatalf("got %q err %v", got, err)
		}
	}
	if n != 1 {
		t.Fatalf("a missing tool must be probed once, got %d calls", n)
	}
}

func TestExtract_ToolFailureDoesNotFailJob(t *testing.T) {
	e := NewExtractor("eng")
	e.run = func(context.Context, string, []string, []byte) ([]byte, error) { return nil, errors.New("corrupt") }
	if got, err := e.Extract(context.Background(), "image/jpeg", []byte("x")); err != nil || got != "" {
		t.Fatalf("got %q err %v", got, err)
	}
}
