package main

import (
	"errors"
	"github.com/discordapp/lilliput"
)

type imageType int

const (
	UNKNOWN imageType = iota
	JPEG
	PNG
	WEBP
	GIF
	WEBM
	MP4
	MOV
	OGG
)

var imageTypes = map[string]imageType{
	"JPG":  JPEG,
	"JPEG": JPEG,
	"PNG":  PNG,
	"WEBP": WEBP,
	"MP4":  MP4,
	"MOV":  MOV,
	"GIF":  GIF,
	"WEBM": WEBM,
	"OGG":  OGG,
}

var outputFileTypes = map[imageType]string{
	JPEG: ".jpeg",
	PNG:  ".png",
	WEBP: ".webp",
	GIF:  ".gif",
}

var EncodeOptions = map[imageType]map[int]int{
	JPEG: map[int]int{lilliput.JpegQuality: 85},
	PNG:  map[int]int{lilliput.PngCompression: 7},
	WEBP: map[int]int{lilliput.WebpQuality: 85},
}

var lilliputSupportSave = map[imageType]bool{
	JPEG: true,
	PNG:  true,
	GIF:  true,
	WEBP: true,
}

type resizeType int

const (
	Fit resizeType = iota
	Fill
	None
	Raw
)

var resizeTypes = map[string]resizeType{
	"fit":  Fit,
	"fill": Fill,
	"none": None,
	"raw":  Raw,
}

var resizeOpSizeMethods = map[resizeType]lilliput.ImageOpsSizeMethod{
	Fit:  lilliput.ImageOpsFit,
	Fill: lilliput.ImageOpsResize,
	None: lilliput.ImageOpsNoResize,
}

type processingOptions struct {
	Resize  resizeType
	Width   int
	Height  int
	Enlarge bool
	Format  imageType
}

func processImage(data []byte, imgtype imageType, po processingOptions, t *timer) ([]byte, error) {
	defer keepAlive(data)

	t.Check()

	decoder, err := lilliput.NewDecoder(data)
	defer decoder.Close()

	header, err := decoder.Header()

	if err != nil {
		return nil, errors.New("Error reading image header")
	}

	imgWidth := header.Width()
	imgHeight := header.Height()

	t.Check()

	ops := lilliput.NewImageOps(8192)
	defer ops.Close()

	outputImg := make([]byte, 50*1024*1024)
	imageResizeOp := resizeOpSizeMethods[po.Resize]

	// Ensure we won't crop out of bounds
	if !po.Enlarge || imageResizeOp == lilliput.ImageOpsNoResize {
		if imgWidth < po.Width {
			po.Width = imgWidth
		}

		if imgHeight < po.Height {
			po.Height = imgHeight
		}
	}

	opts := &lilliput.ImageOptions{
		FileType:             outputFileTypes[po.Format],
		Width:                po.Width,
		Height:               po.Height,
		ResizeMethod:         imageResizeOp,
		NormalizeOrientation: true,
		EncodeOptions:        EncodeOptions[po.Format],
	}

	if outputImg, err = ops.Transform(decoder, opts, outputImg); err != nil {
		return nil, err
	}

	t.Check()

	return outputImg, nil
}
