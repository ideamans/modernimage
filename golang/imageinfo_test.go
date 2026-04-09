package modernimage

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

// imageDimensions returns (width, height) for JPEG, PNG, GIF, WebP, or AVIF
// data by parsing the container's headers. No external dependencies.
func imageDimensions(data []byte) (int, int, error) {
	if len(data) >= 4 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return jpegDimensions(data)
	}
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		return pngDimensions(data)
	}
	if len(data) >= 6 && data[0] == 'G' && data[1] == 'I' && data[2] == 'F' {
		return gifDimensions(data)
	}
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return webpDimensions(data)
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		return avifDimensions(data)
	}
	return 0, 0, fmt.Errorf("unknown image format")
}

// jpegDimensions parses JPEG SOFn marker and returns width/height.
func jpegDimensions(data []byte) (int, int, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0, fmt.Errorf("not a JPEG")
	}
	pos := 2
	for pos+4 <= len(data) {
		// Skip filler 0xFF bytes
		for pos < len(data) && data[pos] == 0xFF {
			pos++
		}
		if pos >= len(data) {
			break
		}
		marker := data[pos]
		pos++

		// Markers without payload
		if marker == 0xD8 || marker == 0xD9 {
			continue
		}
		if marker >= 0xD0 && marker <= 0xD7 {
			continue
		}
		if pos+2 > len(data) {
			break
		}
		segLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if segLen < 2 || pos+segLen > len(data) {
			return 0, 0, fmt.Errorf("invalid JPEG segment length")
		}

		// SOFn markers (Start of Frame): C0..C3, C5..C7, C9..CB, CD..CF
		isSOF := (marker >= 0xC0 && marker <= 0xC3) ||
			(marker >= 0xC5 && marker <= 0xC7) ||
			(marker >= 0xC9 && marker <= 0xCB) ||
			(marker >= 0xCD && marker <= 0xCF)
		if isSOF {
			if segLen < 7 {
				return 0, 0, fmt.Errorf("SOF too small")
			}
			// segment: LL LL P HH HH WW WW ...
			h := int(binary.BigEndian.Uint16(data[pos+3 : pos+5]))
			w := int(binary.BigEndian.Uint16(data[pos+5 : pos+7]))
			return w, h, nil
		}
		pos += segLen
	}
	return 0, 0, fmt.Errorf("no SOF marker found")
}

// pngDimensions parses PNG IHDR and returns width/height.
func pngDimensions(data []byte) (int, int, error) {
	// 8-byte signature + IHDR chunk: [4 byte len][IHDR][4 byte width][4 byte height]...
	if len(data) < 24 {
		return 0, 0, fmt.Errorf("PNG too small")
	}
	if string(data[12:16]) != "IHDR" {
		return 0, 0, fmt.Errorf("missing IHDR")
	}
	w := int(binary.BigEndian.Uint32(data[16:20]))
	h := int(binary.BigEndian.Uint32(data[20:24]))
	return w, h, nil
}

// gifDimensions reads the GIF logical screen descriptor.
func gifDimensions(data []byte) (int, int, error) {
	if len(data) < 10 {
		return 0, 0, fmt.Errorf("GIF too small")
	}
	w := int(binary.LittleEndian.Uint16(data[6:8]))
	h := int(binary.LittleEndian.Uint16(data[8:10]))
	return w, h, nil
}

