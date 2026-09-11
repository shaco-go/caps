// Command genicons 用于生成 CapsLayer 的应用图标。
//
// 它会绘制一个带向上箭头的圆角键帽，并输出：
//   - internal/icon/tray-on.ico / tray-off.ico  （托盘两种状态，32bpp BMP 格式）
//   - build/windows/icon.ico                     （窗口/任务栏/exe 图标）
//   - build/appicon.png                          （256px PNG）
//
// 请在模块根目录执行：go run ./tools/genicons
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

type rgba struct{ r, g, b, a uint8 }

// icon 表示一张内存中的直通 Alpha（非预乘）RGBA 位图。
type icon struct {
	w, h int
	pix  []uint8 // 直通（非预乘）RGBA
}

// 两套配色：启用（蓝底白箭头）与暂停（灰底浅箭头）。
var (
	onBG  = rgba{0x2f, 0x6f, 0xed, 255}
	onFG  = rgba{255, 255, 255, 255}
	offBG = rgba{0x6b, 0x72, 0x80, 255}
	offFG = rgba{0xd1, 0xd5, 0xdb, 255}
)

// inRoundRect 判断点 (px,py) 是否落在圆角矩形内。
func inRoundRect(px, py, x0, y0, x1, y1, r float64) bool {
	if px < x0 || px > x1 || py < y0 || py > y1 {
		return false
	}
	// 把点夹到圆角对应的“圆心”位置，再判断到圆心的距离。
	cx, cy := px, py
	if px < x0+r {
		cx = x0 + r
	} else if px > x1-r {
		cx = x1 - r
	}
	if py < y0+r {
		cy = y0 + r
	} else if py > y1-r {
		cy = y1 - r
	}
	dx, dy := px-cx, py-cy
	return dx*dx+dy*dy <= r*r
}

// inArrow 判断点 (px,py) 是否落在向上箭头（三角箭头 + 竖直箭杆）内。
func inArrow(px, py, cx, headTop, headBase, headHalf, stemHalf, stemBottom float64) bool {
	if py >= headTop && py <= headBase {
		t := (py - headTop) / (headBase - headTop)
		if math.Abs(px-cx) <= headHalf*t {
			return true
		}
	}
	if py >= headBase-0.02*(stemBottom-headTop) && py <= stemBottom {
		if math.Abs(px-cx) <= stemHalf {
			return true
		}
	}
	return false
}

// render 以 4 倍超采样绘制指定像素尺寸的图标。
func render(size int, on bool) *icon {
	const ss = 4
	big := size * ss
	bg, fg := onBG, onFG
	if !on {
		bg, fg = offBG, offFG
	}

	margin := 0.09 * float64(big)
	x0, y0 := margin, margin
	x1, y1 := float64(big)-margin, float64(big)-margin
	radius := 0.30 * float64(big)

	cx := float64(big) / 2
	headTop := 0.30 * float64(big)
	headBase := 0.52 * float64(big)
	headHalf := 0.22 * float64(big)
	stemHalf := 0.075 * float64(big)
	stemBottom := 0.72 * float64(big)

	// 每个最终像素累计其 SS×SS 个子采样点的颜色覆盖率。
	type acc struct{ r, g, b, cov float64 }
	accs := make([]acc, size*size)
	inv := 1.0 / float64(ss*ss)

	for by := 0; by < big; by++ {
		for bx := 0; bx < big; bx++ {
			px, py := float64(bx)+0.5, float64(by)+0.5
			if !inRoundRect(px, py, x0, y0, x1, y1, radius) {
				continue
			}
			c := bg
			if inArrow(px, py, cx, headTop, headBase, headHalf, stemHalf, stemBottom) {
				c = fg
			}
			a := &accs[(by/ss)*size+(bx/ss)]
			a.r += float64(c.r)
			a.g += float64(c.g)
			a.b += float64(c.b)
			a.cov++
		}
	}

	im := &icon{w: size, h: size, pix: make([]uint8, size*size*4)}
	for i := range accs {
		a := accs[i]
		if a.cov == 0 {
			continue
		}
		im.pix[i*4+0] = uint8(a.r/a.cov + 0.5)
		im.pix[i*4+1] = uint8(a.g/a.cov + 0.5)
		im.pix[i*4+2] = uint8(a.b/a.cov + 0.5)
		im.pix[i*4+3] = uint8(a.cov*inv*255 + 0.5)
	}
	return im
}

