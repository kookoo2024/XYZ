package tray

import (
	"bytes"
	"image"
	"image/color"
	"math"
)

// ===== 生成托盘图标（16x16 像素 ICO 格式，Windows 兼容） =====
// Icon 托盘图标（金色五角星，16x16 ICO）。
func Icon() []byte {
	const sz = 16

	// 创建 32 位 RGBA 图像
	img := image.NewRGBA(image.Rect(0, 0, sz, sz))

	// 金黄色星星
	starColor := color.RGBA{255, 180, 0, 255}

	cx, cy := float64(sz)/2, float64(sz)/2

	// 生成五角星的顶点坐标
	points := make([][2]float64, 10)
	outerRadius := float64(sz) * 0.45
	innerRadius := outerRadius * 0.4

	for i := 0; i < 10; i++ {
		angle := float64(i)*36.0 - 90.0
		rad := angle * 3.14159265359 / 180.0
		var r float64
		if i%2 == 0 {
			r = outerRadius
		} else {
			r = innerRadius
		}
		points[i][0] = cx + r*math.Cos(rad)
		points[i][1] = cy + r*math.Sin(rad)
	}

	// 填充五角星（不填充背景，保持透明）
	for y := 0; y < sz; y++ {
		for x := 0; x < sz; x++ {
			if isInsidePolygon(float64(x)+0.5, float64(y)+0.5, points) {
				img.Set(x, y, starColor)
			}
		}
	}

	// 构建 ICO 格式
	// ICO 头: 6 字节
	// 图像目录: 16 字节
	// 图像数据: BMP 格式

	var ico bytes.Buffer

	// ICO 头
	ico.Write([]byte{0, 0}) // Reserved
	ico.Write([]byte{1, 0}) // Type: 1 = ICO
	ico.Write([]byte{1, 0}) // Number of images

	// 图像目录项
	ico.WriteByte(16)        // Width
	ico.WriteByte(16)        // Height
	ico.WriteByte(0)         // Color palette
	ico.WriteByte(0)         // Reserved
	ico.Write([]byte{1, 0})  // Color planes
	ico.Write([]byte{32, 0}) // Bits per pixel

	// BMP 数据大小 (40 字节头 + 16*16*4 字节像素数据)
	bmpDataSize := 40 + sz*sz*4
	sizeBytes := make([]byte, 4)
	sizeBytes[0] = byte(bmpDataSize)
	sizeBytes[1] = byte(bmpDataSize >> 8)
	sizeBytes[2] = byte(bmpDataSize >> 16)
	sizeBytes[3] = byte(bmpDataSize >> 24)
	ico.Write(sizeBytes)

	// 图像数据偏移 (6 + 16 = 22)
	offsetBytes := []byte{22, 0, 0, 0}
	ico.Write(offsetBytes)

	// BMP 信息头 (40 字节)
	ico.Write([]byte{40, 0, 0, 0}) // Header size
	ico.Write([]byte{16, 0, 0, 0}) // Width
	ico.Write([]byte{32, 0, 0, 0}) // Height (doubled for ICO)
	ico.Write([]byte{1, 0})        // Planes
	ico.Write([]byte{32, 0})       // Bits per pixel
	ico.Write([]byte{0, 0, 0, 0})  // Compression
	ico.Write([]byte{0, 0, 0, 0})  // Image size (can be 0 for uncompressed)
	ico.Write([]byte{0, 0, 0, 0})  // X pixels per meter
	ico.Write([]byte{0, 0, 0, 0})  // Y pixels per meter
	ico.Write([]byte{0, 0, 0, 0})  // Colors used
	ico.Write([]byte{0, 0, 0, 0})  // Important colors

	// 像素数据 (从下到上，从左到右)
	for y := sz - 1; y >= 0; y-- {
		for x := 0; x < sz; x++ {
			c := img.RGBAAt(x, y)
			ico.WriteByte(c.B)
			ico.WriteByte(c.G)
			ico.WriteByte(c.R)
			ico.WriteByte(c.A)
		}
	}

	return ico.Bytes()
}

// 判断点是否在多边形内（射线法）
func isInsidePolygon(x, y float64, points [][2]float64) bool {
	n := len(points)
	inside := false

	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := points[i][0], points[i][1]
		xj, yj := points[j][0], points[j][1]

		if ((yi > y) != (yj > y)) && (x < (xj-xi)*(y-yi)/(yj-yi)+xi) {
			inside = !inside
		}
		j = i
	}

	return inside
}
