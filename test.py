#!/usr/bin/env python3

import base64
import hashlib
import hmac
import textwrap
import urllib.request

key = bytes.fromhex("943b421c9eb07c830af81030552c86009268de4e532ba2ee2eab8247c6da0881")
salt = bytes.fromhex("520f986b998545b4785e0defbc4f3c1203f22de2374a3d53cb7a7fe9fea309c5")

url = b"https://asset-bundles-prod.reticulum.io/rooms/atrium/AtriumMeshes-5f8fb06d92.gltf"

encoded_url = base64.urlsafe_b64encode(url).rstrip(b"=").decode()
# You can trim padding spaces to get good-looking url
encoded_url = '/'.join(textwrap.wrap(encoded_url, 16))

path = "/{resize}/{width}/{height}/{enlarge}/{index}/{encoded_url}".format(
    encoded_url=encoded_url,
    resize="raw",
    width=0,
    height=0,
    enlarge=0,
    index=0
).encode()
digest = hmac.new(key, msg=salt+path, digestmod=hashlib.sha256).digest()

protection = base64.urlsafe_b64encode(digest).rstrip(b"=")

url = b'/%s%s' % (
    protection,
    path,
)

with urllib.request.urlopen("http://localhost:8080" + url.decode()) as f:
    print(f.read().decode('utf-8'))

# without / in url
# /_PQ4ytCQMMp-1w1m_vP6g8Qb-Q7yF9mwghf6PddqxLw/fill/300/300/no/1/aHR0cDovL2ltZy5leGFtcGxlLmNvbS9wcmV0dHkvaW1hZ2UuanBn.png

# with / in url
# /MlF9VpgaHqcmVK3FyT9CTJhfm0rfY6JKnAtxoiAX9t0/fill/300/300/no/1/aHR0cDovL2ltZy5l/eGFtcGxlLmNvbS9w/cmV0dHkvaW1hZ2Uu/anBn.png
