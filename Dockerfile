FROM alpine:edge
MAINTAINER Greg Fodor <gfodor@mozilla.com>

ENV GOPATH /go
ENV PATH /usr/local/go/bin:$PATH

RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

ADD . /go/src/github.com/gfodor/farspark

RUN echo "http://dl-cdn.alpinelinux.org/alpine/edge/testing" >> /etc/apk/repositories \
  && apk add --no-cache --update bash vips ca-certificates \
  && apk add --no-cache --virtual .build-deps go gcc musl-dev fftw-dev vips-dev \
  && cd /go/src/github.com/gfodor/farspark \
  && CGO_LDFLAGS_ALLOW="-s|-w" go build -v -o /usr/local/bin/farspark \
  && apk del --purge .build-deps \
  && rm -rf /var/cache/apk*

CMD ["farspark"]

EXPOSE 8080