// encodeBMPEntry 将单张图标编码为 ICO 中的一项：32bpp 自下而上的 BMP 像素
// 数据，后接全零的 AND 掩码。
func encodeBMPEntry(im *icon) []byte {
	w, h := im.w, im.h
	head := make([]byte, 40)
	binary.LittleEndian.PutUint32(head[0:], 40)         // 结构体大小
	binary.LittleEndian.PutUint32(head[4:], uint32(w))  // 宽
	binary.LittleEndian.PutUint32(head[8:], uint32(h*2)) // 高（BMP 中双倍，含掩码）
	binary.LittleEndian.PutUint16(head[12:], 1)          // 平面数
	binary.LittleEndian.PutUint16(head[14:], 32)         // 位深
	binary.LittleEndian.PutUint32(head[20:], uint32(w*h*4))

	pix := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s := (y*w + x) * 4
			d := ((h-1-y)*w + x) * 4 // BMP 自下而上存储，翻转 Y
			pix[d+0] = im.pix[s+2]   // B
			pix[d+1] = im.pix[s+1]   // G
			pix[d+2] = im.pix[s+0]   // R
			pix[d+3] = im.pix[s+3]   // A
		}
	}
	// AND 掩码按 4 字节对齐，32bpp 图标可全零。
	maskRow := ((w + 31) / 32) * 4
	mask := make([]byte, maskRow*h)

	out := make([]byte, 0, len(head)+len(pix)+len(mask))
	out = append(out, head...)
	out = append(out, pix...)
	out = append(out, mask...)
	return out
}

// encodeICO 将多张图标打包成标准 ICO 文件。
func encodeICO(images []*icon) []byte {
	n := len(images)
	dir := make([]byte, 6+16*n) // ICONDIR(6) + n * ICONDIRENTRY(16)
	binary.LittleEndian.PutUint16(dir[0:], 0) // 保留字段
	binary.LittleEndian.PutUint16(dir[2:], 1) // 类型：1 = 图标
	binary.LittleEndian.PutUint16(dir[4:], uint16(n))

	blobs := make([][]byte, n)
	for i, im := range images {
		blobs[i] = encodeBMPEntry(im)
	}
	offset := 6 + 16*n
	for i, im := range images {
		e := dir[6+16*i:]
		// ICO 中用 0 表示 256。
		if im.w >= 256 {
			e[0] = 0
		} else {
			e[0] = uint8(im.w)
		}
		if im.h >= 256 {
			e[1] = 0
		} else {
			e[1] = uint8(im.h)
		}
		binary.LittleEndian.PutUint16(e[4:], 1)  // 平面数
		binary.LittleEndian.PutUint16(e[6:], 32) // 位深
		binary.LittleEndian.PutUint32(e[8:], uint32(len(blobs[i])))  // 数据大小
		binary.LittleEndian.PutUint32(e[12:], uint32(offset))       // 数据偏移
		offset += len(blobs[i])
	}

	out := append([]byte{}, dir...)
	for _, b := range blobs {
		out = append(out, b...)
	}
	return out
}

func main() {
	if err := os.MkdirAll(filepath.Join("internal", "icon"), 0o755); err != nil {
		panic(err)
	}
	sizes := []int{16, 20, 24, 32}
	if err := os.WriteFile("internal/icon/tray-on.ico", multiICO(sizes, true), 0o644); err != nil {
		panic(err)
	}
	if err := os.WriteFile("internal/icon/tray-off.ico", multiICO(sizes, false), 0o644); err != nil {
		panic(err)
	}
	appSizes := []int{16, 24, 32, 48, 64, 128, 256}
	if err := os.WriteFile("build/windows/icon.ico", multiICO(appSizes, true), 0o644); err != nil {
		panic(err)
	}

	// 附带一张 256px PNG，供偏好 PNG 的平台/工具使用。
	im := render(256, true)
	nrgba := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	copy(nrgba.Pix, im.pix)
	var buf bytes.Buffer
	if err := png.Encode(&buf, nrgba); err != nil {
		panic(err)
	}
	if err := os.WriteFile("build/appicon.png", buf.Bytes(), 0o644); err != nil {
		panic(err)
	}

	fmt.Println("icons written")
}

// multiICO 渲染多种尺寸并打包为单个 ICO。
func multiICO(sizes []int, on bool) []byte {
	imgs := make([]*icon, 0, len(sizes))
	for _, s := range sizes {
		imgs = append(imgs, render(s, on))
	}
	return encodeICO(imgs)
}