// webpDimensions reads the canvas dimensions from a WebP file.
// Supports VP8 (lossy), VP8L (lossless), and VP8X (extended).
func webpDimensions(data []byte) (int, int, error) {
	if len(data) < 30 {
		return 0, 0, fmt.Errorf("WebP too small")
	}
	chunk := string(data[12:16])
	switch chunk {
	case "VP8X":
		// VP8X chunk data starts at offset 20.
		// 1 byte flags + 3 bytes reserved + 3 bytes canvas_w-1 (LE) + 3 bytes canvas_h-1 (LE)
		w := int(data[24]) | int(data[25])<<8 | int(data[26])<<16
		h := int(data[27]) | int(data[28])<<8 | int(data[29])<<16
		return w + 1, h + 1, nil
	case "VP8L":
		// VP8L: chunk data offset 20.
		// byte 0: signature 0x2F
		// bytes 1..4: 14 bits width-1, 14 bits height-1, 1 bit alpha, 3 bits version
		if data[20] != 0x2F {
			return 0, 0, fmt.Errorf("VP8L bad signature")
		}
		b0 := uint32(data[21])
		b1 := uint32(data[22])
		b2 := uint32(data[23])
		b3 := uint32(data[24])
		w := int((b0 | (b1 << 8)) & 0x3FFF)
		h := int(((b1 >> 6) | (b2 << 2) | (b3 << 10)) & 0x3FFF)
		return w + 1, h + 1, nil
	case "VP8 ":
		// VP8 lossy: frame tag at offset 20 (3 bytes), then start code (3 bytes),
		// then 2 bytes width and 2 bytes height (low 14 bits each, LE).
		if len(data) < 30 {
			return 0, 0, fmt.Errorf("VP8 truncated")
		}
		// start code at offset 23: 0x9D 0x01 0x2A
		if data[23] != 0x9D || data[24] != 0x01 || data[25] != 0x2A {
			return 0, 0, fmt.Errorf("VP8 missing start code")
		}
		w := int(binary.LittleEndian.Uint16(data[26:28])) & 0x3FFF
		h := int(binary.LittleEndian.Uint16(data[28:30])) & 0x3FFF
		return w, h, nil
	}
	return 0, 0, fmt.Errorf("unknown WebP chunk: %q", chunk)
}

// avifDimensions parses the ISOBMFF box hierarchy to find the first ispe box.
// Recurses into meta → iprp → ipco containers.
func avifDimensions(data []byte) (int, int, error) {
	w, h, ok := findISPE(data)
	if !ok {
		return 0, 0, fmt.Errorf("ispe box not found")
	}
	return w, h, nil
}

// webpInfo summarises WebP container metadata for tests.
type webpInfo struct {
	isAnimated bool
	frameCount int // number of ANMF chunks
	hasAlpha   bool
	hasVP8L    bool // true if any VP8L chunk is present (lossless frame data)
	hasVP8     bool // true if any VP8 chunk is present (lossy frame data)
}

// parseWebP iterates RIFF chunks and extracts the animation/alpha flags
// and ANMF frame count.
func parseWebP(data []byte) (webpInfo, error) {
	info := webpInfo{}
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return info, fmt.Errorf("not WebP")
	}
	pos := 12
	for pos+8 <= len(data) {
		fourCC := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		contentStart := pos + 8
		contentEnd := contentStart + size
		if contentEnd > len(data) {
			return info, fmt.Errorf("chunk %q truncated", fourCC)
		}
		switch fourCC {
		case "VP8X":
			if size >= 1 {
				flags := data[contentStart]
				// VP8X flags: bit 1 = animation, bit 4 = alpha
				info.isAnimated = flags&0x02 != 0
				info.hasAlpha = flags&0x10 != 0
			}
		case "ANMF":
			info.frameCount++
		case "VP8L":
			info.hasVP8L = true
			// VP8L: alpha_is_used is bit 28 of bytes 1..4 LE,
			// i.e. bit 4 of byte 4 of chunk content.
			if size >= 5 {
				if (data[contentStart+4]>>4)&1 != 0 {
					info.hasAlpha = true
				}
			}
		case "VP8 ":
			info.hasVP8 = true
		case "ALPH":
			// Presence of an ALPH chunk implies alpha for VP8 + alpha.
			info.hasAlpha = true
		}
		pos = contentEnd
		if size&1 == 1 {
			pos++ // chunk size padded to even
		}
	}
	return info, nil
}

// --- ICC profile inject / extract helpers ---
//
// Note: injectJpegICC and extractJpegICC live in the production file
// jpegseg.go (they are needed by NormalizeJpegOrientation's fallback path).
// Only injectJpegICCMulti remains here as a test-only helper.

