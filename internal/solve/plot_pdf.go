package solve

// PDF-canvas для gonum/plot с корректной поддержкой UTF-8.
//
// Штатный gonum.org/v1/plot/vg/vgpdf встраивает шрифт с кодовой страницей
// cp1252 и пишет в поток сырые байты UTF-8: µ (UTF-8 C2 B5) в PDF попадает
// как два символа "Âµ", δ (CE B4) — как "Î´", кириллица — как "Ð...".
// Поэтому для встроенного бэкенда используется этот canvas: шрифты
// регистрируются через AddUTF8FontFromBytes (Identity-H + ToUnicode),
// благодаря чему корректно рисуются греческий, кириллица и типографика.
//
// Отрисовка путей/трансформаций повторяет vg/vgpdf из gonum/plot
// (BSD-3-Clause, Copyright ©2015 The Gonum Authors).

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"

	pdf "codeberg.org/go-pdf/fpdf"
	stdfnt "golang.org/x/image/font"

	"gonum.org/v1/plot/font"
	"gonum.org/v1/plot/vg"
)

// pdfDPI — номинальное разрешение PDF (1 pt = 1/72 дюйма).
const pdfDPI = 72

type pdfContext struct {
	fill  color.Color
	line  color.Color
	width vg.Length
}

// pdfCanvas реализует vg.Canvas с UTF-8 шрифтами.
type pdfCanvas struct {
	doc  *pdf.Fpdf
	w, h vg.Length

	numImages int
	stack     []pdfContext
	fonts     map[font.Font]struct{}
}

var _ vg.Canvas = (*pdfCanvas)(nil)

func newPDFCanvas(w, h vg.Length) *pdfCanvas {
	cfg := pdf.InitType{
		UnitStr: "pt",
		Size:    pdf.SizeType{Wd: w.Points(), Ht: h.Points()},
	}
	c := &pdfCanvas{
		doc:   pdf.NewCustom(&cfg),
		w:     w,
		h:     h,
		stack: make([]pdfContext, 1),
		fonts: make(map[font.Font]struct{}),
	}
	c.doc.SetCompression(true)
	c.doc.SetMargins(0, 0, 0)
	c.doc.AddPage()
	c.Push()
	c.Translate(vg.Point{X: 0, Y: c.h})
	c.Scale(1, -1)
	vg.Initialize(c)
	return c
}

func (c *pdfCanvas) context() *pdfContext { return &c.stack[len(c.stack)-1] }

func (c *pdfCanvas) Size() (w, h vg.Length) { return c.w, c.h }

func (c *pdfCanvas) SetLineWidth(w vg.Length) {
	c.context().width = w
	c.doc.SetLineWidth(c.unit(w))
}

func (c *pdfCanvas) SetLineDash(dashes []vg.Length, offs vg.Length) {
	ds := make([]float64, len(dashes))
	for i, d := range dashes {
		ds[i] = c.unit(d)
	}
	c.doc.SetDashPattern(ds, c.unit(offs))
}

func (c *pdfCanvas) SetColor(clr color.Color) {
	if clr == nil {
		clr = color.Black
	}
	c.context().line = clr
	c.context().fill = clr
	r, g, b, a := rgba(clr)
	c.doc.SetFillColor(r, g, b)
	c.doc.SetDrawColor(r, g, b)
	c.doc.SetTextColor(r, g, b)
	c.doc.SetAlpha(a, "Normal")
}

func (c *pdfCanvas) Rotate(r float64) {
	c.doc.TransformRotate(-r*180/math.Pi, 0, 0)
}

func (c *pdfCanvas) Translate(pt vg.Point) {
	xp, yp := c.pdfPoint(pt)
	c.doc.TransformTranslate(xp, yp)
}

func (c *pdfCanvas) Scale(x, y float64) {
	c.doc.TransformScale(x*100, y*100, 0, 0)
}

func (c *pdfCanvas) Push() {
	c.stack = append(c.stack, *c.context())
	c.doc.TransformBegin()
}

func (c *pdfCanvas) Pop() {
	c.doc.TransformEnd()
	c.stack = c.stack[:len(c.stack)-1]
}

func (c *pdfCanvas) Stroke(p vg.Path) {
	if c.context().width > 0 {
		c.pdfPath(p, "D")
	}
}

func (c *pdfCanvas) Fill(p vg.Path) {
	c.pdfPath(p, "F")
}

func (c *pdfCanvas) FillString(fnt font.Face, pt vg.Point, str string) {
	if fnt.Font.Size == 0 || str == "" {
		return
	}
	style := ""
	if fnt.Font.Weight == stdfnt.WeightBold {
		style += "B"
	}
	if fnt.Font.Style == stdfnt.StyleItalic {
		style += "I"
	}
	c.registerFont(fnt, style)

	c.doc.SetFont(fnt.Name(), style, c.unit(fnt.Font.Size))
	c.Push()
	defer c.Pop()
	c.Translate(pt)
	// fpdf использует левый верхний угол как начало координат.
	c.Scale(1, -1)

	h := c.unit(fnt.Font.Size)
	w := c.doc.GetStringWidth(str)
	desc := c.doc.GetFontDesc(fnt.Name(), style)
	var top float64
	if desc.Ascent == 0 && desc.Descent == 0 {
		top = -0.81 * h
	} else {
		top = -float64(desc.Ascent) * h / float64(desc.Ascent-desc.Descent)
	}
	margin := c.doc.GetCellMargin()
	c.doc.MoveTo(-margin, top)
	c.doc.CellFormat(w, h, str, "", 0, "BL", false, 0, "")
}

