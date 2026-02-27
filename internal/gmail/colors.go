package gmail

type colorPair struct {
	Background string
	Text       string
}

// Fixed colors for default labels — semantically chosen.
var defaultLabelColors = map[string]colorPair{
	"Action Required": {"#cc3a21", "#ffffff"},
	"Informational":   {"#4a86e8", "#ffffff"},
	"Newsletter":      {"#fad165", "#000000"},
	"Finance":         {"#16a766", "#ffffff"},
	"Scheduling":      {"#a479e2", "#ffffff"},
	"Personal":        {"#f691b3", "#000000"},
	"Automated":       {"#999999", "#ffffff"},
}

// Overflow palette for custom labels — visually distinct from defaults.
var overflowPalette = []colorPair{
	{"#ffad47", "#000000"},
	{"#3c78d8", "#ffffff"},
	{"#0b804b", "#ffffff"},
	{"#cf8933", "#ffffff"},
	{"#653e9b", "#ffffff"},
	{"#e66550", "#ffffff"},
	{"#2a9c68", "#ffffff"},
	{"#285bac", "#ffffff"},
}

// ColorForLabel returns the Gmail background and text color for a label.
// Default labels get fixed colors. Custom labels get a color from the overflow
// palette based on index (wraps around if more custom labels than palette entries).
func ColorForLabel(name string, customIndex int) (background, text string) {
	if c, ok := defaultLabelColors[name]; ok {
		return c.Background, c.Text
	}
	c := overflowPalette[customIndex%len(overflowPalette)]
	return c.Background, c.Text
}

// IsDefaultLabel returns true if the label name has a fixed default color.
func IsDefaultLabel(name string) bool {
	_, ok := defaultLabelColors[name]
	return ok
}
