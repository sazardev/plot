package export

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-pdf/fpdf"

	"github.com/sazardev/plot/internal/termtheme"
)

// Options control the rasterized screenshot. Palette and font come from the
// terminal/system where possible, with embedded fallbacks for non-TTY runs.
type Options struct {
	Palette  termtheme.Palette
	FontPath string
	FontSize float64
}

// DefaultOptions are used by tests and non-interactive exports.
func DefaultOptions() Options {
	return Options{
		Palette:  termtheme.DefaultPalette(),
		FontSize: 16,
	}
}

type format string

const (
	FormatPNG  format = "png"
	FormatJPEG format = "jpg"
	FormatPDF  format = "pdf"
)

// FormatForPath validates an export path and returns the image format selected
// by its extension.
func FormatForPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("export path is empty")
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return string(FormatPNG), nil
	case ".jpg", ".jpeg":
		return string(FormatJPEG), nil
	case ".pdf":
		return string(FormatPDF), nil
	default:
		return "", fmt.Errorf("unsupported export format %q: use .png, .jpg, .jpeg or .pdf", filepath.Ext(path))
	}
}

// WriteFrame rasterizes a rendered chart frame with the default palette/font
// and writes it to path. The format comes from the file extension.
func WriteFrame(frame, path string) error {
	return WriteFrameWithOptions(frame, path, DefaultOptions())
}

// WriteFrameWithOptions rasterizes a rendered chart frame and writes it to
// path. The format comes from the file extension.
func WriteFrameWithOptions(frame, path string, options Options) error {
	selected, err := FormatForPath(path)
	if err != nil {
		return err
	}
	img, err := RenderFrameWithOptions(frame, options)
	if err != nil {
		return fmt.Errorf("render export: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	switch selected {
	case string(FormatPNG):
		return writePNG(img, path)
	case string(FormatJPEG):
		return writeJPEG(img, path)
	case string(FormatPDF):
		return writePDF(img, path)
	default:
		return fmt.Errorf("unsupported export format %q", selected)
	}
}

func writePNG(img image.Image, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	encoder := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := encoder.Encode(file, img); err != nil {
		return fmt.Errorf("encode PNG: %w", err)
	}
	return nil
}

func writeJPEG(img image.Image, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 92}); err != nil {
		return fmt.Errorf("encode JPEG: %w", err)
	}
	return nil
}

func writePDF(img image.Image, path string) error {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return fmt.Errorf("encode PDF image: %w", err)
	}
	orientation := "L"
	if img.Bounds().Dy() > img.Bounds().Dx() {
		orientation = "P"
	}
	pdf := fpdf.New(orientation, "pt", "A4", "")
	pdf.SetCompression(true)
	pdf.SetTitle(filepath.Base(path), true)
	pdf.SetCreator("pl", true)
	pdf.AddPage()
	pageWidth, pageHeight := pdf.GetPageSize()
	const margin = 24.0
	imageWidth := float64(img.Bounds().Dx())
	imageHeight := float64(img.Bounds().Dy())
	scale := (pageWidth - margin*2) / imageWidth
	if candidate := (pageHeight - margin*2) / imageHeight; candidate < scale {
		scale = candidate
	}
	width := imageWidth * scale
	height := imageHeight * scale
	x := (pageWidth - width) / 2
	y := (pageHeight - height) / 2
	pdf.RegisterImageOptionsReader("chart", fpdf.ImageOptions{ImageType: "PNG"}, &buffer)
	pdf.ImageOptions("chart", x, y, width, height, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	if err := pdf.OutputFileAndClose(path); err != nil {
		return fmt.Errorf("write PDF: %w", err)
	}
	return nil
}