// registerFont добавляет TTF текущего начертания как UTF-8 шрифт fpdf.
func (c *pdfCanvas) registerFont(fnt font.Face, style string) {
	if _, ok := c.fonts[fnt.Font]; ok {
		return
	}
	raw := new(bytes.Buffer)
	if _, err := fnt.Face.WriteSourceTo(nil, raw); err != nil {
		panic(fmt.Sprintf("pdf canvas: could not read font %q: %v",
			fnt.Name(), err))
	}
	c.doc.AddUTF8FontFromBytes(fnt.Name(), style, raw.Bytes())
	c.fonts[fnt.Font] = struct{}{}
}

func (c *pdfCanvas) DrawImage(rect vg.Rectangle, img image.Image) {
	opts := pdf.ImageOptions{ImageType: "png", ReadDpi: true}
	name := c.imageName()

	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		panic(fmt.Sprintf("pdf canvas: error encoding image to PNG: %v", err))
	}
	c.doc.RegisterImageOptionsReader(name, opts, buf)

	xp, yp := c.pdfPoint(rect.Min)
	wp, hp := c.pdfPoint(rect.Size())
	c.doc.ImageOptions(name, xp, yp, wp, hp, false, opts, 0, "")
}

func (c *pdfCanvas) pdfPath(path vg.Path, style string) {
	var (
		xp float64
		yp float64
	)
	for _, comp := range path {
		switch comp.Type {
		case vg.MoveComp:
			xp, yp = c.pdfPoint(comp.Pos)
			c.doc.MoveTo(xp, yp)
		case vg.LineComp:
			xp, yp = c.pdfPoint(comp.Pos)
			c.doc.LineTo(xp, yp)
		case vg.ArcComp:
			c.arc(comp, style)
		case vg.CurveComp:
			px, py := c.pdfPoint(comp.Pos)
			switch len(comp.Control) {
			case 1:
				cx, cy := c.pdfPoint(comp.Control[0])
				c.doc.CurveTo(cx, cy, px, py)
			case 2:
				cx, cy := c.pdfPoint(comp.Control[0])
				dx, dy := c.pdfPoint(comp.Control[1])
				c.doc.CurveBezierCubicTo(cx, cy, dx, dy, px, py)
			default:
				panic("pdf canvas: invalid number of control points")
			}
		case vg.CloseComp:
			c.doc.ClosePath()
		default:
			panic(fmt.Sprintf("pdf canvas: unknown path component type: %d",
				comp.Type))
		}
	}
	c.doc.DrawPath(style)
}

func (c *pdfCanvas) arc(comp vg.PathComp, style string) {
	x0 := comp.Pos.X + comp.Radius*vg.Length(math.Cos(comp.Start))
	y0 := comp.Pos.Y + comp.Radius*vg.Length(math.Sin(comp.Start))
	c.doc.LineTo(c.pdfPointXY(x0, y0))
	r := c.unit(comp.Radius)
	const deg = 180 / math.Pi
	angle := comp.Angle * deg
	beg := comp.Start * deg
	end := beg + angle
	x := c.unit(comp.Pos.X)
	y := c.unit(comp.Pos.Y)
	c.doc.Arc(x, y, r, r, angle, beg, end, style)
	x1 := comp.Pos.X + comp.Radius*vg.Length(math.Cos(comp.Start+comp.Angle))
	y1 := comp.Pos.Y + comp.Radius*vg.Length(math.Sin(comp.Start+comp.Angle))
	c.doc.MoveTo(c.pdfPointXY(x1, y1))
}

func (c *pdfCanvas) pdfPointXY(x, y vg.Length) (float64, float64) {
	return c.unit(x), c.unit(y)
}

func (c *pdfCanvas) pdfPoint(pt vg.Point) (float64, float64) {
	return c.unit(pt.X), c.unit(pt.Y)
}

// unit переводит vg.Length в единицы fpdf.
func (c *pdfCanvas) unit(l vg.Length) float64 { return l.Dots(pdfDPI) }

func (c *pdfCanvas) imageName() string {
	c.numImages++
	return fmt.Sprintf("image_%03d.png", c.numImages)
}

type writerCounter struct {
	io.Writer
	n int64
}

func (w *writerCounter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.n += int64(n)
	return n, err
}

// WriteTo записывает PDF; после вызова canvas нельзя использовать.
func (c *pdfCanvas) WriteTo(w io.Writer) (int64, error) {
	c.Pop() // базовая трансформация из newPDFCanvas
	c.doc.Close()
	wc := &writerCounter{Writer: w}
	b := bufio.NewWriter(wc)
	if err := c.doc.Output(b); err != nil {
		return wc.n, err
	}
	return wc.n, b.Flush()
}

func rgba(c color.Color) (int, int, int, float64) {
	if c == nil {
		c = color.Black
	}
	r, g, b, a := c.RGBA()
	return int(r >> 8), int(g >> 8), int(b >> 8), float64(a) / math.MaxUint16
}
