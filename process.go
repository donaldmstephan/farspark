package main

/*
#cgo pkg-config: vips
#cgo LDFLAGS: -s -w
#include "vips.h"
*/
import "C"

import (
	"errors"
	"github.com/discordapp/lilliput"
)

type imageType int

const (
	UNKNOWN = C.UNKNOWN
	JPEG    = C.JPEG
	PNG     = C.PNG
	WEBP    = C.WEBP
	GIF     = C.GIF
)

var imageTypes = map[string]imageType{
	"jpeg": JPEG,
	"jpg":  JPEG,
	"png":  PNG,
	"webp": WEBP,
	"gif":  GIF,
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

var resizeTypes = map[string]lilliput.ImageOpsSizeMethod{
	"fit":  lilliput.ImageOpsFit,
	"fill": lilliput.ImageOpsResize,
	"crop": lilliput.ImageOpsNoResize,
}

type processingOptions struct {
	Resize  lilliput.ImageOpsSizeMethod
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

	// Ensure we won't crop out of bounds
	if !po.Enlarge || po.Resize == lilliput.ImageOpsNoResize {
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
		ResizeMethod:         po.Resize,
		NormalizeOrientation: true,
		EncodeOptions:        EncodeOptions[po.Format],
	}

	if outputImg, err = ops.Transform(decoder, opts, outputImg); err != nil {
		return nil, err
	}

	t.Check()

	return outputImg, nil
}
