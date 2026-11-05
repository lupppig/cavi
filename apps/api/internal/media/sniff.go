package media

import (
	"bytes"
	"encoding/binary"
)

// Kinds of media CAVI accepts.
const (
	KindVideo = "video"
	KindAudio = "audio"
	KindImage = "image"
)

// sniffBytes is how much of a file sniff needs.
const sniffBytes = 4096

// detected is a recognised file format.
type detected struct {
	Kind        string
	ContentType string
}

// sniff identifies a file from its first bytes. File names and the
// browser's claimed type are never trusted. It recognises the formats CAVI
// accepts and returns ok=false for anything else.
func sniff(b []byte) (detected, bool) {
	switch {
	case len(b) >= 12 && string(b[4:8]) == "ftyp":
		return sniffISOBMFF(b)
	case bytes.HasPrefix(b, []byte{0x1A, 0x45, 0xDF, 0xA3}):
		// EBML: WebM or Matroska. The DocType appears early in the header.
		if bytes.Contains(b[:min(len(b), 64)], []byte("webm")) {
			return detected{KindVideo, "video/webm"}, true
		}
		return detected{KindVideo, "video/x-matroska"}, true
	case len(b) >= 12 && string(b[0:4]) == "RIFF":
		switch string(b[8:12]) {
		case "WAVE":
			return detected{KindAudio, "audio/wav"}, true
		case "AVI ":
			return detected{KindVideo, "video/x-msvideo"}, true
		case "WEBP":
			return detected{KindImage, "image/webp"}, true
		}
	case bytes.HasPrefix(b, []byte("ID3")):
		return detected{KindAudio, "audio/mpeg"}, true
	case bytes.HasPrefix(b, []byte("fLaC")):
		return detected{KindAudio, "audio/flac"}, true
	case bytes.HasPrefix(b, []byte("OggS")):
		return detected{KindAudio, "audio/ogg"}, true
	case len(b) >= 2 && b[0] == 0xFF && b[1]&0xF6 == 0xF0:
		// ADTS AAC: sync word 0xFFF, layer 00.
		return detected{KindAudio, "audio/aac"}, true
	case len(b) >= 2 && b[0] == 0xFF && b[1]&0xE0 == 0xE0 && b[1]&0x06 != 0:
		// MPEG audio frame sync with a valid layer (MP3 without ID3).
		return detected{KindAudio, "audio/mpeg"}, true
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return detected{KindImage, "image/png"}, true
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return detected{KindImage, "image/jpeg"}, true
	}
	return detected{}, false
}

// sniffISOBMFF classifies MP4-family files by their major and compatible
// brands: QuickTime, M4A audio, and MP4 video. Still-image formats in the
// same container (HEIC, AVIF) are not accepted yet.
func sniffISOBMFF(b []byte) (detected, bool) {
	size := int(binary.BigEndian.Uint32(b[0:4]))
	if size < 16 || size > len(b) {
		size = min(len(b), 64)
	}
	brands := []string{string(b[8:12])}
	for i := 16; i+4 <= size; i += 4 {
		brands = append(brands, string(b[i:i+4]))
	}
	switch major := brands[0]; major {
	case "qt  ":
		return detected{KindVideo, "video/quicktime"}, true
	case "M4A ", "M4B ":
		return detected{KindAudio, "audio/mp4"}, true
	case "heic", "heix", "hevc", "mif1", "msf1", "avif", "avis":
		return detected{}, false
	}
	return detected{KindVideo, "video/mp4"}, true
}
