// Command genicons 生成 CapsLayer 的应用与托盘图标。
//
// 输入：tools/genicons/assets/tray-on.png、tray-off.png（设计稿）
// 输出：
//   - internal/icon/tray-on.ico / tray-off.ico （托盘两种状态）
//   - build/windows/icon.ico                    （窗口/任务栏/exe 图标，取开启态）
//   - build/appicon.png                         （256px PNG，取开启态）
//
// 源图为不透明 PNG，会先按四角基准色 flood fill 去除近白背景并做边缘羽化，
// 再缩放到各档尺寸打包进 ICO：缩小用面积平均做抗锯齿，放大用 Catmull-Rom。
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

// icon 表示一张内存中的直通 Alpha（非预乘）RGBA 位图。
type icon struct {
	w, h int
	pix  []uint8 // 直通（非预乘）RGBA
}

// loadPNG 读取 PNG 并转换为直通 Alpha 的 RGBA 位图。
func loadPNG(path string) (*icon, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	im := &icon{w: b.Dx(), h: b.Dy(), pix: make([]uint8, b.Dx()*b.Dy()*4)}
	for y := 0; y < im.h; y++ {
		for x := 0; x < im.w; x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			// RGBA() 返回预乘值，这里还原为直通 Alpha。
			if a != 0 && a != 0xffff {
				r = r * 0xffff / a
				g = g * 0xffff / a
				bl = bl * 0xffff / a
			}
			i := (y*im.w + x) * 4
			im.pix[i+0] = uint8(r >> 8)
			im.pix[i+1] = uint8(g >> 8)
			im.pix[i+2] = uint8(bl >> 8)
			im.pix[i+3] = uint8(a >> 8)
		}
	}
	return im, nil
}

// cornerColor 取四角各一小块的平均色，作为背景基准色。
func cornerColor(im *icon) (r, g, b float64) {
	const n = 4
	var sr, sg, sb, cnt float64
	for _, c := range [][2]int{{0, 0}, {im.w - 1, 0}, {0, im.h - 1}, {im.w - 1, im.h - 1}} {
		for dy := 0; dy < n; dy++ {
			for dx := 0; dx < n; dx++ {
				x, y := c[0]+dx, c[1]+dy
				if c[0] == im.w-1 {
					x = c[0] - dx
				}
				if c[1] == im.h-1 {
					y = c[1] - dy
				}
				i := (y*im.w + x) * 4
				sr += float64(im.pix[i+0])
				sg += float64(im.pix[i+1])
				sb += float64(im.pix[i+2])
				cnt++
			}
		}
	}
	return sr / cnt, sg / cnt, sb / cnt
}

// removeBackground 从图像四周 flood fill 去除与背景基准色相近的像素。
// t1 以内的像素完全透明；t1~t2 之间的边缘像素做半透明羽化并反解前景色，
// 从而在保留圆内白色键帽的同时得到平滑的透明外圈。
func removeBackground(im *icon, t1, t2 float64) {
	br, bg, bb := cornerColor(im)
	dist := func(i int) float64 {
		dr := math.Abs(float64(im.pix[i+0]) - br)
		dg := math.Abs(float64(im.pix[i+1]) - bg)
		db := math.Abs(float64(im.pix[i+2]) - bb)
		return math.Max(dr, math.Max(dg, db))
	}

	w, h := im.w, im.h
	clear := make([]bool, w*h)
	stack := make([]int, 0, w*h)
	push := func(x, y int) {
		if x < 0 || y < 0 || x >= w || y >= h {
			return
		}
		p := y*w + x
		if clear[p] || im.pix[p*4+3] == 0 {
			return
		}
		if dist(p*4) > t1 {
			return
		}
		clear[p] = true
		stack = append(stack, p)
	}
	// 只从边缘向内蔓延，圆内的白色键帽不会被触及。
	for x := 0; x < w; x++ {
		push(x, 0)
		push(x, h-1)
	}
	for y := 0; y < h; y++ {
		push(0, y)
		push(w-1, y)
	}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := p%w, p/w
		push(x-1, y)
		push(x+1, y)
		push(x, y-1)
		push(x, y+1)
	}
	for p := range clear {
		if clear[p] {
			im.pix[p*4+3] = 0
		}
	}

	// 羽化紧邻透明区的边缘像素。
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			if clear[p] {
				continue
			}
			i := p * 4
			d := dist(i)
			if d <= t1 || d >= t2 {
				continue
			}
			adjacent := false
			for _, n := range [][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
				if n[0] >= 0 && n[1] >= 0 && n[0] < w && n[1] < h && clear[n[1]*w+n[0]] {
					adjacent = true
					break
				}
			}
			if !adjacent {
				continue
			}
			a := (d - t1) / (t2 - t1)
			if a <= 0 {
				im.pix[i+3] = 0
				continue
			}
			if a > 1 {
				a = 1
			}
			// 观测色 = 前景*a + 背景*(1-a)，据此反解前景色。
			unmul := func(c, bc float64) float64 {
				return (c - bc*(1-a)) / a
			}
			im.pix[i+0] = clamp8(unmul(float64(im.pix[i+0]), br))
			im.pix[i+1] = clamp8(unmul(float64(im.pix[i+1]), bg))
			im.pix[i+2] = clamp8(unmul(float64(im.pix[i+2]), bb))
			im.pix[i+3] = clamp8(a * 255)
		}
	}
}

