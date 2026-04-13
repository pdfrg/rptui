// Package modals provides modal dialogs for the TUI
package modals

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	termimg "github.com/blacktop/go-termimg"
	"github.com/pdfrg/rptui/internal/config"
)

// Modal layout constants — must match View() layout exactly.
// These are used to compute absolute screen coordinates for image rendering.
const (
	galleryModalBorder = 1 // rounded border
	galleryModalPadH   = 2 // horizontal padding (left + right)
	galleryModalPadV   = 1 // vertical padding (top + bottom)
	galleryLinesAbove  = 3 // title/indicator + blank + blank before image
	galleryLinesBelow  = 4 // blank + source + blank + help
)

// GalleryMsg is sent when the gallery modal closes
type GalleryMsg struct {
	Closed bool
}

// GalleryImageLoadedMsg is sent when a gallery image finishes loading
type GalleryImageLoadedMsg struct {
	Index     int
	ImageData []byte
	Err       error
}

// GalleryRenderImageMsg is sent after a short delay to render gallery image
// via tea.Raw(). The delay ensures the modal View() renders first.
type GalleryRenderImageMsg struct {
	ImageStr string
}

// Gallery modal for viewing artist images
type Gallery struct {
	styles     *config.ThemeStyles
	urls       []string
	source     string
	currentIdx int
	termWidth  int
	termHeight int
	cellRatio  float64

	// Image state
	images  []image.Image // decoded images (nil until loaded)
	loading map[int]bool  // indices currently being fetched
	loaded  map[int]bool  // indices successfully loaded

	// Rendered escape sequence for current image
	renderedStr  string
	renderedW    int // display width in columns
	renderedH    int // display height in rows
	renderFailed bool
}

// NewGallery creates a new Gallery modal
func NewGallery(styles *config.ThemeStyles, urls []string, source string, termWidth, termHeight int, cellRatio float64) *Gallery {
	if cellRatio < 1.0 {
		cellRatio = 2.0
	}
	g := &Gallery{
		styles:     styles,
		urls:       urls,
		source:     source,
		currentIdx: 0,
		termWidth:  termWidth,
		termHeight: termHeight,
		cellRatio:  cellRatio,
		images:     make([]image.Image, len(urls)),
		loading:    make(map[int]bool),
		loaded:     make(map[int]bool),
	}
	return g
}

// PrefetchImages returns commands to load the current and adjacent images
func (g *Gallery) PrefetchImages() tea.Cmd {
	indices := g.prefetchIndices()
	var cmds []tea.Cmd
	for _, idx := range indices {
		if cmd := g.loadImageCmd(idx); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (g *Gallery) prefetchIndices() []int {
	n := len(g.urls)
	if n == 0 {
		return nil
	}
	indices := []int{g.currentIdx}
	if g.currentIdx > 0 {
		indices = append(indices, g.currentIdx-1)
	} else {
		indices = append(indices, n-1)
	}
	if g.currentIdx < n-1 {
		indices = append(indices, g.currentIdx+1)
	} else {
		indices = append(indices, 0)
	}
	return indices
}

func (g *Gallery) loadImageCmd(idx int) tea.Cmd {
	if g.loaded[idx] || g.loading[idx] || idx < 0 || idx >= len(g.urls) {
		return nil
	}
	g.loading[idx] = true
	url := g.urls[idx]
	return func() tea.Msg {
		client := &http.Client{Timeout: 15 * time.Second}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return GalleryImageLoadedMsg{Index: idx, Err: err}
		}
		req.Header.Set("User-Agent", "rptui/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return GalleryImageLoadedMsg{Index: idx, Err: err}
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return GalleryImageLoadedMsg{Index: idx, Err: err}
		}
		return GalleryImageLoadedMsg{Index: idx, ImageData: data}
	}
}

// HandleImageLoaded processes a loaded gallery image.
// Returns a tea.Cmd that renders the image via tea.Raw() after a delay.
func (g *Gallery) HandleImageLoaded(msg GalleryImageLoadedMsg) tea.Cmd {
	g.loading[msg.Index] = false
	if msg.Err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(msg.ImageData))
	if err != nil {
		return nil
	}
	g.images[msg.Index] = img
	g.loaded[msg.Index] = true
	if msg.Index == g.currentIdx {
		g.renderCurrentImage()
		return g.RenderImageCmd()
	}
	return nil
}

