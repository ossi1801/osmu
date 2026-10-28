package editor

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"osmu/pkg/platform"
)

// RenderDrawCmdsToImage rasterizes a slice of platform.DrawCmd into an image.RGBA buffer.
func RenderDrawCmdsToImage(width, height int, ops []platform.DrawCmd) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	toRGBA := func(r, g, b, a float64) color.RGBA {
		return color.RGBA{
			R: uint8(math.Max(0, math.Min(255, r*255.0))),
			G: uint8(math.Max(0, math.Min(255, g*255.0))),
			B: uint8(math.Max(0, math.Min(255, b*255.0))),
			A: uint8(math.Max(0, math.Min(255, a*255.0))),
		}
	}

	blendPixel := func(px, py int, c color.RGBA) {
		if px < 0 || py < 0 || px >= width || py >= height {
			return
		}
		if c.A == 255 {
			img.SetRGBA(px, py, c)
			return
		}
		dst := img.RGBAAt(px, py)
		a := float64(c.A) / 255.0
		inv := 1.0 - a
		img.SetRGBA(px, py, color.RGBA{
			R: uint8(float64(c.R)*a + float64(dst.R)*inv),
			G: uint8(float64(c.G)*a + float64(dst.G)*inv),
			B: uint8(float64(c.B)*a + float64(dst.B)*inv),
			A: 255,
		})
	}

	fillRect := func(x, y, w, h float64, c color.RGBA) {
		x0 := int(math.Round(x))
		y0 := int(math.Round(y))
		x1 := int(math.Round(x + w))
		y1 := int(math.Round(y + h))
		for py := y0; py < y1; py++ {
			for px := x0; px < x1; px++ {
				blendPixel(px, py, c)
			}
		}
	}

	drawLine := func(x1, y1, x2, y2, thickness float64, c color.RGBA) {
		dx := x2 - x1
		dy := y2 - y1
		steps := int(math.Max(math.Abs(dx), math.Abs(dy)) * 1.5)
		if steps < 1 {
			steps = 1
		}
		half := int(math.Max(0, (thickness-1)*0.5))
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps)
			px := int(math.Round(x1 + dx*t))
			py := int(math.Round(y1 + dy*t))
			for oy := -half; oy <= half; oy++ {
				for ox := -half; ox <= half; ox++ {
					blendPixel(px+ox, py+oy, c)
				}
			}
		}
	}

	for _, cmd := range ops {
		c := toRGBA(cmd.R, cmd.G, cmd.B, cmd.A)
		switch cmd.Kind {
		case platform.CmdClear:
			fillRect(0, 0, float64(width), float64(height), c)
		case platform.CmdRect:
			fillRect(cmd.X, cmd.Y, cmd.W, cmd.H, c)
		case platform.CmdTriangle:
			minX := int(math.Floor(math.Min(cmd.X, math.Min(cmd.X2, cmd.X3))))
			maxX := int(math.Ceil(math.Max(cmd.X, math.Max(cmd.X2, cmd.X3))))
			minY := int(math.Floor(math.Min(cmd.Y, math.Min(cmd.Y2, cmd.Y3))))
			maxY := int(math.Ceil(math.Max(cmd.Y, math.Max(cmd.Y2, cmd.Y3))))
			edge := func(ax, ay, bx, by, px, py float64) float64 {
				return (px-ax)*(by-ay) - (py-ay)*(bx-ax)
			}
			area := edge(cmd.X, cmd.Y, cmd.X2, cmd.Y2, cmd.X3, cmd.Y3)
			if math.Abs(area) > 0.1 {
				for py := minY; py <= maxY; py++ {
					for px := minX; px <= maxX; px++ {
						fx, fy := float64(px)+0.5, float64(py)+0.5
						w0 := edge(cmd.X2, cmd.Y2, cmd.X3, cmd.Y3, fx, fy)
						w1 := edge(cmd.X3, cmd.Y3, cmd.X, cmd.Y, fx, fy)
						w2 := edge(cmd.X, cmd.Y, cmd.X2, cmd.Y2, fx, fy)
						if (w0 >= 0 && w1 >= 0 && w2 >= 0) || (w0 <= 0 && w1 <= 0 && w2 <= 0) {
							blendPixel(px, py, c)
						}
					}
				}
			}
		case platform.CmdRectOutline:
			th := math.Max(1.0, cmd.Thickness)
			drawLine(cmd.X, cmd.Y, cmd.X+cmd.W, cmd.Y, th, c)
			drawLine(cmd.X+cmd.W, cmd.Y, cmd.X+cmd.W, cmd.Y+cmd.H, th, c)
			drawLine(cmd.X+cmd.W, cmd.Y+cmd.H, cmd.X, cmd.Y+cmd.H, th, c)
			drawLine(cmd.X, cmd.Y+cmd.H, cmd.X, cmd.Y, th, c)
		case platform.CmdLine:
			drawLine(cmd.X, cmd.Y, cmd.X2, cmd.Y2, math.Max(1.0, cmd.Thickness), c)
		case platform.CmdPolyLine:
			th := math.Max(1.0, cmd.Thickness)
			for i := 0; i < len(cmd.Points)-1; i++ {
				p1 := cmd.Points[i]
				p2 := cmd.Points[i+1]
				drawLine(p1[0], p1[1], p2[0], p2[1], th, c)
			}
		case platform.CmdCircle:
			r := int(math.Ceil(cmd.Radius))
			cx := int(math.Round(cmd.X))
			cy := int(math.Round(cmd.Y))
			r2 := cmd.Radius * cmd.Radius
			for oy := -r; oy <= r; oy++ {
				for ox := -r; ox <= r; ox++ {
					if float64(ox*ox+oy*oy) <= r2 {
						blendPixel(cx+ox, cy+oy, c)
					}
				}
			}
		case platform.CmdText:
			scale := cmd.Scale
			if scale <= 0 {
				scale = 1.5
			}
			cx := cmd.X
			cy := cmd.Y
			for i := 0; i < len(cmd.Text); i++ {
				glyph, ok := platform.Font5x7[cmd.Text[i]]
				if !ok {
					glyph = platform.Font5x7['?']
				}
				for col := 0; col < 5; col++ {
					bits := glyph[col]
					if bits == 0 {
						continue
					}
					for row := 0; row < 7; row++ {
						if (bits & (1 << row)) != 0 {
							fillRect(cx+float64(col)*scale, cy+float64(row)*scale, scale, scale, c)
						}
					}
				}
				cx += 6.0 * scale
			}
		}
	}

	return img
}

// SaveScreenshotPNG renders a frame of EditorApp at (width, height) and writes it to path.
func SaveScreenshotPNG(app *EditorApp, width, height int, path string) error {
	in := platform.NewInputState(width, height)
	ops := app.UpdateAndBuildDrawList(in, 0.016)
	img := RenderDrawCmdsToImage(width, height, ops)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