// resizeArea 用面积加权平均把源图缩放到 size×size，正确处理 Alpha 预乘。
// 适合缩小，能有效抗锯齿。
func resizeArea(src *icon, size int) *icon {
	dst := &icon{w: size, h: size, pix: make([]uint8, size*size*4)}
	sx := float64(src.w) / float64(size)
	sy := float64(src.h) / float64(size)
	for ty := 0; ty < size; ty++ {
		y0, y1 := float64(ty)*sy, float64(ty+1)*sy
		for tx := 0; tx < size; tx++ {
			x0, x1 := float64(tx)*sx, float64(tx+1)*sx
			var asum, rs, gs, bs, wsum float64
			for syi := int(math.Floor(y0)); syi < int(math.Ceil(y1)); syi++ {
				if syi < 0 || syi >= src.h {
					continue
				}
				oy := math.Min(y1, float64(syi+1)) - math.Max(y0, float64(syi))
				if oy <= 0 {
					continue
				}
				for sxi := int(math.Floor(x0)); sxi < int(math.Ceil(x1)); sxi++ {
					if sxi < 0 || sxi >= src.w {
						continue
					}
					ox := math.Min(x1, float64(sxi+1)) - math.Max(x0, float64(sxi))
					if ox <= 0 {
						continue
					}
					wgt := ox * oy
					i := (syi*src.w + sxi) * 4
					a := float64(src.pix[i+3]) / 255
					asum += a * wgt
					rs += float64(src.pix[i+0]) * a * wgt
					gs += float64(src.pix[i+1]) * a * wgt
					bs += float64(src.pix[i+2]) * a * wgt
					wsum += wgt
				}
			}
			d := (ty*size + tx) * 4
			if asum > 0 && wsum > 0 {
				dst.pix[d+0] = clamp8(rs / asum)
				dst.pix[d+1] = clamp8(gs / asum)
				dst.pix[d+2] = clamp8(bs / asum)
				dst.pix[d+3] = clamp8(asum / wsum * 255)
			}
		}
	}
	return dst
}

// catmullRom 是 a=-0.5 的三次卷积核，用于放大插值。
func catmullRom(t float64) float64 {
	if t < 0 {
		t = -t
	}
	if t < 1 {
		return (1.5*t-2.5)*t*t + 1
	}
	if t < 2 {
		return ((-0.5*t+2.5)*t-4)*t + 2
	}
	return 0
}

type tap struct {
	idx int
	w   float64
}

// buildWeights 为沿单个维度从 srcN 缩放到 dstN 预计算卷积权重。
// 缩小时按比例展宽核，避免走样。
func buildWeights(srcN, dstN int) [][]tap {
	scale := float64(srcN) / float64(dstN)
	filter := math.Max(scale, 1)
	out := make([][]tap, dstN)
	for i := 0; i < dstN; i++ {
		center := (float64(i)+0.5)*scale - 0.5
		left := int(math.Ceil(center - 2*filter))
		right := int(math.Floor(center + 2*filter))
		var sum float64
		for j := left; j <= right; j++ {
			if j < 0 || j >= srcN {
				continue
			}
			w := catmullRom((float64(j) - center) / filter)
			if w == 0 {
				continue
			}
			out[i] = append(out[i], tap{j, w})
			sum += w
		}
		if sum != 0 {
			for k := range out[i] {
				out[i][k].w /= sum
			}
		}
	}
	return out
}

