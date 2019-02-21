package main

import (
	"flag"
	"fmt"
	"github.com/peterbourgon/diskv"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func intEnvConfig(i *int, name string) {
	if env, err := strconv.Atoi(os.Getenv(name)); err == nil {
		*i = env
	}
}

func megaIntEnvConfig(f *int, name string) {
	if env, err := strconv.ParseFloat(os.Getenv(name), 64); err == nil {
		*f = int(env * 1000000)
	}
}

func urlEnvConfig(u **url.URL, name string) {
	if env, present := os.LookupEnv(name); present {
		if url, err := url.Parse(env); err == nil {
			*u = url
		}
	}
}

func strEnvConfig(s *string, name string) {
	if env := os.Getenv(name); len(env) > 0 {
		*s = env
	}
}

func strSliceEnvConfig(s *[]string, name string) {
	if env := os.Getenv(name); len(env) > 0 {
		*s = strings.Split(env, ",")
	} else {
		*s = make([]string, 0)
	}
}

func boolEnvConfig(b *bool, name string) {
	*b = false
	if env, err := strconv.ParseBool(os.Getenv(name)); err == nil {
		*b = env
	}
}

type config struct {
	Bind            string
	ReadTimeout     int
	WaitTimeout     int
	WriteTimeout    int
	DownloadTimeout int
	TTL             int

	MaxDimension  int
	MaxResolution int

	GZipCompression int

	AllowOrigins []string

	CacheRoot string
	CacheSize int

	ServerURL *url.URL
}

var conf = config{
	Bind:             ":8080",
	ReadTimeout:      10,
	WriteTimeout:     10,
	DownloadTimeout:  5,
	TTL:              3600,
	MaxDimension:     2048,
	GZipCompression:  5,
}

var farsparkCache *diskv.Diskv

func initCache() {
	if conf.CacheSize > 0 {
		farsparkCache = diskv.New(diskv.Options{
			BasePath:     conf.CacheRoot,
			Transform:    func(s string) []string { return []string{} },
			CacheSizeMax: uint64(conf.CacheSize),
		})
	} else {
		farsparkCache = nil
	}
}

func init() {
	flag.Parse()

	if port := os.Getenv("PORT"); len(port) > 0 {
		conf.Bind = fmt.Sprintf(":%s", port)
	}

	strEnvConfig(&conf.Bind, "FARSPARK_BIND")
	intEnvConfig(&conf.ReadTimeout, "FARSPARK_READ_TIMEOUT")
	intEnvConfig(&conf.WriteTimeout, "FARSPARK_WRITE_TIMEOUT")
	intEnvConfig(&conf.DownloadTimeout, "FARSPARK_DOWNLOAD_TIMEOUT")

	intEnvConfig(&conf.TTL, "FARSPARK_TTL")

	intEnvConfig(&conf.MaxDimension, "FARSPARK_MAX_DIMENSION")

	intEnvConfig(&conf.GZipCompression, "FARSPARK_GZIP_COMPRESSION")

	strSliceEnvConfig(&conf.AllowOrigins, "FARSPARK_ALLOW_ORIGINS")

	strEnvConfig(&conf.CacheRoot, "FARSPARK_CACHE_ROOT")
	intEnvConfig(&conf.CacheSize, "FARSPARK_CACHE_SIZE")

	urlEnvConfig(&conf.ServerURL, "FARSPARK_SERVER_URL")

	if len(conf.Bind) == 0 {
		log.Fatalln("Bind address is not defined")
	}

	if conf.ReadTimeout <= 0 {
		log.Fatalf("Read timeout should be greater than 0, now - %d\n", conf.ReadTimeout)
	}

	if conf.WriteTimeout <= 0 {
		log.Fatalf("Write timeout should be greater than 0, now - %d\n", conf.WriteTimeout)
	}

	if conf.DownloadTimeout <= 0 {
		log.Fatalf("Download timeout should be greater than 0, now - %d\n", conf.DownloadTimeout)
	}

	if conf.TTL <= 0 {
		log.Fatalf("TTL should be greater than 0, now - %d\n", conf.TTL)
	}

	if conf.MaxDimension <= 0 {
		log.Fatalf("Max dimension should be greater than 0, now - %d\n", conf.MaxDimension)
	}

	if conf.GZipCompression < 0 {
		log.Fatalf("GZip compression should be greater than or quual to 0, now - %d\n", conf.GZipCompression)
	} else if conf.GZipCompression > 9 {
		log.Fatalf("GZip compression can't be greater than 9, now - %d\n", conf.GZipCompression)
	}

	initDownloading()
	initCache()
}
