package media

import "testing"

func ftyp(major string, compat ...string) []byte {
	b := []byte{0, 0, 0, byte(16 + 4*len(compat))}
	b = append(b, "ftyp"+major+"\x00\x00\x02\x00"...)
	for _, c := range compat {
		b = append(b, c...)
	}
	return append(b, "\x00\x00\x00\x08free"...)
}

func TestSniff(t *testing.T) {
	cases := []struct {
		name   string
		b      []byte
		kind   string
		ctype  string
		accept bool
	}{
		{"mp4 isom", ftyp("isom", "isom", "iso2", "avc1", "mp41"), KindVideo, "video/mp4", true},
		{"mp4 mp42", ftyp("mp42", "mp42", "isom"), KindVideo, "video/mp4", true},
		{"quicktime", ftyp("qt  ", "qt  "), KindVideo, "video/quicktime", true},
		{"m4a", ftyp("M4A ", "M4A ", "mp42", "isom"), KindAudio, "audio/mp4", true},
		{"heic rejected", ftyp("heic", "mif1", "heic"), "", "", false},
		{"avif rejected", ftyp("avif", "mif1", "avif"), "", "", false},
		{"webm", []byte("\x1a\x45\xdf\xa3\x9f\x42\x86\x81\x01\x42\xf7\x81\x01\x42\x82\x84webm"), KindVideo, "video/webm", true},
		{"mkv", []byte("\x1a\x45\xdf\xa3\xa3\x42\x86\x81\x01\x42\x82\x88matroska"), KindVideo, "video/x-matroska", true},
		{"avi", []byte("RIFF\x00\x00\x00\x00AVI LIST"), KindVideo, "video/x-msvideo", true},
		{"wav", []byte("RIFF\x24\x08\x00\x00WAVEfmt "), KindAudio, "audio/wav", true},
		{"webp", []byte("RIFF\x24\x08\x00\x00WEBPVP8 "), KindImage, "image/webp", true},
		{"riff other", []byte("RIFF\x24\x08\x00\x00CDXA"), "", "", false},
		{"mp3 id3", []byte("ID3\x04\x00\x00\x00\x00\x00\x00"), KindAudio, "audio/mpeg", true},
		{"mp3 frame", []byte{0xFF, 0xFB, 0x90, 0x64}, KindAudio, "audio/mpeg", true},
		{"aac adts", []byte{0xFF, 0xF1, 0x50, 0x80}, KindAudio, "audio/aac", true},
		{"flac", []byte("fLaC\x00\x00\x00\x22"), KindAudio, "audio/flac", true},
		{"ogg", []byte("OggS\x00\x02"), KindAudio, "audio/ogg", true},
		{"png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), KindImage, "image/png", true},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F'}, KindImage, "image/jpeg", true},
		{"html", []byte("<!doctype html><script>alert(1)</script>"), "", "", false},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`), "", "", false},
		{"pdf", []byte("%PDF-1.7\n"), "", "", false},
		{"zip", []byte("PK\x03\x04"), "", "", false},
		{"exe", []byte("MZ\x90\x00"), "", "", false},
		{"empty", nil, "", "", false},
		{"short ftyp", []byte("\x00\x00\x00\x08ftyp"), "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, ok := sniff(tc.b)
			if ok != tc.accept || d.Kind != tc.kind || d.ContentType != tc.ctype {
				t.Errorf("sniff = %+v, %v; want %s %s %v", d, ok, tc.kind, tc.ctype, tc.accept)
			}
		})
	}
}
