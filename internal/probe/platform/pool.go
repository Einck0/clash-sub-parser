package platform

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"
)

var bodyBufPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 64<<10))
	},
}

func getPooledBuf() *bytes.Buffer {
	buf := bodyBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func putPooledBuf(buf *bytes.Buffer) {
	if buf == nil || buf.Cap() > 4<<20 {
		return
	}
	bodyBufPool.Put(buf)
}

func readJSONPooled(r io.Reader, v any) error {
	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(r); err != nil {
		return err
	}
	return json.Unmarshal(buf.Bytes(), v)
}
