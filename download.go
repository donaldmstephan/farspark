package main

import (
	"bufio"
	"bytes"
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

func readAndCheckImage(res *http.Response) ([]byte, imageType, error) {
	nr := newNetReader(res.Body)

	if res.ContentLength > 0 {
		nr.GrowBuf(int(res.ContentLength))
	}

	b, err := nr.ReadAll()

	imgtype, err := checkTypeAndDimensions(b)
	if err != nil {
		return nil, UNKNOWN, err
	}

	return b, imgtype, err
}

func downloadImage(url string) ([]byte, imageType, error) {
	fullURL := fmt.Sprintf("%s%s", conf.BaseURL, url)

	res, err := downloadClient.Get(fullURL)
	if err != nil {
		return nil, UNKNOWN, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		body, _ := ioutil.ReadAll(res.Body)
		return nil, UNKNOWN, fmt.Errorf("Can't download image; Status: %d; %s", res.StatusCode, string(body))
	}

	return readAndCheckImage(res)
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
				fmt.Printf("Jump to range %s", v)
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
