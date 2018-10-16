package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	_ "golang.org/x/image/webp"
)

var downloadClient *http.Client

type mimeType = string

type netReader struct {
	reader *bufio.Reader
	buf    *bytes.Buffer
}

func newNetReader(r io.Reader) *netReader {
	return &netReader{
		reader: bufio.NewReader(r),
		buf:    bytes.NewBuffer([]byte{}),
	}
}

func (r *netReader) ReadAll() ([]byte, error) {
	if _, err := r.buf.ReadFrom(r.reader); err != nil {
		return []byte{}, err
	}
	return r.buf.Bytes(), nil
}

func (r *netReader) GrowBuf(s int) {
	r.buf.Grow(s)
}

func initDownloading() {
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DisableKeepAlives:   true,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
	}
	downloadClient = &http.Client{
		Timeout:   time.Duration(conf.DownloadTimeout) * time.Second,
		Transport: transport,
	}
}

func readAndCheckMediaResponse(res *http.Response) ([]byte, error) {
	nr := newNetReader(res.Body)

	if res.ContentLength > 0 {
		nr.GrowBuf(int(res.ContentLength))
	}

	return nr.ReadAll()
}

func shouldCacheMimeType(t mimeType) bool {
	// For now, just cache PDF files locally since we re-fetch new pages over and over,
	// and they tend to be big files
	if t == "application/pdf" {
		return true
	}

	return false
}

func downloadMedia(url string) ([]byte, mimeType, error) {
	sha256 := sha256.New()
	sha256.Write([]byte(url))
	sha256.Write([]byte("src"))
	srcCacheKey := base64.URLEncoding.EncodeToString(sha256.Sum(nil))

	if farsparkCache != nil && farsparkCache.Has(srcCacheKey) {
		bytes, err := farsparkCache.Read(srcCacheKey)
		if err != nil {
			return nil, "", err
		}

		return bytes, http.DetectContentType(bytes), err
	} else {
		res, err := downloadClient.Get(url)
		if err != nil {
			return nil, "", err
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			body, _ := ioutil.ReadAll(res.Body)
			return nil, "", fmt.Errorf("Can't download media; Status: %d; %s", res.StatusCode, string(body))
		}

		bytes, err := readAndCheckMediaResponse(res)
		if err != nil {
			return nil, "", err
		}

		mimeType := http.DetectContentType(bytes)
		if err == nil && shouldCacheMimeType(mimeType) && farsparkCache != nil {
			farsparkCache.Write(srcCacheKey, bytes)
		}

		return bytes, mimeType, err
	}
}

func streamMedia(url string, incomingRequest *http.Request) (*http.Response, error) {
	outgoingRequest, err := http.NewRequest(incomingRequest.Method, url, nil)

	if err != nil {
		return nil, err
	}

	for headerName, headerValue := range incomingRequest.Header {
		for _, v := range headerValue {
			if headerName == "Range" {
				outgoingRequest.Header.Add(headerName, v)
			}
		}
	}

	res, err := downloadClient.Do(outgoingRequest)
	if err != nil {
		return nil, err
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()

		return nil, fmt.Errorf("Can't stream media; Status: %d", res.StatusCode)
	}

	return res, nil
}
