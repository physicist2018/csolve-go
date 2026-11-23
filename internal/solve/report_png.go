package solve

import (
	"bytes"
	"image/png"

	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

// internalPNGDpi — разрешение растровой картинки для отчёта.
const internalPNGDpi = 150

// internalResults — основной график одиночного режима (3 панели) в PDF.
func internalResults(po *plotOpts, outPath string, rGrid []float64,
	nTrue, nAvg, nAvgRaw, nLow, nHigh, vCum []float64, nBest [][]float64,
	nBestCount int, ch []channel, g1, gFit []float64, mAvgRe, mAvgIm float64) bool {
	drawFn, ok := internalResultsDraw(po, rGrid, nTrue, nAvg, nAvgRaw, nLow,
		nHigh, vCum, nBest, nBestCount, ch, g1, gFit, mAvgRe, mAvgIm)
	if !ok {
		return false
	}
	return internalRender(po, outPath, 8.6667*vg.Inch, 3.4667*vg.Inch, drawFn)
}

// internalResultsPNG — тот же график в PNG (для встраивания в отчёт).
func internalResultsPNG(po *plotOpts, rGrid []float64, nTrue, nAvg,
	nAvgRaw, nLow, nHigh, vCum []float64, nBest [][]float64, nBestCount int,
	ch []channel, g1, gFit []float64, mAvgRe, mAvgIm float64) ([]byte, bool) {
	drawFn, ok := internalResultsDraw(po, rGrid, nTrue, nAvg, nAvgRaw, nLow,
		nHigh, vCum, nBest, nBestCount, ch, g1, gFit, mAvgRe, mAvgIm)
	if !ok {
		return nil, false
	}
	return internalDrawToPNG(po, 8.6667*vg.Inch, 3.4667*vg.Inch, drawFn)
}

// internalDrawToPNG рисует в растр PNG (vgimg); паника не прерывает расчёт —
// печатается предупреждение, как у PDF-ветки.
func internalDrawToPNG(po *plotOpts, w, h vg.Length,
	drawFn func(draw.Canvas)) (out []byte, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			po.warnf("[warning] failed to build PNG plot: %v\n", r)
			out, ok = nil, false
		}
	}()
	c := vgimg.NewWith(vgimg.UseWH(w, h), vgimg.UseDPI(internalPNGDpi))
	drawFn(draw.NewCanvas(c, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, c.Image()); err != nil {
		po.warnf("[warning] failed to encode PNG plot: %v\n", err)
		return nil, false
	}
	return buf.Bytes(), true
}
