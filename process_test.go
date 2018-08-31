package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/url"
	"testing"
	"time"
)

const dataDir = "testdata"

func loadTestData(t *testing.T, inFile string, outFile string) ([]byte, []byte) {
	in, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", dataDir, inFile))
	if err != nil {
		t.Fatal(err)
	}
	out, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", dataDir, outFile))
	if err != nil {
		t.Fatal(err)
	}
	return in, out
}

func Test_PNG_PNG(t *testing.T) {
	in, out := loadTestData(t, "in0.png", "out0.png")
	po := processingOptions{Method: Extract, Format: PNG, Index: 0}
	timer := startTimer(time.Duration(10)*time.Second, "Processing")
	result, _, err := processMedia(in, "dummy", PNG, po, timer)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, result) {
		t.Fatal("Unexpected output.")
	}
}

func Test_PDF_PNG(t *testing.T) {
	in, out := loadTestData(t, "in1.pdf", "out1.png")
	po := processingOptions{Method: Extract, Format: PNG, Index: 3}
	timer := startTimer(time.Duration(10)*time.Second, "Processing")
	result, _, err := processMedia(in, "dummy", PDF, po, timer)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, result) {
		t.Fatal("Unexpected output.")
	}
}

func Test_GLTF_GLTF(t *testing.T) {
	in, out := loadTestData(t, "in2.gltf", "out2.gltf")
	baseURL, err := url.Parse("https://asset-bundles-prod.reticulum.io/rooms/atrium/AtriumMeshes-5f8fb06d92.gltf")
	if err != nil {
		t.Fatal(err)
	}
	serverURL, err := url.Parse("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	result, err := processGLTF(in, baseURL, serverURL)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, result) {
		t.Fatal("Unexpected output.")
	}
}
