# farspark

Farspark is a fast and secure standalone server, based on [imgproxy](https://github.com/DarthSim/imgproxy) for resizing, converting, and proxying remote media. The main principles of are simplicity, speed, and security. Farspark is used with [hubs](https://github.com/mozilla/hubs) in order to proxy and convert shared media assets such as images, videos, and GLTF assets.

Farspark can be used to provide a fast and secure way to replace all the image resizing code of your web application (like calling ImageMagick or GraphicsMagick, or using libraries), while also being able to resize everything on the fly, fast and easy. farspark is also indispensable when handling lots of image resizing, especially when images come from a remote source.

Farspark is a Go application, ready to be installed and used in any Unix environment — also ready to be containerized using Docker.

See this article for a good intro and all the juicy details! [farspark:
Resize your images instantly and securely](https://evilmartians.com/chronicles/introducing-farspark)

<a href="https://evilmartians.com/?utm_source=farspark">
<img src="https://evilmartians.com/badges/sponsored-by-evil-martians.svg" alt="Sponsored by Evil Martians" width="236" height="54">
</a>

#### Simplicity

> "No code is better than no code."

farspark only includes the must-have features for image processing, fine-tuning and security. Specifically,

* It would be great to be able to rotate, flip and apply masks to images, but in most of the cases, it is possible — and is much easier — to do that using CSS3.
* It may be great to have built-in HTTP caching of some kind, but it is way better to use a Content-Delivery Network or a caching proxy server for this, as you will have to do this sooner or later in the production environment.
* It might be useful to have everything built in — such as HTTPS support — but an easy way to solve that would be just to use a proxying HTTP server such as nginx.

#### Speed

farspark uses probably the most efficient image processing library there is, called `libvips`. It is screaming fast and has a very low memory footprint; with it, we can handle the processing for a massive amount of images on the fly.

farspark also uses native Go's `net/http` routing for the best HTTP networking support.

#### Security

Massive processing of remote images is a potentially dangerous thing, security-wise. There are many attack vectors, so it is a good idea to consider attack prevention measures first. Here is how farspark can help:

* farspark checks image type _and_ "real" dimensions when downloading, so the image will not be fully downloaded if it has an unknown format or the dimensions are too big (there is a setting for that). That is how farspark protects you from so called "image bombs" like those described at  https://www.bamsoftware.com/hacks/deflate.html.

* farspark protects image URLs with a signature, so an attacker cannot cause a denial-of-service attack by requesting multiple image resizes.

* farspark supports authorization by an HTTP header. That prevents using farspark directly by an attacker but allows to use it through a CDN or a caching server — just by adding a header to a proxy or CDN config.

## Installation

There are two ways you can install farspark:

#### From the source

1. First, install [libvips](https://github.com/jcupitt/libvips).

  ```bash
  # macOS
  $ brew tap homebrew/science
  $ brew install vips

  # Ubuntu
  # Ubuntu apt repository contains a pretty old version of libvips.
  # It's recommended to use PPA with an up to date version.
  $ sudo add-apt-repository ppa:dhor/myway
  $ sudo apt-get install libvips-dev
  ```

  **Note:** Most libvips packages come with WebP support. If you want libvips to support WebP on macOS, you need to install it this way:

  ```bash
  $ brew tap homebrew/science
  $ brew install vips --with-webp
  ```

2. Next, install farspark itself:

  ```bash
  $ go get -f -u github.com/DarthSim/farspark
  ```

#### Docker

farspark can (and should) be used as a standalone application inside a Docker container. It is ready to be dockerized, plug and play:

```bash
$ docker build -t farspark .
$ docker run -e FARSPARK_KEY=$YOUR_KEY -e FARSPARK_SALT=$YOUR_SALT -p 8080:8080 -t farspark
```

You can also pull the image from Docker Hub:

```bash
$ docker pull darthsim/farspark:latest
$ docker run -e FARSPARK_KEY=$YOUR_KEY -e FARSPARK_SALT=$YOUR_SALT -p 8080:8080 -t darthsim/farspark
```

#### Heroku

farspark can be deployed to Heroku with the click of the button:

[![Deploy](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy)

However, you can do it manually with a few steps:

```bash
$ git clone https://github.com/DarthSim/farspark.git && cd farspark
$ heroku create your-application
$ heroku buildpacks:add https://github.com/heroku/heroku-buildpack-apt
$ heroku buildpacks:add https://github.com/heroku/heroku-buildpack-go
$ heroku config:set FARSPARK_KEY=$YOUR_KEY FARSPARK_SALT=$YOUR_SALT
$ git push heroku master
```

## Configuration

farspark is [Twelve-Factor-App](https://12factor.net/)-ready and can be configured using `ENV` variables.

#### URL signature

farspark requires all URLs to be signed with a key and salt:

* `FARSPARK_KEY` — (**required**) hex-encoded key;
* `FARSPARK_SALT` — (**required**) hex-encoded salt;

You can also specify paths to files with a hex-encoded key and salt (useful in a development environment):

```bash
$ farspark -keypath /path/to/file/with/key -saltpath /path/to/file/with/salt
```

If you need a random key/salt pair real fast, you can quickly generate it using, for example, the following snippet:

```bash
$ xxd -g 2 -l 64 -p /dev/random | tr -d '\n'
```

#### Server

* `FARSPARK_BIND` — TCP address to listen on. Default: `:8080`;
* `FARSPARK_READ_TIMEOUT` — the maximum duration (in seconds) for reading the entire image request, including the body. Default: `10`;
* `FARSPARK_WRITE_TIMEOUT` — the maximum duration (in seconds) for writing the response. Default: `10`;
* `FARSPARK_DOWNLOAD_TIMEOUT` — the maximum duration (in seconds) for downloading the source image. Default: `5`;
* `FARSPARK_CONCURRENCY` — the maximum number of image requests to be processed simultaneously. Default: double number of CPU cores;
* `FARSPARK_MAX_CLIENTS` — the maximum number of simultaneous active connections. Default: `FARSPARK_CONCURRENCY * 10`;
* `FARSPARK_TTL` — duration in seconds sent in `Expires` and `Cache-Control: max-age` headers. Default: `3600` (1 hour);
* `FARSPARK_USE_ETAG` — when true, enables using [ETag](https://en.wikipedia.org/wiki/HTTP_ETag) header for the cache control. Default: false;
* `FARSPARK_LOCAL_FILESYSTEM_ROOT` — root of the local filesystem. See [Serving local files](#serving-local-files). Keep empty to disable serving of local files.

#### Security

farspark protects you from so-called image bombs. Here is how you can specify maximum image dimensions and resolution which you consider reasonable:

* `FARSPARK_ALLOW_ORIGIN` - when set, enables CORS headers with provided origin. CORS headers are disabled by default.
* `FARSPARK_MAX_SRC_DIMENSION` — the maximum dimensions of the source image, in pixels, for both width and height. Images with larger real size will be rejected. Default: `8192`;
* `FARSPARK_MAX_SRC_RESOLUTION` — the maximum resolution of the source image, in megapixels. Images with larger real size will be rejected. Default: `16.8`;

You can also specify a secret to enable authorization with the HTTP `Authorization` header:

* `FARSPARK_SECRET` — the authorization token. If specified, request should contain the `Authorization: Bearer %secret%` header;

#### Compression

* `FARSPARK_QUALITY` — quality of the resulting image, percentage. Default: `80`;
* `FARSPARK_GZIP_COMPRESSION` — GZip compression level. Default: `5`;

#### Miscellaneous

* `FARSPARK_BASE_URL` - base URL part which will be added to every requestsd image URL. For example, if base URL is `http://example.com/images` and `/path/to/image.png` is requested, farspark will download the image from `http://example.com/images/path/to/image.png`. Default: blank.

## Generating the URL

The URL should contain the signature and resize parameters, like this:

```
/%signature/%resizing_type/%width/%height/%enlarge/%encoded_url.%extension
```

#### Resizing types

farspark supports the following resizing types:

* `fit` — resizes the image while keeping aspect ratio to fit given size;
* `fill` — resizes the image while keeping aspect ratio to fill given size and cropping projecting parts;
* `crop` — crops the image to a given size.

#### Width and height

Width and height parameters define the size of the resulting image. Depending on the resizing type applied, the dimensions may differ from the requested ones.

#### Enlarge

If set to `0`, farspark will not enlarge the image if it is smaller than the given size. With any other value, farspark will enlarge the image.

#### Encoded URL

The source URL should be encoded with URL-safe Base64. The encoded URL can be split with `/` for your needs.

#### Extension

Extension specifies the format of the resulting image. At the moment, farspark supports only `jpg`, `png` and `webp`, them being the most popular and useful web image formats.

#### Signature

Signature is a URL-safe Base64-encoded HMAC digest of the rest of the path including the leading `/`. Here's how it is calculated:

* Take the path after the signature — `/%resizing_type/%width/%height/%enlarge/%encoded_url.%extension`;
* Add salt to the beginning;
* Calculate the HMAC digest using SHA256;
* Encode the result with URL-safe Base64.

You can find helpful code snippets in the `examples` folder.

## Serving local files

farspark can process files from your local filesystem. To use this feature do the following:

1. Set `FARSPARK_LOCAL_FILESYSTEM_ROOT` to your images directory path.
2. Use `local:///path/to/image.jpg` as the source image url.

## Source image formats support

farspark supports only the most popular image formats of the moment: PNG, JPEG, GIF and WebP.

## Deployment

There is a special endpoint `/health`, which returns HTTP Status `200 OK` after server successfully starts. This can be used to check container readiness.

## Author

Sergey "DarthSim" Aleksandrovich

Many thanks to @romashamin for the awesome logo.

## License

farspark is licensed under the MIT license.

See LICENSE for the full license text.
