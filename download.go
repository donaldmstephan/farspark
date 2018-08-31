package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/discordapp/lilliput"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	_ "golang.org/x/image/webp"
)

var downloadClient *http.Client

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

func (r *netReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	if err == nil {
		r.buf.Write(p[:n])
	}
	return
}

func (r *netReader) Peek(n int) ([]byte, error) {
	return r.reader.Peek(n)
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
		Proxy:             http.ProxyFromEnvironment,
		DisableKeepAlives: true,
	}
	downloadClient = &http.Client{
		Timeout:   time.Duration(conf.DownloadTimeout) * time.Second,
		Transport: transport,
	}
}

func checkTypeAndDimensions(data []byte) (mediaType, error) {
	contentType := http.DetectContentType(data)

	if contentType == "application/pdf" {
		return PDF, nil
	}

	decoder, err := lilliput.NewDecoder(data)
	if err != nil {
		return UNKNOWN, err
	}

	defer decoder.Close()

	header, err := decoder.Header()
	if err != nil {
		return UNKNOWN, errors.New("Error reading image header")
	}

	imgWidth := header.Width()
	imgHeight := header.Height()
	mtypeStr := decoder.Description()

	mtype, mtypeOk := mediaTypes[mtypeStr]

	if err != nil {
		return UNKNOWN, err
	}
	if imgWidth > conf.MaxSrcDimension || imgHeight > conf.MaxSrcDimension {
		return UNKNOWN, errors.New("Source image is too big")
	}
	if imgWidth*imgHeight > conf.MaxSrcResolution {
		return UNKNOWN, errors.New("Source image is too big")
	}
	if !mtypeOk {
		return UNKNOWN, errors.New("Source image type not supported")
	}

	return mtype, nil
}

func readAndCheckMediaBytes(b []byte) ([]byte, mediaType, error) {
	mtype, err := checkTypeAndDimensions(b)
	if err != nil {
		return nil, UNKNOWN, err
	}

	return b, mtype, err
}

func readAndCheckMediaResponse(res *http.Response) ([]byte, mediaType, error) {
	nr := newNetReader(res.Body)

	if res.ContentLength > 0 {
		nr.GrowBuf(int(res.ContentLength))
	}

	b, err := nr.ReadAll()
	if err != nil {
		return nil, UNKNOWN, err
	}

	return readAndCheckMediaBytes(b)
}

func shouldCacheMediaType(t mediaType) bool {
	// For now, just cache PDF files locally since we re-fetch new pages over and over,
	// and they tend to be big files
	if t == PDF {
		return true
	}

	return false
}

func downloadMedia(url string) ([]byte, mediaType, error) {
	sha256 := sha256.New()
	sha256.Write([]byte(url))
	sha256.Write([]byte("src"))
	srcCacheKey := base64.URLEncoding.EncodeToString(sha256.Sum(nil))

	if farsparkCache != nil && farsparkCache.Has(srcCacheKey) {
		bytes, err := farsparkCache.Read(srcCacheKey)
		if err != nil {
			return nil, UNKNOWN, err
		}

		return readAndCheckMediaBytes(bytes)
	} else {
		res, err := downloadClient.Get(url)
		if err != nil {
			return nil, UNKNOWN, err
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			body, _ := ioutil.ReadAll(res.Body)
			return nil, UNKNOWN, fmt.Errorf("Can't download image; Status: %d; %s", res.StatusCode, string(body))
		}

		bytes, mediaType, err := readAndCheckMediaResponse(res)

		if err == nil && shouldCacheMediaType(mediaType) && farsparkCache != nil {
			farsparkCache.Write(srcCacheKey, bytes)
		}

		return bytes, mediaType, err
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
