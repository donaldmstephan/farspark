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
		Proxy: http.ProxyFromEnvironment,
	}
	if conf.LocalFileSystemRoot != "" {
		transport.RegisterProtocol("local", http.NewFileTransport(http.Dir(conf.LocalFileSystemRoot)))
	}
	downloadClient = &http.Client{
		Timeout:   time.Duration(conf.DownloadTimeout) * time.Second,
		Transport: transport,
	}
}

func checkTypeAndDimensions(data []byte) (imageType, error) {
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
	imgtypeStr := decoder.Description()

	imgtype, imgtypeOk := imageTypes[imgtypeStr]

	if err != nil {
		return UNKNOWN, err
	}
	if imgWidth > conf.MaxSrcDimension || imgHeight > conf.MaxSrcDimension {
		return UNKNOWN, errors.New("Source image is too big")
	}
	if imgWidth*imgHeight > conf.MaxSrcResolution {
		return UNKNOWN, errors.New("Source image is too big")
	}
	if !imgtypeOk {
		return UNKNOWN, errors.New("Source image type not supported")
	}

	return imgtype, nil
}

func readAndCheckImageBytes(b []byte) ([]byte, imageType, error) {
	imgtype, err := checkTypeAndDimensions(b)
	if err != nil {
		return nil, UNKNOWN, err
	}

	return b, imgtype, err
}

func readAndCheckImageResponse(res *http.Response) ([]byte, imageType, error) {
	nr := newNetReader(res.Body)

	if res.ContentLength > 0 {
		nr.GrowBuf(int(res.ContentLength))
	}

	b, err := nr.ReadAll()
	if err != nil {
		return nil, UNKNOWN, err
	}

	return readAndCheckImageBytes(b)
}

func shouldCacheImageType(t imageType) bool {
	// For now, just cache PDF files locally since we re-fetch new pages over and over,
	// and they tend to be big files
	if t == PDF {
		return true
	}

	return false
}

func downloadImage(url string) ([]byte, imageType, error) {
	fullURL := fmt.Sprintf("%s%s", conf.BaseURL, url)
	sha256 := sha256.New()
	sha256.Write([]byte(fullURL))
	sha256.Write([]byte("src"))
	srcCacheKey := base64.URLEncoding.EncodeToString(sha256.Sum(nil))

	if farsparkCache != nil && farsparkCache.Has(srcCacheKey) {
		bytes, err := farsparkCache.Read(srcCacheKey)
		if err != nil {
			return nil, UNKNOWN, err
		}

		return readAndCheckImageBytes(bytes)
	} else {
		res, err := downloadClient.Get(fullURL)
		if err != nil {
			return nil, UNKNOWN, err
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			body, _ := ioutil.ReadAll(res.Body)
			return nil, UNKNOWN, fmt.Errorf("Can't download image; Status: %d; %s", res.StatusCode, string(body))
		}

		bytes, imageType, err := readAndCheckImageResponse(res)

		if err == nil && shouldCacheImageType(imageType) && farsparkCache != nil {
			farsparkCache.Write(srcCacheKey, bytes)
		}

		return bytes, imageType, err
	}
}

func streamImage(url string, incomingRequest *http.Request) (*http.Response, error) {
	fullURL := fmt.Sprintf("%s%s", conf.BaseURL, url)

	outgoingRequest, err := http.NewRequest("GET", fullURL, nil)

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
