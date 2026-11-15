// Package attachments derives bounded, read-only file previews from decoded HTTP
// messages. It never writes files, opens documents, or changes captured bytes.
package attachments

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"gomate/internal/model"
)

const maxBody = 256 << 10
const maxFiles = 32

// Enrich runs only when a page is read, including messages captured before this
// feature existed. Raw TCP fragments are deliberately not treated as whole files.
func Enrich(p *model.Packet) {
	if p.Kind != "http" || p.Incomplete || len(p.Content) > maxBody {
		return
	}
	head, body, ok := bytes.Cut(p.Content, []byte("\r\n\r\n"))
	if !ok {
		return
	}
	_, headers, ok := bytes.Cut(head, []byte("\r\n"))
	if !ok {
		return
	}
	h, err := textproto.NewReader(bufio.NewReader(bytes.NewReader(append(append([]byte{}, headers...), '\r', '\n', '\r', '\n')))).ReadMIMEHeader()
	if err != nil {
		return
	}
	typ, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil {
		typ = ""
	}
	var files []model.Attachment
	var summary strings.Builder
	if typ == "multipart/form-data" {
		if params["boundary"] == "" {
			return
		}
		r := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for n := 0; ; n++ {
			part, e := r.NextRawPart()
			if e == io.EOF {
				break
			}
			if e != nil || n >= 64 {
				return
			}
			data, e := io.ReadAll(io.LimitReader(part, maxBody+1))
			if e != nil || len(data) > maxBody {
				return
			}
			_, disposition, e := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			if e != nil {
				return
			}
			name, isFile := disposition["filename"]
			if isFile {
				if len(files) >= maxFiles {
					return
				}
				a := file(name, disposition["name"], part.Header.Get("Content-Type"), data)
				files = append(files, a)
				fmt.Fprintf(&summary, "%s: [file %s · %s · %d B]\n", a.Field, a.Name, a.Type, len(data))
			} else {
				value := "[binary field]"
				if utf8.Valid(data) {
					value = string(data)
				}
				fmt.Fprintf(&summary, "%s: %s\n", disposition["name"], value)
			}
		}
	} else {
		disposition, disp, _ := mime.ParseMediaType(h.Get("Content-Disposition"))
		detected := http.DetectContentType(body)
		if disposition != "attachment" && disp["filename"] == "" && typ != "application/octet-stream" && !strings.HasPrefix(detected, "image/") && detected != "application/pdf" && detected != "application/zip" {
			return
		}
		files = append(files, file(disp["filename"], "", typ, body))
		fmt.Fprintf(&summary, "[file %s · %s · %d B]\n", files[0].Name, files[0].Type, len(body))
	}
	if len(files) == 0 {
		return
	}
	p.Attachments = files
	p.Preview = string(head) + "\r\n\r\n" + summary.String()
}

func file(name, field, declared string, data []byte) model.Attachment {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	if len(name) > 180 {
		name = "file"
	}
	typ := http.DetectContentType(data)
	a := model.Attachment{Name: name, Field: field, Type: typ, DeclaredType: declared, Size: len(data), Content: data}
	a.DownloadName = name
	return a
}
