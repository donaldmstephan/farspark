package main

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/discordapp/lilliput"
	"github.com/gfodor/go-ghostscript/ghostscript"
	"io/ioutil"
	"os"
	"rsc.io/pdf"
	"strconv"
	"sync"
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
	PDF
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
	"PDF":  PDF,
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

type processingMethod int

const (
	Fit processingMethod = iota
	Fill
	Extract
	Raw
)

var processingMethods = map[string]processingMethod{
	"fit":     Fit,
	"fill":    Fill,
	"extract": Extract,
	"raw":     Raw,
}

var resizeOpSizeMethods = map[processingMethod]lilliput.ImageOpsSizeMethod{
	Fit:     lilliput.ImageOpsFit,
	Fill:    lilliput.ImageOpsResize,
	Extract: lilliput.ImageOpsNoResize,
}

type processingOptions struct {
	Method  processingMethod
	Width   int
	Height  int
	Enlarge bool
	Format  imageType
	Index   int
}

type OutputBuffer struct {
	buf []byte
	ops *lilliput.ImageOps
}

var outputBufferPool = make(chan *OutputBuffer, conf.Concurrency)
var gsMutex = &sync.Mutex{}
var gs *ghostscript.Ghostscript = nil

func getIndexCacheKey(url string, index int, suffix string) string {
	sha256 := sha256.New()
	sha256.Write([]byte(url))
	sha256.Write([]byte(fmt.Sprintf("%d", index)))
	sha256.Write([]byte(suffix))
	return base64.URLEncoding.EncodeToString(sha256.Sum(nil))
}

func getIndexContentsCacheKey(url string, index int) string {
	return getIndexCacheKey(url, index, "contents")
}

func getMaxIndexCacheKey(url string) string {
	return getIndexCacheKey(url, 0, "max_index")
}

func extractPDFPage(data []byte, url string, index int) ([]byte, int, error) {
	scratchDir, err := ioutil.TempDir("", "farspark-scratch")

	if err != nil {
		return nil, 0, errors.New("Error creating scratch dir")
	}

	defer os.RemoveAll(scratchDir)

	inFile := fmt.Sprintf("%s/in.pdf", scratchDir)
	outFile := fmt.Sprintf("%s/out.png", scratchDir)

	if err := ioutil.WriteFile(inFile, data, 0600); err != nil {
		return nil, 0, errors.New("Error writing temporary PDF file")
	}

	gsMutex.Lock()

	if gs == nil {
		_, err = ghostscript.GetRevision()

		if err != nil {
			gsMutex.Unlock()
			return nil, 0, err
		}

		gsPtr, err := ghostscript.NewInstance()
		if err != nil {
			gsMutex.Unlock()
			return nil, 0, err
		}

		gs = gsPtr
	}

	args := []string{
		"gs",
		"-sDEVICE=pngalpha",
		fmt.Sprintf("-sOutputFile=%s", outFile),
		fmt.Sprintf("-dFirstPage=%d", index+1),
		fmt.Sprintf("-dLastPage=%d", index+1),
		"-dNOPAUSE",
		"-r144",
		inFile,
	}

	if err := gs.Init(args); err != nil {
		gsMutex.Unlock()

		return nil, 0, err
	}

	gs.Exit()
	gsMutex.Unlock()

	pdfInst, _ := pdf.Open(inFile)
	if err != nil {
		return nil, 0, err
	}

	maxIndex := pdfInst.NumPage() - 1

	outFilePtr, err := os.Open(outFile)
	defer outFilePtr.Close()

	if err != nil {
		return nil, 0, err
	}

	outBytes, err := ioutil.ReadAll(outFilePtr)

	if err == nil && farsparkCache != nil {
		contentsCacheKey := getIndexContentsCacheKey(url, index)
		maxIndexCacheKey := getMaxIndexCacheKey(url)

		farsparkCache.Write(contentsCacheKey, outBytes)
		farsparkCache.Write(maxIndexCacheKey, []byte(strconv.Itoa(maxIndex)))
	}

	return outBytes, maxIndex, err
}

func processImage(data []byte, url string, imgtype imageType, po processingOptions, t *timer) ([]byte, int, error) {
	defer keepAlive(data)

	var imageData = data
	var maxIndex = 1

	if imgtype == PDF {
		pdfImageData, extractedPageCount, err := extractPDFPage(data, url, po.Index)

		if err != nil {
			return nil, 0, err
		}

		imageData = pdfImageData
		maxIndex = extractedPageCount
	}

	t.Check()

	decoder, err := lilliput.NewDecoder(imageData)
	defer decoder.Close()

	header, err := decoder.Header()

	if err != nil {
		return nil, 0, errors.New("Error reading image header")
	}

	imgWidth := header.Width()
	imgHeight := header.Height()

	t.Check()

	var outputBuffer *OutputBuffer

	select {
	case outputBuffer = <-outputBufferPool:
	default:
		outputBuffer = &OutputBuffer{
			buf: make([]byte, 50*1024*1024),
			ops: lilliput.NewImageOps(8192),
		}
	}

	t.Check()

	ops := outputBuffer.ops
	outputImg := outputBuffer.buf

	defer func() {
		ops.Clear()
		outputBufferPool <- outputBuffer
	}()

	imageResizeOp := resizeOpSizeMethods[po.Method]

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
		return nil, 0, err
	}

	t.Check()

	return outputImg, maxIndex, nil
}
