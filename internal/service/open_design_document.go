package service

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

const MaxTeachingFileBytes = 3 * 1024 * 1024

var ErrTeachingFile = errors.New("Không đọc được tài liệu. Hỗ trợ PDF có lớp văn bản, DOCX, PPTX, TXT và Markdown; tối đa 3 MB, 100 trang và 60.000 ký tự. PDF scan cần OCR trước khi tải lên.")
var teachingSlidePath = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

func ExtractTeachingDocument(name string, raw []byte) (result *TeachingDocument, err error) {
	// Malformed PDF parsers may panic. Convert that to a user-facing file error.
	defer func() {
		if recover() != nil {
			result = nil
			err = ErrTeachingFile
		}
	}()
	if len(raw) == 0 || len(raw) > MaxTeachingFileBytes || len([]rune(name)) > 200 {
		return nil, ErrTeachingFile
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	extension := strings.ToLower(filepath.Ext(name))
	text := ""
	switch extension {
	case ".txt", ".md":
		if !utf8.Valid(raw) || bytes.ContainsRune(raw, 0) {
			return nil, ErrTeachingFile
		}
		text = string(raw)
	case ".pdf":
		if !bytes.HasPrefix(raw, []byte("%PDF-")) {
			return nil, ErrTeachingFile
		}
		reader, e := pdf.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if e != nil || reader.NumPage() > 100 {
			return nil, ErrTeachingFile
		}
		// Read page by page to bound accumulated text, not the entire document at once.
		var b strings.Builder
		for i := 1; i <= reader.NumPage(); i++ {
			page := reader.Page(i)
			pageText, e := page.GetPlainText(nil)
			if e != nil {
				return nil, ErrTeachingFile
			}
			b.WriteString(pageText)
			b.WriteByte('\n')
			if b.Len() > 300000 {
				return nil, ErrTeachingFile
			}
		}
		text = b.String()
	case ".docx", ".pptx":
		archive, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if e != nil || len(archive.File) > 3000 {
			return nil, ErrTeachingFile
		}
		files := []*zip.File{}
		for _, f := range archive.File {
			if (extension == ".docx" && f.Name == "word/document.xml") || (extension == ".pptx" && teachingSlidePath.MatchString(f.Name)) {
				files = append(files, f)
			}
		}
		if len(files) == 0 || len(files) > 100 {
			return nil, ErrTeachingFile
		}
		sort.Slice(files, func(i, j int) bool {
			a := teachingSlidePath.FindStringSubmatch(files[i].Name)
			b := teachingSlidePath.FindStringSubmatch(files[j].Name)
			if len(a) > 1 && len(b) > 1 {
				ai, _ := strconv.Atoi(a[1])
				bi, _ := strconv.Atoi(b[1])
				return ai < bi
			}
			return files[i].Name < files[j].Name
		})
		var b strings.Builder
		for _, f := range files {
			if f.UncompressedSize64 > 2*1024*1024 {
				return nil, ErrTeachingFile
			}
			stream, e := f.Open()
			if e != nil {
				return nil, ErrTeachingFile
			}
			fragment, e := readOfficeText(io.LimitReader(stream, 2*1024*1024+1))
			stream.Close()
			if e != nil {
				return nil, ErrTeachingFile
			}
			b.WriteString(fragment)
			b.WriteByte('\n')
			if b.Len() > 300000 {
				return nil, ErrTeachingFile
			}
		}
		text = b.String()
	default:
		return nil, ErrTeachingFile
	}
	text = strings.TrimSpace(text)
	if !boundedText(text, 1, 60000) {
		return nil, ErrTeachingFile
	}
	return &TeachingDocument{Name: name, Text: text}, nil
}
func readOfficeText(reader io.Reader) (string, error) {
	decoder := xml.NewDecoder(reader)
	var b strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				var text string
				if err := decoder.DecodeElement(&text, &t); err != nil {
					return "", err
				}
				b.WriteString(text)
				b.WriteByte(' ')
			}
		case xml.EndElement:
			if t.Name.Local == "p" {
				b.WriteByte('\n')
			}
		}
		if b.Len() > 300000 {
			return "", ErrTeachingFile
		}
	}
	return b.String(), nil
}
