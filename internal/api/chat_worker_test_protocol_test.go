package api

import (
	"bufio"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

func decodeWorkerTestRequest(reader io.Reader, target interface{}) error {
	// 假 worker 必须读完换行帧，否则 io.Pipe 写入与回复会相互等待。
	frame, err := bufio.NewReader(reader).ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(frame, target)
}

func TestDecodeWorkerTestRequestDrainsFrameBeforeReply(t *testing.T) {
	const waitForWrite = time.Second
	const emptyRequest = "{\"padding\":\"\"}"
	for _, size := range []int{510, 511, 512, 513, 1024} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			reader, writer := io.Pipe()
			t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
			request := map[string]string{"padding": strings.Repeat("x", size-len(emptyRequest))}
			writeDone := make(chan error, 1)
			go func() { writeDone <- json.NewEncoder(writer).Encode(request) }()
			var decoded map[string]string
			if err := decodeWorkerTestRequest(reader, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded["padding"] != request["padding"] {
				t.Fatal("worker request payload changed")
			}
			select {
			case err := <-writeDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(waitForWrite):
				t.Fatal("request writer is blocked on an unread frame delimiter before worker reply")
			}
		})
	}
}

func TestDecodeWorkerTestRequestRejectsInvalidFrames(t *testing.T) {
	for name, frame := range map[string]string{
		"missing delimiter": "{\"padding\":\"value\"}",
		"invalid JSON":      "{invalid}\n",
	} {
		t.Run(name, func(t *testing.T) {
			var decoded map[string]string
			if err := decodeWorkerTestRequest(strings.NewReader(frame), &decoded); err == nil {
				t.Fatal("invalid worker request frame was accepted")
			}
		})
	}
}
