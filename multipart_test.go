package doubaospeech

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"
)

func TestBuildMultipartBody(t *testing.T) {
	contentType, payload, err := buildMultipartBody(
		map[string]string{
			"appid":      "app-test",
			"speaker_id": "speaker-1",
		},
		[]MultipartFile{{
			FieldName:   "audio",
			FileName:    "sample.wav",
			ContentType: "audio/wav",
			Data:        []byte("audio-bytes"),
		}},
	)
	if err != nil {
		t.Fatalf("buildMultipartBody error = %v", err)
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("ParseMediaType error = %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("media type = %q, want %q", mediaType, "multipart/form-data")
	}

	reader := multipart.NewReader(bytes.NewReader(payload), params["boundary"])
	fields := make(map[string]string)
	files := make(map[string][]byte)

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart error = %v", err)
		}

		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("ReadAll part error = %v", err)
		}

		if part.FileName() != "" {
			files[part.FormName()] = data
			continue
		}

		fields[part.FormName()] = string(data)
	}

	if fields["appid"] != "app-test" {
		t.Fatalf("appid = %q, want %q", fields["appid"], "app-test")
	}
	if fields["speaker_id"] != "speaker-1" {
		t.Fatalf("speaker_id = %q, want %q", fields["speaker_id"], "speaker-1")
	}

	if got := files["audio"]; !bytes.Equal(got, []byte("audio-bytes")) {
		t.Fatalf("audio bytes = %q, want %q", string(got), "audio-bytes")
	}
}

func TestBuildMultipartBodyRejectsEmptyFileFieldName(t *testing.T) {
	_, _, err := buildMultipartBody(nil, []MultipartFile{{Data: []byte("x")}})
	if err == nil {
		t.Fatalf("expected error for empty multipart file field name")
	}
	if !strings.Contains(err.Error(), "field name") {
		t.Fatalf("unexpected error = %v", err)
	}
}
