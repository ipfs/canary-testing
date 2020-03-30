package rpc

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/ipfs/testground/pkg/logging"

	"github.com/docker/docker/pkg/ioutils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type OutputWriter struct {
	sync.Mutex
	*zap.SugaredLogger
	*progressWriter

	out io.Writer
}

var _ io.Writer = (*OutputWriter)(nil)

func NewOutputWriter(w http.ResponseWriter, r *http.Request) *OutputWriter {
	w.Header().Set("Content-Type", "application/json")

	httpWriter := ioutils.NewWriteFlusher(w)

	// progressWriter will emit log output as progress messages.
	progressWriter := &progressWriter{out: httpWriter}
	writeSyncer := zapcore.Lock(zapcore.AddSync(progressWriter))

	// this logger has two sinks: stdout and the writeSyncer, wired to the HTTP
	// response.
	logger := logging.NewLogger(writeSyncer).With(zap.String("req_id", r.Header.Get("X-Request-ID")))

	ow := &OutputWriter{
		SugaredLogger:  logger.Sugar(),
		out:            httpWriter,
		progressWriter: progressWriter,
	}

	// we need to wire this back for the lock.
	progressWriter.ow = ow
	return ow
}

func Discard() *OutputWriter {
	pw := &progressWriter{out: ioutil.Discard}
	ow := &OutputWriter{
		SugaredLogger:  zap.NewNop().Sugar(),
		out:            ioutil.Discard,
		progressWriter: pw,
	}
	ow.progressWriter = pw
	pw.ow = ow
	return ow
}

type progressWriter struct {
	ow  *OutputWriter
	out io.Writer
}

var _ io.Writer = (*progressWriter)(nil)

// Write on the logWriter wraps the incoming write into a progress message.
func (w *progressWriter) Write(p []byte) (n int, err error) {
	if p == nil {
		return 0, nil
	}

	msg := Chunk{Type: ChunkTypeProgress, Payload: p}
	json, err := json.Marshal(msg)
	if err != nil {
		return 0, err
	}

	w.ow.Lock()
	defer w.ow.Unlock()

	return w.out.Write(json)
}

// infoWriter implements io.Writer, and turns all writes into Info log
// statements in the underlying logger.
type infoWriter struct{ ow *OutputWriter }

var _ io.Writer = (*infoWriter)(nil)

func (iw *infoWriter) Write(p []byte) (n int, err error) {
	iw.ow.Info(string(p))
	return len(p), nil
}

// InfoWriter returns an io.Writer that turns all writes into Info log
// statements in the underlying logger.
func (ow *OutputWriter) InfoWriter() io.Writer {
	return &infoWriter{ow}
}

// stdoutWriter implements io.Writer, and turns all writes into stdout writes,
// piping them to the underlying progressWriter, so that they're sent to the client.
type stdoutWriter struct{ ow *OutputWriter }

var _ io.Writer = (*stdoutWriter)(nil)

func (sw *stdoutWriter) Write(p []byte) (n int, err error) {
	_, _ = os.Stdout.Write(p)
	return sw.ow.progressWriter.Write(p)
}

// StdoutWriter returns an io.Writer that prints all writes into Stdout, and
// sends them to the client as a progress chunk.
func (ow *OutputWriter) StdoutWriter() io.Writer {
	return &stdoutWriter{ow}
}

// With returns a new OutputWriter, replacing the SugaredLogger with the result
// from delegating to SugaredLogger.With.
func (ow *OutputWriter) With(args ...interface{}) *OutputWriter {
	return &OutputWriter{
		SugaredLogger:  ow.SugaredLogger.With(args...),
		out:            ow.out,
		progressWriter: ow.progressWriter,
	}
}

func (ow *OutputWriter) WriteResult(res interface{}) {
	msg := Chunk{Type: ChunkTypeResult, Payload: res}
	json, err := json.Marshal(msg)
	if err != nil {
		logging.S().Errorw("could not write result", "err", err)
		return
	}

	ow.Lock()
	defer ow.Unlock()

	_, err = ow.out.Write(json)
	if err != nil {
		logging.S().Errorw("could not write result", "err", err)
	}
}

func (ow *OutputWriter) WriteError(message string, keysAndValues ...interface{}) {
	ow.Warnw(message, keysAndValues...)

	if len(keysAndValues) > 0 {
		b := &strings.Builder{}
		for i := 0; i < len(keysAndValues); i = i + 2 {
			fmt.Fprintf(b, "%s: %s;", keysAndValues[i], keysAndValues[i+1])
		}
		kvs := b.String()
		message = message + "; " + kvs[:len(kvs)-1]
	}

	pld := Chunk{Type: ChunkTypeError, Error: &Error{message}}
	json, err := json.Marshal(pld)
	if err != nil {
		logging.S().Errorw("could not write error response", "err", err)
		return
	}

	ow.Lock()
	defer ow.Unlock()

	_, err = ow.out.Write(json)
	if err != nil {
		logging.S().Errorw("could not write error response", "err", err)
	}
}

func (ow *OutputWriter) Flush() {
	if f, ok := ow.out.(http.Flusher); ok {
		f.Flush()
	}
}
