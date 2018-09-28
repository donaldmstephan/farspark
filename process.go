package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/discordapp/lilliput"
	"github.com/gfodor/go-ghostscript/ghostscript"
	"io/ioutil"
	"net/url"
	"os"
	"rsc.io/pdf"
	"strconv"
	"sync"
)

type mediaType int

const (
	UNKNOWN mediaType = iota
	JPEG
	PNG
	WEBP
	GIF
	PDF
	GLTF
)

// Map from URL extension to inferred output media type.
var mediaTypes = map[string]mediaType{
	"JPG":  JPEG,
	"JPEG": JPEG,
	"PNG":  PNG,
	"GLTF": GLTF,
}

// Map from output media type to Lilliput output file type identifier.
var outputFileTypes = map[mediaType]string{
	JPEG: ".jpeg",
	PNG:  ".png",
}

// Map from output media type to Ghostscript output device identifier.
var outputFileDevices = map[mediaType]string{
	JPEG: "jpeg",
	PNG:  "png16m",
}

var EncodeOptions = map[mediaType]map[int]int{
	JPEG: map[int]int{lilliput.JpegQuality: 85},
	PNG:  map[int]int{lilliput.PngCompression: 7},
}

type processingMethod int

const (
	Raw processingMethod = iota
	Extract
)

var processingMethods = map[string]processingMethod{
	"extract": Extract,
	"raw":     Raw,
}

type processingOptions struct {
	Method processingMethod
	Format mediaType
	Index  int
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

func extractPDFPage(data []byte, url string, po processingOptions) ([]byte, int, error) {
	scratchDir, err := ioutil.TempDir("", "farspark-scratch")

	if err != nil {
		return nil, 0, errors.New("Error creating scratch dir")
	}

	defer os.RemoveAll(scratchDir)

	inFile := fmt.Sprintf("%s/in.pdf", scratchDir)
	outFile := fmt.Sprintf("%s/out", scratchDir)

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
		fmt.Sprintf("-sDEVICE=%s", outputFileDevices[po.Format]),
		fmt.Sprintf("-sOutputFile=%s", outFile),
		fmt.Sprintf("-dFirstPage=%d", po.Index+1),
		fmt.Sprintf("-dLastPage=%d", po.Index+1),
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
		contentsCacheKey := getIndexContentsCacheKey(url, po.Index)
		maxIndexCacheKey := getMaxIndexCacheKey(url)

		farsparkCache.Write(contentsCacheKey, outBytes)
		farsparkCache.Write(maxIndexCacheKey, []byte(strconv.Itoa(maxIndex)))
	}

	return outBytes, maxIndex, err
}

func generateFarsparkURL(targetURL *url.URL, serverURL *url.URL) (*url.URL, error) {
	path, err := url.Parse("/0/raw/0/0/0/0/" + base64.RawURLEncoding.EncodeToString([]byte(targetURL.String())))
	if err != nil {
		return nil, err
	}
	return serverURL.ResolveReference(path), nil
}

func transformSubresourceURL(subresourceURL *url.URL, baseURL *url.URL, serverURL *url.URL) (*url.URL, error) {
	targetURL := baseURL.ResolveReference(subresourceURL)
	return generateFarsparkURL(targetURL, serverURL)
}

func processGLTF(data []byte, baseURL *url.URL, serverURL *url.URL) ([]byte, error) {
	var model map[string]interface{}
	err := json.Unmarshal(data, &model)
	if err != nil {
		return nil, err
	}

	switch images := model["images"].(type) {
	case []interface{}:
		for _, v := range images {
			image := v.(map[string]interface{})
			oldURL, err := url.Parse(image["uri"].(string))
			if err != nil {
				return nil, err
			}
			newURL, err := transformSubresourceURL(oldURL, baseURL, serverURL)
			if err != nil {
				return nil, err
			}
			image["uri"] = newURL.String()
		}
	}

	switch buffers := model["buffers"].(type) {
	case []interface{}:
		for _, v := range buffers {
			buffer := v.(map[string]interface{})
			oldURL, err := url.Parse(buffer["uri"].(string))
			if err != nil {
				return nil, err
			}
			newURL, err := transformSubresourceURL(oldURL, baseURL, serverURL)
			if err != nil {
				return nil, err
			}
			buffer["uri"] = newURL.String()
		}
	}

	result, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func processImage(data []byte, po processingOptions, t *timer) ([]byte, error) {

	decoder, err := lilliput.NewDecoder(data)
	defer decoder.Close()

	header, err := decoder.Header()

	if err != nil {
		return nil, errors.New("Error reading image header")
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

	opts := &lilliput.ImageOptions{
		FileType:             outputFileTypes[po.Format],
		Width:                imgWidth,
		Height:               imgHeight,
		ResizeMethod:         lilliput.ImageOpsNoResize,
		NormalizeOrientation: true,
		EncodeOptions:        EncodeOptions[po.Format],
	}

	if outputImg, err = ops.Transform(decoder, opts, outputImg); err != nil {
		return nil, err
	}

	t.Check()

	return outputImg, nil
}

func processMedia(data []byte, url string, mtype mediaType, po processingOptions, t *timer) ([]byte, int, error) {
	t.Check()

	switch mtype {
	case PDF:
		outputImg, maxIndex, err := extractPDFPage(data, url, po)
		return outputImg, maxIndex, err
	default:
		outputImg, err := processImage(data, po, t)
		return outputImg, 1, err
	}
}
