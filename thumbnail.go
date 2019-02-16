package main
import (
	"errors"
	"github.com/mqp/lilliput"
)

type OutputBuffer struct {
	buf []byte
	ops *lilliput.ImageOps
}

var EncodeOptions = map[mimeType]map[int]int{
	"image/gif": map[int]int{},
	"image/jpeg": map[int]int{lilliput.JpegQuality: 85},
	"image/png":  map[int]int{lilliput.PngCompression: 7},
}

// Map from output media type to Lilliput output file type identifier.
var outputFileTypes = map[mimeType]string{
	"image/gif": ".gif",
	"image/jpeg": ".jpeg",
	"image/png":  ".png",
}

var outputBufferPool = make(chan *OutputBuffer, conf.Concurrency)

func processImage(data []byte, outputFormat mimeType, width int, height int, t *timer) ([]byte, error) {
	decoder, err := lilliput.NewDecoder(data)
	if err != nil {
		return nil, errors.New("Error initializing image decoder")
	}
	defer decoder.Close()
	t.Check()

	header, err := decoder.Header()
	if err != nil {
		return nil, errors.New("Error reading image header")
	}
	imgWidth := header.Width()
	imgHeight := header.Height()

	if imgWidth > conf.MaxDimension || imgHeight > conf.MaxDimension {
		return nil, errors.New("Source image is too big")
	}
	if imgWidth * imgHeight > conf.MaxResolution {
		return nil, errors.New("Source image is too big")
	}
	t.Check()

	var outputBuffer *OutputBuffer
	select {
	case outputBuffer = <-outputBufferPool: // acquire from pool
	default: // pool is empty, create one
		outputBuffer = &OutputBuffer{
			buf: make([]byte, 50*1024*1024),
			ops: lilliput.NewImageOps(8192),
		}
	}
	t.Check()

	defer func() {
		outputBuffer.ops.Clear()
		select {
		case outputBufferPool <- outputBuffer: // release into pool
		default: // pool is full, throw out this one
		}
	}()

	opts := &lilliput.ImageOptions{
		FileType:             outputFileTypes[outputFormat],
		Width:                width,
		Height:               height,
		ResizeMethod:         lilliput.ImageOpsFit,
		NormalizeOrientation: true,
		EncodeOptions:        EncodeOptions[outputFormat],
	}
	return outputBuffer.ops.Transform(decoder, opts, outputBuffer.buf)
}