// injectJpegICCMulti inserts an ICC profile split across multiple APP2
// segments. Each segment carries a chunk of the profile and the standard
// `[seq] [total]` markers, mimicking how cameras and editors emit large
// ICC profiles. The encoder must reassemble these into a single profile.
func injectJpegICCMulti(jpeg, icc []byte, chunkSize int) ([]byte, error) {
	if len(jpeg) < 2 || jpeg[0] != 0xFF || jpeg[1] != 0xD8 {
		return nil, fmt.Errorf("not a JPEG")
	}
	if chunkSize <= 0 {
		return nil, fmt.Errorf("chunkSize must be positive")
	}
	totalChunks := (len(icc) + chunkSize - 1) / chunkSize
	if totalChunks > 255 {
		return nil, fmt.Errorf("too many chunks (max 255)")
	}

	var allSegs []byte
	for i := 0; i < totalChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(icc) {
			end = len(icc)
		}
		chunk := icc[start:end]
		const overhead = 2 + 12 + 2
		segLen := overhead + len(chunk)
		seg := make([]byte, 0, 2+segLen)
		seg = append(seg, 0xFF, 0xE2)
		seg = append(seg, byte(segLen>>8), byte(segLen&0xFF))
		seg = append(seg, []byte("ICC_PROFILE\x00")...)
		seg = append(seg, byte(i+1), byte(totalChunks)) // seq, total
		seg = append(seg, chunk...)
		allSegs = append(allSegs, seg...)
	}

	out := make([]byte, 0, len(jpeg)+len(allSegs))
	out = append(out, jpeg[:2]...)
	out = append(out, allSegs...)
	out = append(out, jpeg[2:]...)
	return out, nil
}

// (extractJpegICC is now in production code in jpegseg.go)

// injectPngICC inserts an iCCP chunk after IHDR. Existing iCCP/sRGB chunks
// are removed to avoid conflicts.
func injectPngICC(png, icc []byte) ([]byte, error) {
	if len(png) < 8 || png[0] != 0x89 || string(png[1:4]) != "PNG" {
		return nil, fmt.Errorf("not a PNG")
	}
	// Compress ICC with zlib
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(icc); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	// Build iCCP chunk: name "test" + \0 + compression byte (0x00) + zlib data
	chunkData := append([]byte("test\x00\x00"), compressed.Bytes()...)
	iccpChunk := buildPngChunk("iCCP", chunkData)

	// Walk source PNG, copy chunks, but drop any existing iCCP/sRGB.
	// Insert new iCCP right after IHDR.
	out := make([]byte, 0, len(png)+len(iccpChunk))
	out = append(out, png[:8]...) // signature

	pos := 8
	insertedAfterIHDR := false
	for pos+8 <= len(png) {
		chunkLen := int(binary.BigEndian.Uint32(png[pos : pos+4]))
		chunkType := string(png[pos+4 : pos+8])
		chunkEnd := pos + 8 + chunkLen + 4
		if chunkEnd > len(png) {
			return nil, fmt.Errorf("truncated PNG chunk")
		}
		if chunkType == "iCCP" || chunkType == "sRGB" {
			pos = chunkEnd
			continue
		}
		out = append(out, png[pos:chunkEnd]...)
		pos = chunkEnd
		if chunkType == "IHDR" && !insertedAfterIHDR {
			out = append(out, iccpChunk...)
			insertedAfterIHDR = true
		}
	}
	if !insertedAfterIHDR {
		return nil, fmt.Errorf("no IHDR found")
	}
	return out, nil
}

func buildPngChunk(chunkType string, data []byte) []byte {
	out := make([]byte, 0, 12+len(data))
	out = binary.BigEndian.AppendUint32(out, uint32(len(data)))
	out = append(out, []byte(chunkType)...)
	out = append(out, data...)
	crc := crc32.NewIEEE()
	crc.Write([]byte(chunkType))
	crc.Write(data)
	out = binary.BigEndian.AppendUint32(out, crc.Sum32())
	return out
}

// extractPngICC finds the iCCP chunk and returns the decompressed ICC bytes.
func extractPngICC(data []byte) ([]byte, error) {
	if len(data) < 8 || data[0] != 0x89 || string(data[1:4]) != "PNG" {
		return nil, fmt.Errorf("not a PNG")
	}
	pos := 8
	for pos+8 <= len(data) {
		chunkLen := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		chunkType := string(data[pos+4 : pos+8])
		if pos+8+chunkLen+4 > len(data) {
			return nil, fmt.Errorf("truncated chunk")
		}
		if chunkType == "iCCP" {
			content := data[pos+8 : pos+8+chunkLen]
			// content: name \0 compression_method [zlib data]
			nullIdx := bytes.IndexByte(content, 0x00)
			if nullIdx < 0 || nullIdx+2 >= len(content) {
				return nil, fmt.Errorf("malformed iCCP")
			}
			zlibData := content[nullIdx+2:]
			zr, err := zlib.NewReader(bytes.NewReader(zlibData))
			if err != nil {
				return nil, err
			}
			defer zr.Close()
			return io.ReadAll(zr)
		}
		pos += 8 + chunkLen + 4
	}
	return nil, nil
}

