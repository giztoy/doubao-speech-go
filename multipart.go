package doubaospeech

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"sort"
	"strings"
)

// MultipartFile is one file part in multipart/form-data payload.
type MultipartFile struct {
	FieldName   string
	FileName    string
	ContentType string
	Data        []byte
}

func buildMultipartBody(fields map[string]string, files []MultipartFile) (contentType string, body []byte, err error) {
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if err := writer.WriteField(k, fields[k]); err != nil {
			return "", nil, wrapError(err, "write multipart field")
		}
	}

	for _, file := range files {
		fieldName := strings.TrimSpace(file.FieldName)
		if fieldName == "" {
			return "", nil, fmt.Errorf("multipart file field name is empty")
		}

		fileName := strings.TrimSpace(file.FileName)
		if fileName == "" {
			fileName = "upload.bin"
		}

		contentType := strings.TrimSpace(file.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, fieldName, fileName))
		header.Set("Content-Type", contentType)

		part, createErr := writer.CreatePart(header)
		if createErr != nil {
			return "", nil, wrapError(createErr, "create multipart file part")
		}
		if _, writeErr := part.Write(file.Data); writeErr != nil {
			return "", nil, wrapError(writeErr, "write multipart file data")
		}
	}

	if err := writer.Close(); err != nil {
		return "", nil, wrapError(err, "close multipart writer")
	}

	return writer.FormDataContentType(), buf.Bytes(), nil
}