// resample 用 Catmull-Rom 把源图缩放到 w×h，按预乘 Alpha 计算后再还原。
func resample(src *icon, w, h int) *icon {
	xw := buildWeights(src.w, w)
	yw := buildWeights(src.h, h)

	// 横向：src.w×src.h -> w×src.h（预乘）
	tmp := make([]float64, w*src.h*4)
	for y := 0; y < src.h; y++ {
		for xd := 0; xd < w; xd++ {
			var r, g, b, a float64
			for _, t := range xw[xd] {
				i := (y*src.w + t.idx) * 4
				av := float64(src.pix[i+3]) / 255
				r += float64(src.pix[i+0]) * av * t.w
				g += float64(src.pix[i+1]) * av * t.w
				b += float64(src.pix[i+2]) * av * t.w
				a += av * t.w
			}
			j := (y*w + xd) * 4
			tmp[j+0], tmp[j+1], tmp[j+2], tmp[j+3] = r, g, b, a
		}
	}
	// 纵向：w×src.h -> w×h
	dst := &icon{w: w, h: h, pix: make([]uint8, w*h*4)}
	for yd := 0; yd < h; yd++ {
		for x := 0; x < w; x++ {
			var r, g, b, a float64
			for _, t := range yw[yd] {
				j := (t.idx*w + x) * 4
				r += tmp[j+0] * t.w
				g += tmp[j+1] * t.w
				b += tmp[j+2] * t.w
				a += tmp[j+3] * t.w
			}
			d := (yd*w + x) * 4
			dst.pix[d+3] = clamp8(a * 255)
			if a > 0 {
				dst.pix[d+0] = clamp8(r / a)
				dst.pix[d+1] = clamp8(g / a)
				dst.pix[d+2] = clamp8(b / a)
			}
		}
	}
	return dst
}

// resizeBest 缩小走面积平均，放大走 Catmull-Rom。
func resizeBest(src *icon, size int) *icon {
	if size <= src.w {
		return resizeArea(src, size)
	}
	return resample(src, size, size)
}

// encodeBMPEntry 将单张图标编码为 ICO 中的一项：32bpp 自下而上的 BMP 像素
// 数据，后接全零的 AND 掩码。
func encodeBMPEntry(im *icon) []byte {
	w, h := im.w, im.h
	head := make([]byte, 40)
	binary.LittleEndian.PutUint32(head[0:], 40)          // 结构体大小
	binary.LittleEndian.PutUint32(head[4:], uint32(w))   // 宽
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
	dir := make([]byte, 6+16*n)               // ICONDIR(6) + n * ICONDIRENTRY(16)
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
		binary.LittleEndian.PutUint16(e[4:], 1)                     // 平面数
		binary.LittleEndian.PutUint16(e[6:], 32)                    // 位深
		binary.LittleEndian.PutUint32(e[8:], uint32(len(blobs[i]))) // 数据大小
		binary.LittleEndian.PutUint32(e[12:], uint32(offset))       // 数据偏移
		offset += len(blobs[i])
	}

	out := append([]byte{}, dir...)
	for _, b := range blobs {
		out = append(out, b...)
	}
	return out
}

// clamp8 把浮点色值夹到 [0,255] 并四舍五入为字节。
func clamp8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// icoFromSource 把源图缩放到多种尺寸并打包为 ICO。
func icoFromSource(src *icon, sizes []int) []byte {
	imgs := make([]*icon, 0, len(sizes))
	for _, s := range sizes {
		imgs = append(imgs, resizeBest(src, s))
	}
	return encodeICO(imgs)
}

func writePNG(path string, im *icon) error {
	nrgba := image.NewNRGBA(image.Rect(0, 0, im.w, im.h))
	copy(nrgba.Pix, im.pix)
	var buf bytes.Buffer
	if err := png.Encode(&buf, nrgba); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func main() {
	on, err := loadPNG("tools/genicons/assets/tray-on.png")
	if err != nil {
		panic(err)
	}
	off, err := loadPNG("tools/genicons/assets/tray-off.png")
	if err != nil {
		panic(err)
	}
	removeBackground(on, 10, 45)
	removeBackground(off, 10, 45)

	if err := os.MkdirAll(filepath.Join("internal", "icon"), 0o755); err != nil {
		panic(err)
	}
	traySizes := []int{16, 20, 24, 32}
	if err := os.WriteFile("internal/icon/tray-on.ico", icoFromSource(on, traySizes), 0o644); err != nil {
		panic(err)
	}
	if err := os.WriteFile("internal/icon/tray-off.ico", icoFromSource(off, traySizes), 0o644); err != nil {
		panic(err)
	}

	// 程序 / 任务栏 / exe 图标沿用开启态设计。
	appSizes := []int{16, 24, 32, 48, 64, 128, 256}
	if err := os.WriteFile("build/windows/icon.ico", icoFromSource(on, appSizes), 0o644); err != nil {
		panic(err)
	}
	if err := writePNG("build/appicon.png", resizeBest(on, 256)); err != nil {
		panic(err)
	}

	fmt.Println("icons written")
}
