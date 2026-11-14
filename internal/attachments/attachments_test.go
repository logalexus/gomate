package attachments

import (
	"bytes"
	"gomate/internal/model"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
)

func message(t *testing.T, data []byte) model.Packet {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("object", `{"name":"test"}`)
	part, e := w.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="../../file"`}})
	if e != nil {
		t.Fatal(e)
	}
	part.Write(data)
	w.Close()
	return model.Packet{Kind: "http", Content: append([]byte("POST /upload HTTP/1.1\r\nContent-Type: "+w.FormDataContentType()+"\r\n\r\n"), b.Bytes()...)}
}
func TestMultipartArbitraryFile(t *testing.T) {
	for _, data := range [][]byte{[]byte("PK\x03\x04opaque ZIP, no document parsing"), {0, 255, 1, 128}, []byte("<script>alert(1)</script>"), {}, []byte("Привет")} {
		p := message(t, data)
		before := bytes.Clone(p.Content)
		Enrich(&p)
		if len(p.Attachments) != 1 {
			t.Fatalf("files: %+v", p)
		}
		a := p.Attachments[0]
		if a.DownloadName != "file" || a.Field != "file" || !bytes.Equal(a.Content, data) {
			t.Fatalf("attachment: %+v", a)
		}
		if !bytes.Equal(before, p.Content) || !strings.Contains(p.Preview, `object: {"name":"test"}`) || strings.Contains(p.Preview, "PK") {
			t.Fatal("raw bytes changed or binary leaked into preview")
		}
	}
}
func TestRejectFragmentsAndLimits(t *testing.T) {
	for _, kind := range []string{"", "decode-error"} {
		p := message(t, []byte("test"))
		p.Kind = kind
		Enrich(&p)
		if len(p.Attachments) > 0 {
			t.Fatal("raw fragment accepted")
		}
	}
	p := message(t, []byte("test"))
	p.Incomplete = true
	Enrich(&p)
	if len(p.Attachments) > 0 {
		t.Fatal("incomplete")
	}
	p = message(t, []byte("test"))
	p.Content = p.Content[:len(p.Content)-10]
	Enrich(&p)
	if len(p.Attachments) > 0 {
		t.Fatal("truncated multipart")
	}
	p = message(t, bytes.Repeat([]byte("a"), maxBody))
	Enrich(&p)
	if len(p.Attachments) > 0 {
		t.Fatal("oversized body")
	}
}
func TestHTTPDownloadAndOrdinaryText(t *testing.T) {
	data := []byte("%PDF-1.5\nexample")
	p := model.Packet{Kind: "http", Content: append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\n\r\n"), data...)}
	Enrich(&p)
	if len(p.Attachments) != 1 || p.Attachments[0].DownloadName != "file" || !bytes.Equal(p.Attachments[0].Content, data) {
		t.Fatal("PDF not recognized")
	}
	p = model.Packet{Kind: "http", Content: []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{}")}
	Enrich(&p)
	if len(p.Attachments) != 0 {
		t.Fatal("JSON is not a file")
	}
	p = model.Packet{Kind: "http", Content: []byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=example.bin\r\n\r\n\x00\xff")}
	Enrich(&p)
	if len(p.Attachments) != 1 || p.Attachments[0].Name != "example.bin" {
		t.Fatal("arbitrary HTTP download")
	}
}

func TestMultipleFilesAndPartLimit(t *testing.T) {
	for _, count := range []int{2, 33} {
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		for i := 0; i < count; i++ {
			part, e := w.CreateFormFile("files", "same.bin")
			if e != nil {
				t.Fatal(e)
			}
			part.Write([]byte{byte(i), 0, 255})
		}
		w.Close()
		p := model.Packet{Kind: "http", Content: append([]byte("POST / HTTP/1.1\r\nContent-Type: "+w.FormDataContentType()+"\r\n\r\n"), b.Bytes()...)}
		Enrich(&p)
		if count > 32 {
			if len(p.Attachments) != 0 {
				t.Fatal("file limit")
			}
			continue
		}
		if len(p.Attachments) != count {
			t.Fatal("missing files")
		}
		for i, a := range p.Attachments {
			if !bytes.Equal(a.Content, []byte{byte(i), 0, 255}) {
				t.Fatal("files mixed")
			}
		}
	}
}
