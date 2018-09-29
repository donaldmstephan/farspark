package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	nanoid "github.com/matoous/go-nanoid"
)

var mimes = map[mediaType]string{
	JPEG: "image/jpeg",
	PNG:  "image/png",
	WEBP: "image/webp",
	GLTF: "model/gltf+json",
}

type httpHandler struct {
	sem chan struct{}
}

func newHTTPHandler() *httpHandler {
	return &httpHandler{make(chan struct{}, conf.Concurrency)}
}

func parsePath(r *http.Request) (string, processingOptions, error) {
	var po processingOptions
	var err error

	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	if len(parts) < 6 {
		return "", po, errors.New("Invalid path")
	}

	// path part 0 corresponds to signature of rest of path, which we no longer care about

	if r, ok := processingMethods[parts[1]]; ok {
		po.Method = r
	} else {
		return "", po, fmt.Errorf("Invalid resize type: %s", parts[1])
	}

	// path parts 2-4 correspond to obsolete image transformation options (width, height, enlarge)

	if po.Index, err = strconv.Atoi(parts[5]); err != nil {
		return "", po, fmt.Errorf("Invalid index: %s", parts[5])
	}

	filenameParts := strings.Split(strings.Join(parts[6:], ""), ".")

	if len(filenameParts) < 2 {
		po.Format = mediaTypes["JPG"]
	} else if f, ok := mediaTypes[strings.ToUpper(filenameParts[1])]; ok {
		po.Format = f
	} else {
		return "", po, fmt.Errorf("Invalid image format: %s", filenameParts[1])
	}

	filename, err := base64.RawURLEncoding.DecodeString(filenameParts[0])
	if err != nil {
		return "", po, errors.New("Invalid filename encoding")
	}

	return string(filename), po, nil
}

func logResponse(status int, msg string) {
	var color int

	if status >= 500 {
		color = 31
	} else if status >= 400 {
		color = 33
	} else {
		color = 32
	}

	log.Printf("|\033[7;%dm %d \033[0m| %s\n", color, status, msg)
}

func writeCORS(r *http.Request, rw http.ResponseWriter) {
	origin := r.Header.Get("origin")

	if len(conf.AllowOrigins) == 0 || len(origin) == 0 {
		return
	}

	allowedOrigin := "null"

	for _, nextOrigin := range conf.AllowOrigins {
		if nextOrigin == "*" || nextOrigin == origin {
			rw.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
			allowedOrigin = nextOrigin
			break
		}
	}
	rw.Header().Add("Vary", "Origin")
	rw.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
	rw.Header().Set("Access-Control-Expose-Headers", "Age, Date, Content-Length, Content-Range, X-Content-Duration, X-Content-Index, X-Max-Content-Index, X-Cache, X-Varnish")
}

func addCacheControlHeadersIfMissing(header http.Header) {
	if header.Get("Expires") == "" && header.Get("Cache-Control") == "" {
		header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", conf.TTL))
	}
}

func respondWithMedia(reqID string, r *http.Request, rw http.ResponseWriter, data []byte, mediaURL string, po processingOptions, duration time.Duration) {
	gzipped := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && conf.GZipCompression > 0

	addCacheControlHeadersIfMissing(rw.Header())
	rw.Header().Set("Content-Type", mimes[po.Format])

	dataToRespond := data

	if gzipped {
		var buf bytes.Buffer

		gz, _ := gzip.NewWriterLevel(&buf, conf.GZipCompression)
		gz.Write(data)
		gz.Close()

		dataToRespond = buf.Bytes()

		rw.Header().Set("Content-Encoding", "gzip")
	}

	rw.Header().Set("Content-Length", strconv.Itoa(len(dataToRespond)))

	rw.WriteHeader(200)
	rw.Write(dataToRespond)

	logResponse(200, fmt.Sprintf("[%s] Processed in %s: %s; %+v", reqID, duration, mediaURL, po))
}

func respondWithError(reqID string, rw http.ResponseWriter, err farsparkError) {
	logResponse(err.StatusCode, fmt.Sprintf("[%s] %s", reqID, err.Message))

	rw.WriteHeader(err.StatusCode)
	rw.Write([]byte(err.PublicMessage))
}

func (h *httpHandler) lock() {
	h.sem <- struct{}{}
}

