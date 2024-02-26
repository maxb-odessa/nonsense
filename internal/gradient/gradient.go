package gradient

import (
	"fmt"
)

type Color struct {
	r, g, b int
}

type Gradient3 struct {
	color0, colorN, color100 Color
	colorNP                  float64
}

func linearInterp(start int, end int, percents float64) int {
	return int(start + (int)(percents*float64(end-start)))
}

func colorInterp(start Color, end Color, percents float64) *Color {
	return &Color{
		r: linearInterp(start.r, end.r, percents),
		g: linearInterp(start.g, end.g, percents),
		b: linearInterp(start.b, end.b, percents),
	}
}

func (c *Color) FromString(rgb string) {
	fmt.Sscanf(rgb, "#%02x%02x%02x", &c.r, &c.g, &c.b)
}

func (c *Color) FromRGB(r, g, b int) {
	c.r = r
	c.g = g
	c.b = b
}

func (g *Gradient3) Make(color0 string, colorN string, color100 string, colorNP float64) {
	g.color0.FromString(color0)
	g.colorN.FromString(colorN)
	g.color100.FromString(color100)
	g.colorNP = colorNP / 100.0
}

func (g *Gradient3) ColorAt(percents float64) *Color {
	perc := percents / 100.0
	if perc < g.colorNP {
		return colorInterp(g.color0, g.colorN, g.colorNP)
	} else if perc == g.colorNP {
		return &g.colorN
	} else {
		return colorInterp(g.colorN, g.color100, (perc-0.5)/0.5)
	}
}

func (c *Color) String() string {
	return fmt.Sprintf("#%02X%02X%02x", c.r, c.g, c.b)
}

func (c *Color) RGB() (int, int, int) {
	return c.r, c.g, c.b
}