// Update handles messages
func (g *Gallery) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			return func() tea.Msg { return GalleryMsg{Closed: true} }
		case "left", "h", "p":
			return g.navigate(-1)
		case "right", "l", "n":
			return g.navigate(1)
		case "up", "k":
			return g.navigate(-1)
		case "down", "j":
			return g.navigate(1)
		}
	}
	return nil
}

func (g *Gallery) navigate(delta int) tea.Cmd {
	n := len(g.urls)
	if n == 0 {
		return nil
	}
	g.currentIdx = (g.currentIdx + delta + n) % n
	g.renderedStr = ""
	g.renderFailed = false
	g.renderCurrentImage()

	// Batch: render image (if loaded) + prefetch adjacent
	var cmds []tea.Cmd
	if g.renderedStr != "" {
		cmds = append(cmds, g.RenderImageCmd())
	}
	if prefetch := g.PrefetchImages(); prefetch != nil {
		cmds = append(cmds, prefetch)
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (g *Gallery) renderCurrentImage() {
	img := g.images[g.currentIdx]
	if img == nil {
		return
	}
	termimg.ClearResizeCache()

	maxW, maxH := g.maxImageSize()
	if maxW < 4 || maxH < 2 {
		return
	}

	imgBounds := img.Bounds()
	imgW := float64(imgBounds.Dx())
	imgH := float64(imgBounds.Dy())

	// Calculate display size preserving aspect ratio
	displayWidth := maxW
	displayHeight := int(float64(displayWidth) * (imgH / imgW) / g.cellRatio)

	// If height exceeds max, scale down from height
	if displayHeight > maxH {
		displayHeight = maxH
		displayWidth = int(float64(displayHeight) * (imgW / imgH) * g.cellRatio)
	}

	if displayWidth < 4 {
		displayWidth = 4
	}
	if displayHeight < 2 {
		displayHeight = 2
	}

	// Don't upscale beyond native resolution.
	// displayWidth/Height are in character cells; source is in pixels.
	// Convert cells to pixels using font metrics, then clamp.
	nativeW := imgBounds.Dx()
	nativeH := imgBounds.Dy()
	features := termimg.QueryTerminalFeatures()
	if features.FontWidth > 0 && features.FontHeight > 0 {
		maxNativeCols := nativeW / features.FontWidth
		maxNativeRows := nativeH / features.FontHeight
		if displayWidth > maxNativeCols || displayHeight > maxNativeRows {
			// Scale down to fit native resolution
			if displayWidth > maxNativeCols {
				scale := float64(maxNativeCols) / float64(displayWidth)
				displayWidth = maxNativeCols
				displayHeight = int(float64(displayHeight) * scale)
			}
			if displayHeight > maxNativeRows {
				scale := float64(maxNativeRows) / float64(displayHeight)
				displayHeight = maxNativeRows
				displayWidth = int(float64(displayWidth) * scale)
			}
		}
	}

	tiImg := termimg.New(img).
		Size(displayWidth, displayHeight).
		Scale(termimg.ScaleFit).
		Protocol(termimg.Auto)

	rendered, err := tiImg.Render()
	if err != nil {
		g.renderFailed = true
		return
	}
	g.renderedStr = rendered
	g.renderedW = displayWidth
	g.renderedH = displayHeight
}

// maxImageSize returns max width and height in character cells for the image
func (g *Gallery) maxImageSize() (int, int) {
	modalWidth := g.modalWidth()
	contentWidth := modalWidth - galleryModalBorder*2 - galleryModalPadH
	w := contentWidth - 4 // leave some margin inside content area

	// Image area height: full terminal minus non-image lines
	h := g.termHeight - galleryModalBorder*2 - galleryModalPadV*2 - galleryLinesAbove - galleryLinesBelow
	if w < 10 {
		w = 10
	}
	if h < 4 {
		h = 4
	}
	return w, h
}

func (g *Gallery) modalWidth() int {
	return g.termWidth
}

// ImageScreenPosition returns the absolute screen row and column (1-indexed)
// where the gallery image should be drawn.
func (g *Gallery) ImageScreenPosition() (int, int) {
	modalWidth := g.modalWidth()
	contentWidth := modalWidth - galleryModalBorder*2 - galleryModalPadH

	// Modal centering
	modalHeight := g.modalLineCount() + galleryModalBorder*2 + galleryModalPadV*2
	padTop := (g.termHeight - modalHeight) / 2
	if padTop < 0 {
		padTop = 0
	}
	padLeft := (g.termWidth - modalWidth) / 2
	if padLeft < 0 {
		padLeft = 0
	}

	// Image row: modal top + border + padding + lines above image area (1-indexed)
	row := padTop + galleryModalBorder + galleryModalPadV + galleryLinesAbove + 1

	// Image col: modal left + border + padding + centering offset (1-indexed)
	imgPadLeft := (contentWidth - g.renderedW) / 2
	if imgPadLeft < 0 {
		imgPadLeft = 0
	}
	col := padLeft + galleryModalBorder + galleryModalPadH + imgPadLeft + 1

	return row, col
}

// modalLineCount returns the number of content lines in the modal (inside border+padding)
func (g *Gallery) modalLineCount() int {
	// blank + title + blank + indicator + blank + imageArea + blank + source + blank + help
	return galleryLinesAbove + g.imageAreaHeight() + galleryLinesBelow
}

// imageAreaHeight returns the fixed height of the image area in rows
func (g *Gallery) imageAreaHeight() int {
	_, maxH := g.maxImageSize()
	h := maxH
	if h < 3 {
		h = 3
	}
	return h
}

// RenderImageCmd returns a tea.Cmd that draws the current image at its
// computed screen position via tea.Raw(), after a short delay.
func (g *Gallery) RenderImageCmd() tea.Cmd {
	if g.renderedStr == "" {
		return nil
	}
	row, col := g.ImageScreenPosition()
	imgStr := g.renderedStr
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		clearStr := termimg.ClearAllString()
		raw := clearStr + fmt.Sprintf("\x1b[s\x1b[%d;%dH%s\x1b[u", row, col, imgStr)
		return GalleryRenderImageMsg{ImageStr: raw}
	})
}

