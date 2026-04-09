// JPEG segment manipulation helpers used by NormalizeJpegOrientation's
// fallback path. These were previously test-only helpers; they are now
// production code so the orientation fallback can preserve ICC profiles
// across the lossy decode → rotate → re-encode round trip.

package modernimage

import (
	"encoding/binary"
	"fmt"
)

// injectJpegICC inserts a single APP2 ICC_PROFILE segment right after the
// SOI marker. Single-segment only — input ICC must fit in one APP2
// (max ~65500 bytes), which covers the vast majority of real-world
// profiles (sRGB, Adobe RGB, Display P3 are all < 10 KB).
func injectJpegICC(jpeg, icc []byte) ([]byte, error) {
	if len(jpeg) < 2 || jpeg[0] != 0xFF || jpeg[1] != 0xD8 {
		return nil, fmt.Errorf("not a JPEG")
	}
	const overhead = 2 + 12 + 2 // length + "ICC_PROFILE\0" + seq+total
	if len(icc)+overhead > 0xFFFF {
		return nil, fmt.Errorf("ICC too large for single APP2 segment")
	}
	segLen := overhead + len(icc)
	seg := make([]byte, 0, 2+segLen)
	seg = append(seg, 0xFF, 0xE2)
	seg = append(seg, byte(segLen>>8), byte(segLen&0xFF))
	seg = append(seg, []byte("ICC_PROFILE\x00")...)
	seg = append(seg, 0x01, 0x01) // chunk 1 of 1
	seg = append(seg, icc...)

	out := make([]byte, 0, len(jpeg)+len(seg))
	out = append(out, jpeg[:2]...)
	out = append(out, seg...)
	out = append(out, jpeg[2:]...)
	return out, nil
}

// extractJpegICC walks JPEG markers and concatenates all APP2 ICC_PROFILE
// segments in chunk-order. Returns nil if none found. Handles multi-segment
// ICC profiles (used by some cameras for large profiles).
func extractJpegICC(data []byte) ([]byte, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, fmt.Errorf("not a JPEG")
	}
	type iccChunk struct {
		seq, total int
		bytes      []byte
	}
	var chunks []iccChunk
	pos := 2
	for pos+4 <= len(data) {
		for pos < len(data) && data[pos] == 0xFF {
			pos++
		}
		if pos >= len(data) {
			break
		}
		marker := data[pos]
		pos++
		if marker == 0xD8 || marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
			continue
		}
		if marker == 0xDA {
			// SOS — entropy-coded data follows; ICC must be before SOS.
			break
		}
		if pos+2 > len(data) {
			break
		}
		segLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if segLen < 2 || pos+segLen > len(data) {
			return nil, fmt.Errorf("invalid segment")
		}
		segData := data[pos+2 : pos+segLen]
		pos += segLen

		if marker == 0xE2 && len(segData) >= 14 && string(segData[:12]) == "ICC_PROFILE\x00" {
			chunks = append(chunks, iccChunk{
				seq:   int(segData[12]),
				total: int(segData[13]),
				bytes: segData[14:],
			})
		}
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	out := make([]byte, 0)
	for s := 1; s <= chunks[0].total; s++ {
		for _, c := range chunks {
			if c.seq == s {
				out = append(out, c.bytes...)
				break
			}
		}
	}
	return out, nil
}
