package main

import (
	"bytes"
	"compress/gzip"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	nanoid "github.com/matoous/go-nanoid"
)

var mimes = map[imageType]string{
	JPEG: "image/jpeg",
	PNG:  "image/png",
	WEBP: "image/webp",
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

	token := parts[0]

	if err = validatePath(token, strings.TrimPrefix(path, fmt.Sprintf("/%s", token))); err != nil {
		return "", po, err
	}

	if r, ok := processingMethods[parts[1]]; ok {
		po.Method = r
	} else {
		return "", po, fmt.Errorf("Invalid resize type: %s", parts[1])
	}

	if po.Width, err = strconv.Atoi(parts[2]); err != nil {
		return "", po, fmt.Errorf("Invalid width: %s", parts[2])
	}

	if po.Height, err = strconv.Atoi(parts[3]); err != nil {
		return "", po, fmt.Errorf("Invalid height: %s", parts[3])
	}

	po.Enlarge = parts[4] != "0"

	if po.Index, err = strconv.Atoi(parts[5]); err != nil {
		return "", po, fmt.Errorf("Invalid index: %s", parts[5])
	}

	filenameParts := strings.Split(strings.Join(parts[6:], ""), ".")

	if len(filenameParts) < 2 {
		po.Format = imageTypes["JPG"]
	} else if f, ok := imageTypes[strings.ToUpper(filenameParts[1])]; ok {
		po.Format = f
	} else {
		return "", po, fmt.Errorf("Invalid image format: %s", filenameParts[1])
	}

	if !lilliputSupportSave[po.Format] {
		return "", po, errors.New("Resulting image type not supported")
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
		if nextOrigin == origin {
			rw.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
			allowedOrigin = origin
			break
		}
	}

	rw.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
}

func respondWithImage(reqID string, r *http.Request, rw http.ResponseWriter, data []byte, imgURL string, po processingOptions, duration time.Duration) {
	gzipped := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && conf.GZipCompression > 0

	rw.Header().Set("Expires", time.Now().Add(time.Second*time.Duration(conf.TTL)).Format(http.TimeFormat))
	rw.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d, public", conf.TTL))
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

	logResponse(200, fmt.Sprintf("[%s] Processed in %s: %s; %+v", reqID, duration, imgURL, po))
}

func respondWithError(reqID string, rw http.ResponseWriter, err farsparkError) {
	logResponse(err.StatusCode, fmt.Sprintf("[%s] %s", reqID, err.Message))

	rw.WriteHeader(err.StatusCode)
	rw.Write([]byte(err.PublicMessage))
}

func respondWithOptions(reqID string, rw http.ResponseWriter) {
	logResponse(200, fmt.Sprintf("[%s] Respond with options", reqID))
	rw.WriteHeader(200)
}

func checkSecret(s string) bool {
	if len(conf.Secret) == 0 {
		return true
	}
	return strings.HasPrefix(s, "Bearer ") && subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(s, "Bearer ")), []byte(conf.Secret)) == 1
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

	if r.Method == http.MethodOptions {
		respondWithOptions(reqID, rw)
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		panic(invalidMethodErr)
	}

	if !checkSecret(r.Header.Get("Authorization")) {
		panic(invalidSecretErr)
	}

	h.lock()
	defer h.unlock()

	if r.URL.Path == "/health" {
		rw.WriteHeader(200)
		rw.Write([]byte("farspark is running"))
		return
	}

	imgURL, procOpt, err := parsePath(r)
	if err != nil {
		panic(newError(404, err.Error(), "Invalid image url"))
	}

	if _, err = url.ParseRequestURI(imgURL); err != nil {
		panic(newError(404, err.Error(), "Invalid image url"))
	}

	if procOpt.Method != Raw {
		// Only allow HEAD requests for raw URLs
		if r.Method == http.MethodHead {
			panic(invalidMethodErr)
		}

		var b []byte = nil
		var maxIndex int
		var imgtype imageType = UNKNOWN

		t := startTimer(time.Duration(conf.WriteTimeout)*time.Second, "Processing")

		contentsKey := getIndexContentsCacheKey(imgURL, procOpt.Index)

		// Optimization: use the local page contents cache and skip download if possible
		if farsparkCache != nil && farsparkCache.Has(contentsKey) {
			outData, contentErr := farsparkCache.Read(contentsKey)
			maxIndexBytes, maxIndexErr := farsparkCache.Read(getMaxIndexCacheKey(imgURL))
			maxIndexParsed, maxIndexParseErr := strconv.Atoi(string(maxIndexBytes))

			if contentErr == nil && maxIndexErr == nil && maxIndexParseErr == nil {
				b = outData
				imgtype = PNG
				maxIndex = maxIndexParsed
			}
		}

		if b == nil {
			downloadBytes, downloadImageType, err := downloadImage(imgURL)

			if err != nil {
				panic(newError(404, err.Error(), "Image is unreachable"))
			}

			b = downloadBytes
			imgtype = downloadImageType
		}

		t.Check()

		if conf.ETagEnabled {
			eTag := calcETag(b, &procOpt)
			rw.Header().Set("ETag", eTag)

			if eTag == r.Header.Get("If-None-Match") {
				panic(notModifiedErr)
			}
		}

		t.Check()

		processedBytes, processedMaxIndex, err := processImage(b, imgURL, imgtype, procOpt, t)

		if err != nil {
			panic(newError(500, err.Error(), "Error occurred while processing image"))
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

		respondWithImage(reqID, r, rw, b, imgURL, procOpt, t.Since())
	} else {
		res, err := streamImage(imgURL, r)

		if err != nil {
			panic(newError(500, err.Error(), "Error occurred while streaming media"))
		}

		body := res.Body
		defer body.Close()
		copyHeader(rw.Header(), res.Header)
		writeCORS(r, rw)

		rw.WriteHeader(res.StatusCode)

		if r.Method == http.MethodGet {
			io.Copy(rw, body)
		}
	}
}