// extractWebpICC returns the ICCP chunk bytes (plain ICC profile data).
func extractWebpICC(data []byte) ([]byte, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, fmt.Errorf("not WebP")
	}
	pos := 12
	for pos+8 <= len(data) {
		fourCC := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		contentStart := pos + 8
		contentEnd := contentStart + size
		if contentEnd > len(data) {
			return nil, fmt.Errorf("truncated chunk")
		}
		if fourCC == "ICCP" {
			out := make([]byte, size)
			copy(out, data[contentStart:contentEnd])
			return out, nil
		}
		pos = contentEnd
		if size&1 == 1 {
			pos++
		}
	}
	return nil, nil
}

// extractAvifICC walks the AVIF box hierarchy looking for a colr box of
// type "prof" inside meta/iprp/ipco.
func extractAvifICC(data []byte) ([]byte, error) {
	icc := findColrProf(data)
	if icc == nil {
		return nil, nil
	}
	return icc, nil
}

func findColrProf(data []byte) []byte {
	pos := 0
	for pos+8 <= len(data) {
		size := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		boxType := string(data[pos+4 : pos+8])
		headerLen := 8
		if size == 1 {
			if pos+16 > len(data) {
				return nil
			}
			size = int(binary.BigEndian.Uint64(data[pos+8 : pos+16]))
			headerLen = 16
		} else if size == 0 {
			size = len(data) - pos
		}
		if size < headerLen || pos+size > len(data) {
			return nil
		}
		contentStart := pos + headerLen
		contentEnd := pos + size

		switch boxType {
		case "colr":
			// colr layout: 4 bytes type ("prof"/"rICC"/"nclx") + payload
			if contentEnd-contentStart >= 4 {
				colorType := string(data[contentStart : contentStart+4])
				if colorType == "prof" || colorType == "rICC" {
					out := make([]byte, contentEnd-contentStart-4)
					copy(out, data[contentStart+4:contentEnd])
					return out
				}
			}
		case "meta":
			if contentEnd-contentStart >= 4 {
				if r := findColrProf(data[contentStart+4 : contentEnd]); r != nil {
					return r
				}
			}
		case "iprp", "ipco", "moov", "trak", "mdia", "minf", "stbl":
			if r := findColrProf(data[contentStart:contentEnd]); r != nil {
				return r
			}
		}
		pos += size
	}
	return nil
}

// findISPE recursively scans top-level boxes for the first ispe.
func findISPE(data []byte) (int, int, bool) {
	pos := 0
	for pos+8 <= len(data) {
		size := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		boxType := string(data[pos+4 : pos+8])
		headerLen := 8
		if size == 1 {
			if pos+16 > len(data) {
				return 0, 0, false
			}
			size = int(binary.BigEndian.Uint64(data[pos+8 : pos+16]))
			headerLen = 16
		} else if size == 0 {
			size = len(data) - pos
		}
		if size < headerLen || pos+size > len(data) {
			return 0, 0, false
		}
		contentStart := pos + headerLen
		contentEnd := pos + size

		switch boxType {
		case "ispe":
			// FullBox: 1 byte version + 3 bytes flags + 4 bytes width + 4 bytes height
			if contentEnd-contentStart < 12 {
				return 0, 0, false
			}
			w := int(binary.BigEndian.Uint32(data[contentStart+4 : contentStart+8]))
			h := int(binary.BigEndian.Uint32(data[contentStart+8 : contentStart+12]))
			return w, h, true
		case "meta":
			// FullBox: skip 4 bytes version+flags
			if contentEnd-contentStart < 4 {
				break
			}
			if w, h, ok := findISPE(data[contentStart+4 : contentEnd]); ok {
				return w, h, true
			}
		case "moov", "trak", "mdia", "minf", "stbl", "iprp", "ipco":
			// Plain container boxes (no version/flags)
			if w, h, ok := findISPE(data[contentStart:contentEnd]); ok {
				return w, h, true
			}
		}
		pos += size
	}
	return 0, 0, false
}
