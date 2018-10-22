package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gfodor/go-ghostscript/ghostscript"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"rsc.io/pdf"
	"strconv"
	"sync"
)

// Map from output MIME type to Ghostscript output device identifier.
var outputFileDevices = map[mimeType]string{
	"image/jpeg": "jpeg",
	"image/png":  "png16m",
}

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

func extractPDFPage(data []byte, url string, index int, outputFormat string) ([]byte, int, error) {
	scratchDir, err := ioutil.TempDir("", "farspark-scratch")

	if err != nil {
		return nil, 0, errors.New("Error creating scratch dir")
	}

	defer os.RemoveAll(scratchDir)

	inFile := fmt.Sprintf("%s/in.pdf", scratchDir)
	outFile := fmt.Sprintf("%s/out", scratchDir)

	log.Printf("Preparing to extract page from %s into %s\n", inFile, outFile)

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
		fmt.Sprintf("-sDEVICE=%s", outputFileDevices[outputFormat]),
		fmt.Sprintf("-sOutputFile=%s", outFile),
		fmt.Sprintf("-dFirstPage=%d", index),
		fmt.Sprintf("-dLastPage=%d", index),
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
