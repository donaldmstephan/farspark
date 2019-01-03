package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"gopkg.in/alexcesaro/statsd.v2"
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

type processingMethod int

const (
	Unknown processingMethod = iota
	Raw
	Extract
	Thumbnail
)

var processingMethods = map[string]processingMethod{
	"extract":   Extract,
	"thumbnail": Thumbnail,
	"raw":       Raw,
}

type processingOptions struct {
	Method processingMethod
	Index  int
}

type thumbnailOptions struct {
	SourceURL string
	Width     int
	Height    int
}

type httpHandler struct {
	sem chan struct{}
}

func newHTTPHandler() *httpHandler {
	return &httpHandler{make(chan struct{}, conf.Concurrency)}
}

func parseEndpoint(r *http.Request) (processingMethod, error) {
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if r, ok := processingMethods[parts[0]]; ok {
		return r, nil
	} else if len(parts) >= 2 {
		if r, ok := processingMethods[parts[1]]; ok {
			return r, nil
		}
	}
	return Unknown, errors.New("Invalid endpoint.")
}

func parseThumbnailOptions(r *http.Request) (thumbnailOptions, error) {
	var opts thumbnailOptions
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// path part 0 corresponds to "thumbnail" endpoint

	filename, err := base64.RawURLEncoding.DecodeString(strings.Join(parts[1:], "/"))
	if err != nil {
		return opts, errors.New("Invalid filename encoding")
	}
	opts.SourceURL = string(filename);
	if _, err = url.ParseRequestURI(opts.SourceURL); err != nil {
		return opts, errors.New("Invalid media url")
	}

	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return opts, errors.New("Invalid query string")
	}

	if opts.Width, err = strconv.Atoi(query.Get("w")); err != nil {
		return opts, fmt.Errorf("Invalid width: %s", query.Get("w"))
	}

	if opts.Height, err = strconv.Atoi(query.Get("h")); err != nil {
		return opts, fmt.Errorf("Invalid height: %s", query.Get("h"))
	}

	if opts.Width > conf.MaxDimension || opts.Height > conf.MaxDimension {
		return opts, errors.New("Requested size is too big")
	}

	if opts.Width * opts.Height > conf.MaxResolution {
		return opts, errors.New("Requested size is too big")
	}

	return opts, nil
}

func parseLegacyOptions(r *http.Request) (string, processingOptions, error) {
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
		return "", po, fmt.Errorf("Invalid transformation type: %s", parts[1])
	}

	// path part 2-4 corresponds to obsolete image transformation options (width, height, enlarge)

	if po.Index, err = strconv.Atoi(parts[5]); err != nil {
		return "", po, fmt.Errorf("Invalid index: %s", parts[5])
	}

	filename, err := base64.RawURLEncoding.DecodeString(strings.Join(parts[6:], "/"))
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
		header.Set("Cache-Control", fmt.Sprintf("max-age=%d", conf.TTL))
	}
}

