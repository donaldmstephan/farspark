# farspark

farspark is a fast and secure standalone server, based on [imgproxy](https://github.com/DarthSim/imgproxy), for resizing, converting, and proxying remote media. The main principles of are simplicity, speed, and security. farspark is used with [hubs](https://github.com/mozilla/hubs) in order to proxy and convert shared media assets such as images, videos, and GLTF assets. See the [imgproxy documentation](https://github.com/DarthSim/imgproxy/blob/master/README.md) for more details.

## Installation

1. Install farspark dependencies:

``` bash
sudo apt install libgs-dev
```

2. Next, install farspark itself:

```bash
$ go get -f -u github.com/MozillaReality/farspark
```

#### Habitat

Farspark publishes a Habitat plan for its installation, and is the method we use to deploy farspark for [Hubs](https://hubs.mozilla.com).

## Features

Farspark's features are largely a superset of imgproxy's.

#### Configuration

Farspark supports most [imgproxy configuration options](https://github.com/DarthSim/imgproxy/blob/master/README.md#configuration), plus:

* `FARSPARK_ALLOW_ORIGINS` - when set, enables CORS headers with provided list of comma-separated origins. CORS headers are disabled by default.
* `FARSPARK_SERVER_URL` - The URL of this server; used for rewriting URLs for asset subresources, i.e. in GLTFs.
* `FARSPARK_CACHE_ROOT` - Root folder for filesystem cache used to speed up frame/page extraction across requests
* `FARSPARK_CACHE_SIZE` - Size (in bytes) for the filesystem cache

#### Processing methods

Farspark supports most [imgproxy resizing types](https://github.com/DarthSim/imgproxy/blob/master/README.md#resizing-types), plus:

* `extract` — does not perform any image transformations, but extracts a single page or frame from an indexable media as an image (right now video and PDFs are supported.)
* `raw` — performs no extraction or processing but streams the media through as-is as a proxy (this can be used to simply add CORS headers.) Note that when `raw` is specified, you can also perform an HTTP `HEAD` request to just fetch the remote HTTP headers.

#### Index

If the media being requested has multiple pages or frames, you can request to render a specific one. The page/frame index starts at zero, and media which supports index selection will include an `X-Max-Content-Index` header to indicate the maximum index that can be requested. Right now only supported for PDFs.

## Author

imgproxy by Sergey "DarthSim" Aleksandrovich

## License

imgproxy and farspark are both licensed under the MIT license.