// View renders the text-only modal content. Images are drawn separately
// via tea.Raw() at absolute screen coordinates (see RenderImageCmd).
func (g Gallery) View() string {
	modalWidth := g.modalWidth()
	contentWidth := modalWidth - galleryModalBorder*2 - galleryModalPadH
	imageAreaH := g.imageAreaHeight()

	accentStyle := g.styles.AccentStyle
	mutedStyle := g.styles.MutedStyle

	var b strings.Builder

	// Title + indicator on one line
	indicator := fmt.Sprintf("%d/%d", g.currentIdx+1, len(g.urls))
	titleLine := accentStyle.Render("ARTIST IMAGES") + " " + accentStyle.Render(indicator)
	b.WriteString(centerStyled(titleLine, contentWidth))
	b.WriteString("\n\n")

	// Fixed image area — shows status text on first line, actual image drawn via tea.Raw()
	if g.loading[g.currentIdx] {
		b.WriteString(centerStyled(mutedStyle.Render("Loading image..."), contentWidth))
	} else if g.renderFailed {
		b.WriteString(centerStyled(mutedStyle.Render("Failed to render image"), contentWidth))
	} else if g.renderedStr == "" {
		b.WriteString(centerStyled(mutedStyle.Render("Loading image..."), contentWidth))
	}
	// Pad remaining image area lines
	for i := 1; i < imageAreaH; i++ {
		b.WriteString("\n")
	}

	// Blank line before source
	b.WriteString("\n")

	// Source attribution
	if g.source != "" {
		sourceText := mutedStyle.Render("Source: " + g.source)
		b.WriteString(centerStyled(sourceText, contentWidth))
	}
	b.WriteString("\n")

	// Help text
	helpText := accentStyle.Render("←/→") + mutedStyle.Render(" navigate  ") +
		accentStyle.Render("esc") + mutedStyle.Render(" close")
	b.WriteString(centerStyled(helpText, contentWidth))

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(g.styles.Accent)).
		Padding(1, 2).
		Width(modalWidth)

	return modalStyle.Render(b.String())
}

// HasImages returns true if the gallery has at least one URL
func (g *Gallery) HasImages() bool {
	return len(g.urls) > 0
}

// ImageCount returns the number of images in the gallery
func (g *Gallery) ImageCount() int {
	return len(g.urls)
}