func respondWithMedia(reqID string, r *http.Request, rw http.ResponseWriter, data []byte, mediaURL string, mimeType string, duration time.Duration) {
	gzipped := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && conf.GZipCompression > 0

	addCacheControlHeadersIfMissing(rw.Header())
	rw.Header().Set("Content-Type", mimeType)

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

	logResponse(200, fmt.Sprintf("[%s] Processed in %s: %s; %+v", reqID, duration, mediaURL, r.URL))
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
		if k == "set-cookie" || k == "set-cookie2" || strings.HasPrefix(k, "x-amz") || strings.HasPrefix(k, "X-Amz") {
			continue
		}

		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func (h *httpHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	reqID, _ := nanoid.Nanoid()
	stats, err := statsd.New()
	defer stats.Close()

	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(farsparkError); ok {
				respondWithError(reqID, rw, err)
			} else {
				respondWithError(reqID, rw, newUnexpectedError(r.(error), 4))
			}

			stats.Increment("farspark.request_errors")
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

	endpoint, err := parseEndpoint(r);
	if err != nil {
		panic(newError(404, err.Error(), "Invalid endpoint specified"))
	}

	switch endpoint {
	case Thumbnail:
		opts, err := parseThumbnailOptions(r)
		if err != nil {
			panic(newError(400, fmt.Sprintf("Error: %+v", err), "Error parsing options"))
		}

		if r.Method != http.MethodGet {
			panic(invalidMethodErr)
		}
		t := startTimer(time.Duration(conf.WriteTimeout)*time.Second, "Processing")
		tThumbnail := stats.NewTiming()

		imageBytes, imageMimeType, err := downloadMedia(opts.SourceURL)
		if err != nil {
			panic(newError(404, fmt.Sprintf("Error: %+v", err), "Media is unreachable"))
		}

		outputBytes, err := processImage(imageBytes, imageMimeType, opts.Width, opts.Height, t)
		if err != nil {
			stats.Increment("farspark.thumbnail_errors")
			panic(newError(500, fmt.Sprintf("Error: %+v", err), "Error occurred while generating thumbnail"))
		}
		t.Check()

		writeCORS(r, rw)

		respondWithMedia(reqID, r, rw, outputBytes, opts.SourceURL, imageMimeType, t.Since())
		stats.Increment("farspark.thumbnail_ok")
		tThumbnail.Send("farspark.thumbnail_time")

	case Extract:
		mediaURL, procOpt, err := parseLegacyOptions(r)
		if err != nil {
			panic(newError(400, err.Error(), "Error parsing options"))
		}

		if r.Method != http.MethodGet {
			panic(invalidMethodErr)
		}

		var b []byte = nil
		var maxIndex int
		outputMimeType := "image/png"

		t := startTimer(time.Duration(conf.WriteTimeout)*time.Second, "Processing")
		tProcess := stats.NewTiming()

		contentsKey := getIndexContentsCacheKey(mediaURL, procOpt.Index)

		// Optimization: use the local page contents cache and skip download if possible
		if farsparkCache != nil && farsparkCache.Has(contentsKey) {
			outData, contentErr := farsparkCache.Read(contentsKey)
			maxIndexBytes, maxIndexErr := farsparkCache.Read(getMaxIndexCacheKey(mediaURL))
			maxIndexParsed, maxIndexParseErr := strconv.Atoi(string(maxIndexBytes))

			if contentErr == nil && maxIndexErr == nil && maxIndexParseErr == nil {
				b = outData
				maxIndex = maxIndexParsed
			}
		} else {
			downloadBytes, downloadMimeType, err := downloadMedia(mediaURL)

			if err != nil {
				panic(newError(404, err.Error(), "Media is unreachable"))
			}

			if downloadMimeType != "application/pdf" {
				panic(newError(400, err.Error(), "Media type has no subresources to extract"))
			}

			t.Check()

			processedBytes, processedMaxIndex, err := extractPDFPage(downloadBytes, mediaURL, procOpt.Index, outputMimeType)

			if err != nil {
				stats.Increment("farspark.process_errors")
				panic(newError(500, err.Error(), "Error occurred while processing media"))
			}

			b = processedBytes
			maxIndex = processedMaxIndex
		}

		t.Check()

		writeCORS(r, rw)

		if maxIndex > 0 {
			rw.Header().Set("X-Content-Index", strconv.Itoa(procOpt.Index))
			rw.Header().Set("X-Max-Content-Index", strconv.Itoa(maxIndex))
		}

		respondWithMedia(reqID, r, rw, b, mediaURL, outputMimeType, t.Since())
		stats.Increment("farspark.process_ok")
		tProcess.Send("farspark.process_time")
	case Raw:
		mediaURL, _, err := parseLegacyOptions(r)
		if err != nil {
			panic(newError(400, err.Error(), "Error parsing options"))
		}

		tRaw := stats.NewTiming()
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
			tGLTF := stats.NewTiming()
			contents, err := ioutil.ReadAll(body)
			if err != nil {
				stats.Increment("farspark.gltf_read_errors")
				panic(newError(500, err.Error(), "Error occurred while reading content"))
			}
			baseURL, err := url.Parse(mediaURL)
			if err != nil {
				panic(newError(500, err.Error(), "Invalid GLTF base URL"))
			}
			transformed, err := processGLTF(contents, baseURL, conf.ServerURL)
			if err != nil {
				stats.Increment("farspark.gltf_xform_errors")
				panic(newError(500, err.Error(), "Error occurred while transforming GLTF"))
			}
			body = ioutil.NopCloser(bytes.NewReader(transformed))
			tGLTF.Send("farspark.gltf_process_time")
			stats.Increment("farspark.gltf_process_ok")
		}

		copyHeader(rw.Header(), res.Header)
		rw.Header().Set("Server", "Farspark")
		addCacheControlHeadersIfMissing(rw.Header()) // If origin has no cache control, we assume farspark CDN will cache.
		writeCORS(r, rw)
		rw.WriteHeader(res.StatusCode)
		io.Copy(rw, body)
		stats.Increment("farspark.raw_ok")
		tRaw.Send("farspark.raw_time")
	}
}
