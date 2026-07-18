package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const marker = "__GA_ADMIN_BUNDLED_WORLDLINE_B64__"

func main() {
	workerPath := flag.String("worker", "", "source chat_worker.py")
	worldlinePath := flag.String("worldline", "", "source frontends/worldline.py")
	outputPath := flag.String("output", "", "packaged chat_worker.py")
	flag.Parse()
	if *workerPath == "" || *worldlinePath == "" || *outputPath == "" {
		fatalf("worker, worldline, and output are required")
	}
	worker, err := os.ReadFile(*workerPath)
	if err != nil {
		fatalf("read worker: %v", err)
	}
	worldline, err := os.ReadFile(*worldlinePath)
	if err != nil {
		fatalf("read worldline: %v", err)
	}
	packaged, err := packageRuntime(worker, worldline)
	if err != nil {
		fatalf("package chat runtime: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0755); err != nil {
		fatalf("create output directory: %v", err)
	}
	if err := os.WriteFile(*outputPath, packaged, 0644); err != nil {
		fatalf("write packaged worker: %v", err)
	}
	fmt.Printf("packaged %s with %d-byte worldline runtime\n", *outputPath, len(worldline))
}

func packageRuntime(worker, worldline []byte) ([]byte, error) {
	if bytes.Count(worker, []byte(marker)) != 2 {
		return nil, errors.New("worker must contain exactly two bundle markers")
	}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(worldline); err != nil {
		return nil, fmt.Errorf("compress worldline: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finish worldline compression: %w", err)
	}
	payload := base64.StdEncoding.EncodeToString(compressed.Bytes())
	return bytes.Replace(worker, []byte(marker), []byte(payload), 1), nil
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
