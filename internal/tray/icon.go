package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"sync"

	"github.com/erros-gg/journal-companion/assets"
)

// iconDefault returns the base tray icon bytes (the embedded ICO file).
func iconDefault() []byte {
	return assets.Icon
}

var (
	workingOnce sync.Once
	workingData []byte
)

// iconWorking returns icon bytes for the "upload in progress" state.
// Built once on first call by drawing a gold dot over the base icon.
func iconWorking() []byte {
	workingOnce.Do(func() {
		workingData = buildWorkingIcon()
	})
	return workingData
}

// buildWorkingIcon extracts a PNG frame from the embedded ICO, draws a gold
// dot overlay in the bottom-right corner, and returns PNG-encoded bytes.
// Falls back to the plain ICO if no PNG frame is found in the container.
func buildWorkingIcon() []byte {
	base, err := extractPNGFromICO(assets.Icon)
	if err != nil {
		slog.Warn("icon overlay: no PNG frame in ICO, working icon will match default", "err", err)
		return assets.Icon
	}

	bounds := base.Bounds()
	out := image.NewNRGBA(bounds)
	draw.Draw(out, bounds, base, bounds.Min, draw.Src)

	// 6×6 gold dot (brand color #C9A24A) at bottom-right, inset 2 px from edge.
	dot := color.NRGBA{R: 201, G: 162, B: 74, A: 255}
	w, h := bounds.Max.X, bounds.Max.Y
	for dy := 0; dy < 6; dy++ {
		for dx := 0; dx < 6; dx++ {
			out.SetNRGBA(w-8+dx, h-8+dy, dot)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		slog.Warn("icon overlay: encode failed, using default", "err", err)
		return assets.Icon
	}
	return buf.Bytes()
}

// extractPNGFromICO scans an ICO container for a PNG-compressed frame.
// ICO format: 6-byte ICONDIR, then one 16-byte ICONDIRENTRY per image.
func extractPNGFromICO(icoData []byte) (image.Image, error) {
	if len(icoData) < 6 {
		return nil, bytes.ErrTooLarge
	}
	count := int(binary.LittleEndian.Uint16(icoData[4:6]))
	for i := 0; i < count; i++ {
		entry := 6 + i*16
		if entry+16 > len(icoData) {
			break
		}
		size := int(binary.LittleEndian.Uint32(icoData[entry+8 : entry+12]))
		off := int(binary.LittleEndian.Uint32(icoData[entry+12 : entry+16]))
		if off+size > len(icoData) || size < 8 {
			continue
		}
		frame := icoData[off : off+size]
		// PNG magic bytes: \x89 P N G
		if frame[0] == 0x89 && frame[1] == 'P' && frame[2] == 'N' && frame[3] == 'G' {
			return png.Decode(bytes.NewReader(frame))
		}
	}
	return nil, bytes.ErrTooLarge
}