func (h *httpHandler) unlock() {
	<-h.sem
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		if k == "set-cookie" || k == "set-cookie2" {
			continue
		}

		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func (h *httpHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	reqID, _ := nanoid.Nanoid()

	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(farsparkError); ok {
				respondWithError(reqID, rw, err)
			} else {
				respondWithError(reqID, rw, newUnexpectedError(r.(error), 4))
			}
		}
	}()

	log.Printf("[%s] %s: %s\n", reqID, r.Method, r.URL.RequestURI())

	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		panic(invalidMethodErr)
	}

	h.lock()
	defer h.unlock()

	if r.URL.Path == "/health" {
		rw.WriteHeader(200)
		rw.Write([]byte("farspark is running"))
		return
	}

	mediaURL, procOpt, err := parsePath(r)
	if err != nil {
		panic(newError(404, err.Error(), "Invalid media url"))
	}

	if _, err = url.ParseRequestURI(mediaURL); err != nil {
		panic(newError(404, err.Error(), "Invalid media url"))
	}

	if procOpt.Method != Raw {
		// Only allow HEAD and OPTIONS requests for raw URLs
		if r.Method == http.MethodHead || r.Method == http.MethodOptions {
			panic(invalidMethodErr)
		}

		var b []byte = nil
		var maxIndex int
		var mtype mediaType = UNKNOWN

		t := startTimer(time.Duration(conf.WriteTimeout)*time.Second, "Processing")

		contentsKey := getIndexContentsCacheKey(mediaURL, procOpt.Index)

		// Optimization: use the local page contents cache and skip download if possible
		if farsparkCache != nil && farsparkCache.Has(contentsKey) {
			outData, contentErr := farsparkCache.Read(contentsKey)
			maxIndexBytes, maxIndexErr := farsparkCache.Read(getMaxIndexCacheKey(mediaURL))
			maxIndexParsed, maxIndexParseErr := strconv.Atoi(string(maxIndexBytes))

			if contentErr == nil && maxIndexErr == nil && maxIndexParseErr == nil {
				b = outData
				mtype = PNG
				maxIndex = maxIndexParsed
			}
		}

		if b == nil {
			downloadBytes, downloadMediaType, err := downloadMedia(mediaURL)

			if err != nil {
				panic(newError(404, err.Error(), "Media is unreachable"))
			}

			b = downloadBytes
			mtype = downloadMediaType
		}

		t.Check()

		processedBytes, processedMaxIndex, err := processMedia(b, mediaURL, mtype, procOpt, t)

		if err != nil {
			panic(newError(500, err.Error(), "Error occurred while processing media"))
		}

		b = processedBytes

		if processedMaxIndex > maxIndex {
			maxIndex = processedMaxIndex
		}

		t.Check()

		writeCORS(r, rw)

		if maxIndex > 0 {
			rw.Header().Set("X-Content-Index", strconv.Itoa(procOpt.Index))
			rw.Header().Set("X-Max-Content-Index", strconv.Itoa(maxIndex))
		}

		respondWithMedia(reqID, r, rw, b, mediaURL, procOpt, t.Since())
	} else {
		res, err := streamMedia(mediaURL, r)

		if err != nil {
			panic(newError(500, err.Error(), "Error occurred while streaming media"))
		}

		defer res.Body.Close()
		body := res.Body

		isGLTF := res.Header.Get("Content-Type") == "model/gltf+json"
		expectBody := r.Method != http.MethodHead && r.Method != http.MethodOptions
		shouldRewrite := conf.ServerURL != nil
		if isGLTF && expectBody && shouldRewrite {
			contents, err := ioutil.ReadAll(body)
			if err != nil {
				panic(newError(500, err.Error(), "Error occurred while reading content"))
			}
			baseURL, err := url.Parse(mediaURL)
			if err != nil {
				panic(newError(500, err.Error(), "Invalid GLTF base URL"))
			}
			transformed, err := processGLTF(contents, baseURL, conf.ServerURL)
			if err != nil {
				panic(newError(500, err.Error(), "Error occurred while transforming GLTF"))
			}
			body = ioutil.NopCloser(bytes.NewReader(transformed))
		}

		copyHeader(rw.Header(), res.Header)
		addCacheControlHeadersIfMissing(rw.Header()) // If origin has no cache control, we assume farspark CDN will cache.
		writeCORS(r, rw)
		rw.WriteHeader(res.StatusCode)
		io.Copy(rw, body)
	}
}
