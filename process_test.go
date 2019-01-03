package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/url"
	"testing"
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

func Test_PDF_PNG(t *testing.T) {
	in, out := loadTestData(t, "in1.pdf", "out1.png")
	result, _, err := extractPDFPage(in, "dummy", 3, "image/png")

	ioutil.WriteFile("out.png", result, 0600);

	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, result) {
		t.Fatal("Unexpected output.")
	}
}

func Test_GLTF_GLTF_rewrite(t *testing.T) {
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

func Test_GLTF_GLTF_no_images(t *testing.T) {
	in, out := loadTestData(t, "in3.gltf", "out3.gltf")
	baseURL, err := url.Parse("https://poly.googleapis.com/downloads/2PDe5PSncTC/bM1VRy9M_TP/Wolf_01.gltf")
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
